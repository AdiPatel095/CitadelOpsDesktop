package Session

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	gameConnectionProfileFileName      = "GameConnection.json"
	gameConnectionProfileSchemaVersion = 1
	maxGameConnectionProfileBytes      = 64 << 10
	defaultGameNamespace               = "EmpireEx_21"
	defaultGamePlatform                = "web-html5"
	defaultGameLanguage                = "en"
	defaultGameReferrer                = "https://empire.goodgamestudios.com"
)

var (
	gameNamespacePattern = regexp.MustCompile(`^EmpireEx_[0-9]+$`)
	gameBuildPattern     = regexp.MustCompile(`^[0-9]{1,20}$`)
	safeLoginContextKeys = map[string]struct{}{
		"LANG": {}, "DID": {}, "AID": {}, "KID": {}, "ID": {}, "REF": {},
		"PL": {}, "GCI": {}, "SID": {}, "PLFID": {},
	}
)

// gameConnectionProfile caches only validated, non-secret connection metadata.
// It may be captured from a successful Full-mode login or derived from the
// selected server after a successful Background-mode login. Rotating login,
// captcha, and social tokens are deliberately excluded.
type gameConnectionProfile struct {
	SchemaVersion int                        `json:"schemaVersion"`
	CapturedAt    time.Time                  `json:"capturedAt"`
	ServerURL     string                     `json:"serverUrl"`
	Namespace     string                     `json:"namespace"`
	ClientBuild   string                     `json:"clientBuild"`
	Platform      string                     `json:"platform"`
	LoginContext  map[string]json.RawMessage `json:"loginContext"`
}

func loadGameConnectionProfile(dataDir string) (gameConnectionProfile, error) {
	path := gameConnectionProfilePath(dataDir)
	contents, err := os.ReadFile(path)
	if err != nil {
		return gameConnectionProfile{}, err
	}
	if len(contents) > maxGameConnectionProfileBytes {
		return gameConnectionProfile{}, fmt.Errorf("saved game connection profile is too large")
	}
	var profile gameConnectionProfile
	if err := json.Unmarshal(contents, &profile); err != nil {
		return gameConnectionProfile{}, fmt.Errorf("decode saved game connection profile: %w", err)
	}
	if profile.SchemaVersion != gameConnectionProfileSchemaVersion {
		return gameConnectionProfile{}, fmt.Errorf(
			"saved game connection schema %d is not supported", profile.SchemaVersion,
		)
	}
	if err := validateGameConnectionProfile(profile); err != nil {
		return gameConnectionProfile{}, err
	}
	_ = os.Chmod(path, 0o600)
	return profile, nil
}

