package Accounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTenantConfigResolvesSecretsAndDefaults(t *testing.T) {
	path := writeTenantConfig(t, `{
  "version": 1,
  "sessionKeyEnv": "TENANT_SESSION_KEY",
  "accounts": [
    {"id": "Alpha", "tokenEnv": "ALPHA_TOKEN"},
    {"id": "bravo", "tokenEnv": "BRAVO_TOKEN", "startSession": false}
  ]
}`)
	values := map[string]string{
		"ALPHA_TOKEN":        strings.Repeat("a", 32),
		"BRAVO_TOKEN":        strings.Repeat("b", 32),
		"TENANT_SESSION_KEY": strings.Repeat("k", 32),
	}
	loaded, err := LoadTenantConfig(path, func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.MaxAccounts != DefaultTenantMaxAccounts {
		t.Fatalf("max accounts = %d", loaded.MaxAccounts)
	}
	if len(loaded.Accounts) != 2 || loaded.Accounts[0].ID != "alpha" || !loaded.Accounts[0].StartSession {
		t.Fatalf("first account = %+v", loaded.Accounts)
	}
	if loaded.Accounts[1].StartSession {
		t.Fatal("explicit startSession=false was ignored")
	}
	if string(loaded.SessionKey) != values["TENANT_SESSION_KEY"] {
		t.Fatal("session key environment was not resolved")
	}
}

func TestLoadTenantConfigFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{"unknown field", `{"version":1,"accounts":[],"surprise":true}`, "unknown field"},
		{"duplicate account", `{"version":1,"accounts":[{"id":"alpha","tokenEnv":"A_TOKEN"},{"id":"ALPHA","tokenEnv":"B_TOKEN"}]}`, "more than once"},
		{"reused secret", `{"version":1,"accounts":[{"id":"alpha","tokenEnv":"A_TOKEN"},{"id":"bravo","tokenEnv":"A_TOKEN"}]}`, "reused"},
		{"over limit", `{"version":1,"maxAccounts":1,"accounts":[{"id":"alpha","tokenEnv":"A_TOKEN"},{"id":"bravo","tokenEnv":"B_TOKEN"}]}`, "maxAccounts"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeTenantConfig(t, test.config)
			_, err := LoadTenantConfig(path, func(string) (string, bool) { return strings.Repeat("x", 32), true })
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func writeTenantConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tenant.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
