package Session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	loginCredentialFileName      = "LoginCredentials.json"
	loginCredentialSchemaVersion = 1
	maxLoginUsernameBytes        = 1024
	maxLoginPasswordBytes        = 16 << 10
)

type persistedLoginCredential struct {
	SchemaVersion int       `json:"schemaVersion"`
	CapturedAt    time.Time `json:"capturedAt"`
	AutoRestore   bool      `json:"autoRestore"`
	Username      string    `json:"username"`
	Password      string    `json:"password"`
}

func loadLoginCredential(dataDir string) (persistedLoginCredential, error) {
	path := loginCredentialPath(dataDir)
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
	contents, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("encode game login: %w", err)
	}
	contents = append(contents, '\n')
	temporary, err := os.CreateTemp(directory, ".LoginCredentials-*")
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
	destination := loginCredentialPath(dataDir)
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
	return nil
}

func loginCredentialPath(dataDir string) string {
	return filepath.Join(dataDir, "Session", loginCredentialFileName)
}
