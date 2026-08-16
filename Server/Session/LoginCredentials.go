package Session

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	loginCredentialFileName           = "LoginCredentials.json"
	backgroundLoginCredentialFileName = "BackgroundLogin.json"
	loginCredentialSchemaVersion      = 1
	maxLoginUsernameBytes             = 1024
	maxLoginPasswordBytes             = 16 << 10
	maxGameServerSelectionBytes       = 32
)

var (
	loginLanguagePattern       = regexp.MustCompile(`^[A-Za-z]{2}(?:_[A-Za-z]{2})?$`)
	gameServerSelectionPattern = regexp.MustCompile(
		`^[A-Za-z](?:[A-Za-z0-9-]{0,30}[A-Za-z0-9])?$`,
	)
	gameServerHostnamePattern = regexp.MustCompile(
		`^ep-live-([a-z](?:[a-z0-9-]{0,30}[a-z0-9])?)-game\.goodgamestudios\.com$`,
	)
)

// BackgroundLoginInput is accepted only by the protected session-login API.
// It must not be submitted through the intent engine because intent arguments
// are retained in operation receipts.
type BackgroundLoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Server   string `json:"server"`
	// ServerURL and Zone optionally pin the world's websocket endpoint and
	// SmartFox zone, as resolved by the hosted control plane's game server
	// directory. Both are validated against the official host and zone
	// shapes; empty means "resolve from the local catalog".
	ServerURL string `json:"serverUrl,omitempty"`
	Zone      string `json:"zone,omitempty"`
	Language  string `json:"language,omitempty"`
}