func saveGameConnectionProfile(dataDir string, profile gameConnectionProfile) error {
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("game connection data directory is required")
	}
	profile.SchemaVersion = gameConnectionProfileSchemaVersion
	if profile.CapturedAt.IsZero() {
		profile.CapturedAt = time.Now().UTC()
	}
	if err := validateGameConnectionProfile(profile); err != nil {
		return err
	}
	directory := filepath.Join(dataDir, "Session")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create game connection directory: %w", err)
	}
	contents, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("encode game connection profile: %w", err)
	}
	if len(contents) > maxGameConnectionProfileBytes {
		return fmt.Errorf("saved game connection profile is too large")
	}
	contents = append(contents, '\n')
	temporary, err := os.CreateTemp(directory, ".GameConnection-*")
	if err != nil {
		return fmt.Errorf("create game connection file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write game connection profile: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync game connection profile: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	destination := gameConnectionProfilePath(dataDir)
	if err := os.Rename(temporaryPath, destination); err != nil {
		_ = os.Remove(destination)
		if retryErr := os.Rename(temporaryPath, destination); retryErr != nil {
			return fmt.Errorf("save game connection profile: %w", retryErr)
		}
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		return fmt.Errorf("protect game connection profile: %w", err)
	}
	return nil
}

func validateGameConnectionProfile(profile gameConnectionProfile) error {
	if !gameBuildPattern.MatchString(strings.TrimSpace(profile.ClientBuild)) {
		return fmt.Errorf("saved game connection has an invalid client build")
	}
	return validateGameConnectionBootstrap(profile)
}

func validateGameConnectionBootstrap(profile gameConnectionProfile) error {
	if !gameNamespacePattern.MatchString(strings.TrimSpace(profile.Namespace)) {
		return fmt.Errorf("saved game connection has an invalid namespace")
	}
	if strings.TrimSpace(profile.Platform) != defaultGamePlatform {
		return fmt.Errorf("saved game connection has an unsupported platform")
	}
	if err := validateDirectGameServerURL(profile.ServerURL); err != nil {
		return err
	}
	for key, value := range profile.LoginContext {
		if _, allowed := safeLoginContextKeys[key]; !allowed {
			return fmt.Errorf("saved game connection contains unsupported login metadata %q", key)
		}
		if len(value) == 0 || !json.Valid(value) {
			return fmt.Errorf("saved game connection contains invalid login metadata %q", key)
		}
	}
	if _, ok := profile.LoginContext["LANG"]; !ok {
		return fmt.Errorf("saved game connection is missing language metadata")
	}
	if _, ok := profile.LoginContext["DID"]; !ok {
		return fmt.Errorf("saved game connection is missing distributor metadata")
	}
	return nil
}

func derivedGameConnectionProfile(
	serverURL string,
	namespace string,
	language string,
) (gameConnectionProfile, error) {
	serverURL = strings.TrimSpace(serverURL)
	if err := validateDirectGameServerURL(serverURL); err != nil {
		return gameConnectionProfile{}, err
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = defaultGameNamespace
	}
	if !gameNamespacePattern.MatchString(namespace) {
		return gameConnectionProfile{}, fmt.Errorf("saved game connection has an invalid namespace")
	}
	language = strings.TrimSpace(language)
	if language == "" {
		language = defaultGameLanguage
	}
	if !loginLanguagePattern.MatchString(language) {
		return gameConnectionProfile{}, fmt.Errorf("saved game connection has an invalid language")
	}
	capturedAt := time.Now().UTC()
	profile := gameConnectionProfile{
		SchemaVersion: gameConnectionProfileSchemaVersion,
		CapturedAt:    capturedAt,
		ServerURL:     serverURL,
		Namespace:     namespace,
		Platform:      defaultGamePlatform,
		LoginContext: map[string]json.RawMessage{
			"LANG":  mustMarshalGameConnectionValue(language),
			"DID":   json.RawMessage(`"0"`),
			"AID":   mustMarshalGameConnectionValue(newGameAccountID(capturedAt)),
			"KID":   json.RawMessage(`""`),
			"ID":    json.RawMessage(`0`),
			"REF":   mustMarshalGameConnectionValue(defaultGameReferrer),
			"PL":    json.RawMessage(`1`),
			"GCI":   json.RawMessage(`""`),
			"SID":   json.RawMessage(`9`),
			"PLFID": json.RawMessage(`1`),
		},
	}
	if err := validateGameConnectionBootstrap(profile); err != nil {
		return gameConnectionProfile{}, err
	}
	return profile, nil
}

func mustMarshalGameConnectionValue(value string) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return json.RawMessage(encoded)
}

func newGameAccountID(now time.Time) string {
	suffix, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		suffix = big.NewInt(now.UnixNano() % 1_000_000)
	}
	return fmt.Sprintf("%d%06d", now.UnixMilli(), suffix.Int64())
}

func validateDirectGameServerURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "wss" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("saved game connection has an invalid secure server URL")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if !gameServerHostnamePattern.MatchString(hostname) {
		return fmt.Errorf("saved game connection server is not an official live game server")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return fmt.Errorf("saved game connection server uses an unsupported port")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("saved game connection server uses an unsupported path")
	}
	return nil
}

func captureGameVersionFrame(raw string) (namespace string, build string, platform string, ok bool) {
	parts := strings.Split(raw, "%")
	if len(parts) < 8 || parts[0] != "" || parts[1] != "xt" ||
		strings.ToLower(strings.TrimSpace(parts[3])) != "vck" {
		return "", "", "", false
	}
	namespace = strings.TrimSpace(parts[2])
	build = strings.TrimSpace(parts[5])
	platform = strings.TrimSpace(parts[6])
	if !gameNamespacePattern.MatchString(namespace) || !gameBuildPattern.MatchString(build) || platform != "web-html5" {
		return "", "", "", false
	}
	return namespace, build, platform, true
}

func sanitizedGameLoginContext(raw json.RawMessage) map[string]json.RawMessage {
	var input map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &input) != nil {
		return nil
	}
	result := make(map[string]json.RawMessage, len(safeLoginContextKeys))
	for key := range safeLoginContextKeys {
		value, ok := input[key]
		if !ok || len(value) == 0 || !json.Valid(value) {
			continue
		}
		result[key] = append(json.RawMessage(nil), value...)
	}
	return result
}

func gameConnectionContextString(context map[string]json.RawMessage, key string) (string, error) {
	raw, ok := context[key]
	if !ok {
		return "", fmt.Errorf("saved game connection is missing %s", key)
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text), nil
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		if _, err := strconv.ParseFloat(number.String(), 64); err == nil {
			return number.String(), nil
		}
	}
	return "", fmt.Errorf("saved game connection has invalid %s", key)
}

func gameConnectionProfilePath(dataDir string) string {
	return filepath.Join(dataDir, "Session", gameConnectionProfileFileName)
}
