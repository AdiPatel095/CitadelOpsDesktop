package Accounts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	TenantConfigVersion       = 1
	DefaultTenantMaxAccounts  = 8
	minimumTenantTokenLength  = 32
	maximumTenantConfigBytes  = 1 << 20
	defaultTenantSessionHours = 12
)

var environmentNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// TenantConfig is deliberately metadata-only. Account secrets stay in the
// environment (normally injected by GCP Secret Manager), never in this file.
type TenantConfig struct {
	Version       int                 `json:"version"`
	MaxAccounts   int                 `json:"maxAccounts,omitempty"`
	SessionKeyEnv string              `json:"sessionKeyEnv,omitempty"`
	Accounts      []TenantFileAccount `json:"accounts"`
}

type TenantFileAccount struct {
	ID           string `json:"id"`
	TokenEnv     string `json:"tokenEnv"`
	StartSession *bool  `json:"startSession,omitempty"`
}

type LoadedTenantConfig struct {
	MaxAccounts int
	SessionKey  []byte
	Accounts    []LoadedTenantAccount
}

type LoadedTenantAccount struct {
	ID           AccountID
	Token        string
	StartSession bool
}

func LoadTenantConfig(path string, lookupEnv func(string) (string, bool)) (LoadedTenantConfig, error) {
	if strings.TrimSpace(path) == "" {
		return LoadedTenantConfig{}, fmt.Errorf("tenant config path is required")
	}
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	file, err := os.Open(path)
	if err != nil {
		return LoadedTenantConfig{}, fmt.Errorf("open tenant config: %w", err)
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, maximumTenantConfigBytes+1))
	if err != nil {
		return LoadedTenantConfig{}, fmt.Errorf("read tenant config: %w", err)
	}
	if len(contents) > maximumTenantConfigBytes {
		return LoadedTenantConfig{}, fmt.Errorf("tenant config exceeds %d bytes", maximumTenantConfigBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var config TenantConfig
	if err := decoder.Decode(&config); err != nil {
		return LoadedTenantConfig{}, fmt.Errorf("decode tenant config: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return LoadedTenantConfig{}, err
	}
	if config.Version != TenantConfigVersion {
		return LoadedTenantConfig{}, fmt.Errorf("tenant config version %d is unsupported; want %d", config.Version, TenantConfigVersion)
	}
	maxAccounts := config.MaxAccounts
	if maxAccounts == 0 {
		maxAccounts = DefaultTenantMaxAccounts
	}
	if maxAccounts < 1 || maxAccounts > 64 {
		return LoadedTenantConfig{}, fmt.Errorf("maxAccounts must be between 1 and 64")
	}
	if len(config.Accounts) == 0 {
		return LoadedTenantConfig{}, fmt.Errorf("tenant config must contain at least one account")
	}
	if len(config.Accounts) > maxAccounts {
		return LoadedTenantConfig{}, fmt.Errorf("tenant config has %d accounts but maxAccounts is %d", len(config.Accounts), maxAccounts)
	}

	loaded := LoadedTenantConfig{MaxAccounts: maxAccounts, Accounts: make([]LoadedTenantAccount, 0, len(config.Accounts))}
	seenIDs := make(map[AccountID]struct{}, len(config.Accounts))
	seenEnvironments := make(map[string]struct{}, len(config.Accounts))
	for index, account := range config.Accounts {
		id, err := ParseAccountID(account.ID)
		if err != nil {
			return LoadedTenantConfig{}, fmt.Errorf("accounts[%d].id: %w", index, err)
		}
		if _, exists := seenIDs[id]; exists {
			return LoadedTenantConfig{}, fmt.Errorf("account %q is configured more than once", id)
		}
		tokenEnvironment := strings.TrimSpace(account.TokenEnv)
		if !environmentNamePattern.MatchString(tokenEnvironment) {
			return LoadedTenantConfig{}, fmt.Errorf("accounts[%d].tokenEnv must be an uppercase environment variable name", index)
		}
		if _, exists := seenEnvironments[tokenEnvironment]; exists {
			return LoadedTenantConfig{}, fmt.Errorf("token environment %q is reused by multiple accounts", tokenEnvironment)
		}
		token, exists := lookupEnv(tokenEnvironment)
		if !exists || len(token) < minimumTenantTokenLength {
			return LoadedTenantConfig{}, fmt.Errorf("account %q token environment %q must contain at least %d characters", id, tokenEnvironment, minimumTenantTokenLength)
		}
		startSession := true
		if account.StartSession != nil {
			startSession = *account.StartSession
		}
		loaded.Accounts = append(loaded.Accounts, LoadedTenantAccount{
			ID: id, Token: token, StartSession: startSession,
		})
		seenIDs[id] = struct{}{}
		seenEnvironments[tokenEnvironment] = struct{}{}
	}
	if keyEnvironment := strings.TrimSpace(config.SessionKeyEnv); keyEnvironment != "" {
		if !environmentNamePattern.MatchString(keyEnvironment) {
			return LoadedTenantConfig{}, fmt.Errorf("sessionKeyEnv must be an uppercase environment variable name")
		}
		key, exists := lookupEnv(keyEnvironment)
		if !exists || len(key) < minimumTenantTokenLength {
			return LoadedTenantConfig{}, fmt.Errorf("session key environment %q must contain at least %d characters", keyEnvironment, minimumTenantTokenLength)
		}
		loaded.SessionKey = []byte(key)
	}
	return loaded, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode tenant config trailing data: %w", err)
	}
	return fmt.Errorf("tenant config contains multiple JSON values")
}