// BackgroundLoginStatus deliberately excludes the username and password.
type BackgroundLoginStatus struct {
	Configured bool       `json:"configured"`
	Server     string     `json:"server,omitempty"`
	ServerURL  string     `json:"serverUrl,omitempty"`
	Language   string     `json:"language,omitempty"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}

type BackgroundLoginStore struct {
	dataDir string
	mu      sync.Mutex
}

func NewBackgroundLoginStore(dataDir string) *BackgroundLoginStore {
	return &BackgroundLoginStore{dataDir: dataDir}
}

func (store *BackgroundLoginStore) Status() (BackgroundLoginStatus, error) {
	if store == nil {
		return BackgroundLoginStatus{}, fmt.Errorf("background login store is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return backgroundLoginStatus(store.dataDir)
}

func (store *BackgroundLoginStore) Configure(input BackgroundLoginInput) (BackgroundLoginStatus, error) {
	if store == nil {
		return BackgroundLoginStatus{}, fmt.Errorf("background login store is unavailable")
	}
	username := strings.TrimSpace(input.Username)
	serverURL, server, err := backgroundGameServerURL(input.Server)
	if err != nil {
		return BackgroundLoginStatus{}, err
	}
	// An explicitly resolved endpoint (from the control plane's game server
	// directory) wins over the local catalog, but only when it points at an
	// official live game host.
	if explicitURL := strings.TrimSpace(input.ServerURL); explicitURL != "" {
		if err := validateDirectGameServerURL(explicitURL); err != nil {
			return BackgroundLoginStatus{}, fmt.Errorf("game server URL is invalid: %w", err)
		}
		serverURL = explicitURL
	}
	zone := strings.TrimSpace(input.Zone)
	if zone != "" && !gameNamespacePattern.MatchString(zone) {
		return BackgroundLoginStatus{}, fmt.Errorf("game server zone is invalid")
	}
	language := strings.TrimSpace(input.Language)
	if language == "" {
		language = defaultGameLanguage
	}
	credential := persistedLoginCredential{
		SchemaVersion: loginCredentialSchemaVersion,
		CapturedAt:    time.Now().UTC(),
		AutoRestore:   true,
		Username:      username,
		Password:      input.Password,
		Server:        server,
		ServerURL:     serverURL,
		Zone:          zone,
		Language:      language,
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := saveBackgroundLoginCredential(store.dataDir, credential); err != nil {
		return BackgroundLoginStatus{}, err
	}
	return BackgroundLoginStatus{
		Configured: true,
		Server:     server,
		ServerURL:  serverURL,
		Language:   language,
		UpdatedAt:  &credential.CapturedAt,
	}, nil
}

// Clear removes every saved game login for the profile: the Background login
// written through Configure and any Full-mode capture. It is used when
// orchestration revokes an account's credential from a cell.
func (store *BackgroundLoginStore) Clear() error {
	if store == nil {
		return fmt.Errorf("background login store is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return ClearSavedLogins(store.dataDir)
}

// ClearSavedLogins deletes the saved game login files under dataDir without
// touching the rest of the profile. Missing files are not an error.
func ClearSavedLogins(dataDir string) error {
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("game login data directory is required")
	}
	var clearErr error
	for _, path := range []string{backgroundLoginCredentialPath(dataDir), loginCredentialPath(dataDir)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			clearErr = fmt.Errorf("remove saved game login: %w", err)
		}
	}
	return clearErr
}

func backgroundLoginStatus(dataDir string) (BackgroundLoginStatus, error) {
	credential, err := loadBackgroundLoginCredential(dataDir)
	if os.IsNotExist(err) {
		return BackgroundLoginStatus{}, nil
	}
	if err != nil {
		return BackgroundLoginStatus{}, err
	}
	server := strings.ToUpper(strings.TrimSpace(credential.Server))
	if server == "" {
		server = gameServerSelection(credential.ServerURL)
	}
	configured := credential.AutoRestore && server != ""
	return BackgroundLoginStatus{
		Configured: configured,
		Server:     server,
		ServerURL:  credential.ServerURL,
		Language:   credential.Language,
		UpdatedAt:  &credential.CapturedAt,
	}, nil
}

// backgroundGameServerURL resolves a world code (US1, GB1, INT1 …) to its
// secure WebSocket URL. Known worlds come from the game server catalog, which
// is what makes multi-zone hosts (GB1 lives on ep-live-mz-int1-sk1-gb1-game)
// connect; a code the catalog does not list yet falls back to the single-world
// host convention so a freshly opened world is still reachable.
func backgroundGameServerURL(selection string) (string, string, error) {
	selection = strings.TrimSpace(selection)
	if len(selection) == 0 || len(selection) > maxGameServerSelectionBytes ||
		!gameServerSelectionPattern.MatchString(selection) {
		return "", "", fmt.Errorf("game server must be a world code such as US1")
	}
	server := strings.ToUpper(selection)
	if known, ok := LookupGameServer(server); ok {
		return known.URL, known.Code, nil
	}
	serverURL := fmt.Sprintf(
		"wss://ep-live-%s-game.goodgamestudios.com:443",
		strings.ToLower(server),
	)
	if err := validateDirectGameServerURL(serverURL); err != nil {
		return "", "", fmt.Errorf("game server selection is invalid: %w", err)
	}
	return serverURL, server, nil
}

// gameServerSelection recovers a world code from a server URL for logins
// saved before the code itself was persisted. Single-world hosts resolve
// exactly; a multi-zone host without a saved code reports its first world.
func gameServerSelection(serverURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil {
		return ""
	}
	if servers := gameServersForHost(parsed.Hostname()); len(servers) > 0 {
		return servers[0].Code
	}
	match := gameServerHostnamePattern.FindStringSubmatch(strings.ToLower(parsed.Hostname()))
	if len(match) != 2 {
		return ""
	}
	return strings.ToUpper(match[1])
}

type persistedLoginCredential struct {
	SchemaVersion int       `json:"schemaVersion"`
	CapturedAt    time.Time `json:"capturedAt"`
	AutoRestore   bool      `json:"autoRestore"`
	Username      string    `json:"username"`
	Password      string    `json:"password"`
	// Server is the world code the login was saved for (US1, GB1 …). It is
	// what picks the SmartFox zone on multi-zone hosts; ServerURL alone cannot.
	Server    string `json:"server,omitempty"`
	ServerURL string `json:"serverUrl,omitempty"`
	// Zone pins the SmartFox zone when the login was installed with an
	// explicitly resolved endpoint; empty means "derive from the catalog".
	Zone     string `json:"zone,omitempty"`
	Language string `json:"language,omitempty"`
}

func loadLoginCredential(dataDir string) (persistedLoginCredential, error) {
	return loadPersistedLoginCredential(loginCredentialPath(dataDir))
}

func loadBackgroundLoginCredential(dataDir string) (persistedLoginCredential, error) {
	return loadPersistedLoginCredential(backgroundLoginCredentialPath(dataDir))
}

func loadPersistedLoginCredential(path string) (persistedLoginCredential, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return persistedLoginCredential{}, err
	}
	var credential persistedLoginCredential
	if err := json.Unmarshal(contents, &credential); err != nil {
		return persistedLoginCredential{}, fmt.Errorf("decode saved game login: %w", err)
	}
	if credential.SchemaVersion != loginCredentialSchemaVersion {
		return persistedLoginCredential{}, fmt.Errorf(
			"saved game login schema %d is not supported", credential.SchemaVersion,
		)
	}
	if err := validateLoginCredential(credential); err != nil {
		return persistedLoginCredential{}, err
	}
	_ = os.Chmod(path, 0o600)
	return credential, nil
}

func saveLoginCredential(dataDir string, credential persistedLoginCredential) error {
	return savePersistedLoginCredential(dataDir, loginCredentialFileName, ".LoginCredentials-", credential)
}

func saveBackgroundLoginCredential(dataDir string, credential persistedLoginCredential) error {
	return savePersistedLoginCredential(dataDir, backgroundLoginCredentialFileName, ".BackgroundLogin-", credential)
}

func savePersistedLoginCredential(
	dataDir string,
	fileName string,
	temporaryPrefix string,
	credential persistedLoginCredential,
) error {
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("game login data directory is required")
	}
	if err := validateLoginCredential(credential); err != nil {
		return err
	}
	credential.SchemaVersion = loginCredentialSchemaVersion
	if credential.CapturedAt.IsZero() {
		credential.CapturedAt = time.Now().UTC()
	}
	directory := filepath.Join(dataDir, "Session")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create game login directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("protect game login directory: %w", err)
	}
	contents, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("encode game login: %w", err)
	}
	contents = append(contents, '\n')
	temporary, err := os.CreateTemp(directory, temporaryPrefix+"*")
	if err != nil {
		return fmt.Errorf("create game login file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write game login: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync game login: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	destination := filepath.Join(directory, fileName)
	if err := os.Rename(temporaryPath, destination); err != nil {
		_ = os.Remove(destination)
		if retryErr := os.Rename(temporaryPath, destination); retryErr != nil {
			return fmt.Errorf("save game login: %w", retryErr)
		}
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		return fmt.Errorf("protect game login: %w", err)
	}
	return nil
}

// PrepareBackgroundLogin validates the protected Full-mode bootstrap and
// explicitly authorizes it for the next Background-mode connection. The
// credential and connection profile never leave the Session package.
func PrepareBackgroundLogin(dataDir string) error {
	_, _, err := prepareBackgroundLogin(dataDir)
	return err
}

func prepareBackgroundLogin(dataDir string) (persistedLoginCredential, gameConnectionProfile, error) {
	credential, err := loadLoginCredential(dataDir)
	if err != nil {
		return persistedLoginCredential{}, gameConnectionProfile{}, fmt.Errorf(
			"Background mode needs a valid saved game login; use Full application mode and sign in manually once",
		)
	}
	profile, err := loadGameConnectionProfile(dataDir)
	if err != nil {
		return persistedLoginCredential{}, gameConnectionProfile{}, fmt.Errorf(
			"Background mode needs current connection details; use Full application mode and sign in manually once",
		)
	}
	if credential.AutoRestore {
		return credential, profile, nil
	}
	credential.AutoRestore = true
	credential.CapturedAt = time.Now().UTC()
	if err := saveLoginCredential(dataDir, credential); err != nil {
		return persistedLoginCredential{}, gameConnectionProfile{}, fmt.Errorf(
			"authorize the saved game login for Background mode: %w", err,
		)
	}
	return credential, profile, nil
}

func validateLoginCredential(credential persistedLoginCredential) error {
	if strings.TrimSpace(credential.Username) == "" || len(credential.Username) > maxLoginUsernameBytes {
		return fmt.Errorf("saved game login has an invalid username")
	}
	if credential.Password == "" || len(credential.Password) > maxLoginPasswordBytes {
		return fmt.Errorf("saved game login has an invalid password")
	}
	if serverURL := strings.TrimSpace(credential.ServerURL); serverURL != "" {
		if err := validateDirectGameServerURL(serverURL); err != nil {
			return fmt.Errorf("saved game login has an invalid server selection: %w", err)
		}
	}
	if language := strings.TrimSpace(credential.Language); language != "" &&
		!loginLanguagePattern.MatchString(language) {
		return fmt.Errorf("saved game login has an invalid language")
	}
	return nil
}

func loginCredentialPath(dataDir string) string {
	return filepath.Join(dataDir, "Session", loginCredentialFileName)
}

func backgroundLoginCredentialPath(dataDir string) string {
	return filepath.Join(dataDir, "Session", backgroundLoginCredentialFileName)
}
