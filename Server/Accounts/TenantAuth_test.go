package Accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTenantLoginCreatesAccountScopedSession(t *testing.T) {
	auth := newTestTenantAuthenticator(t)
	request := httptest.NewRequest(http.MethodPost, "https://tenant.example/tenant/login", strings.NewReader(`{
  "accountId":"alpha",
  "token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}`))
	request.Host = "tenant.example"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://tenant.example")
	response := httptest.NewRecorder()
	auth.LoginHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("login status = %d, body %s", response.Code, response.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["path"] != "/accounts/alpha/" {
		t.Fatalf("login path = %q", result["path"])
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v", cookies)
	}
	cookie := cookies[0]
	if cookie.Path != "/accounts/alpha/" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe tenant cookie = %+v", cookie)
	}

	authenticated := httptest.NewRequest(http.MethodGet, "https://tenant.example/accounts/alpha/api/v2/state", nil)
	authenticated.AddCookie(cookie)
	if id, valid := auth.Authenticate(authenticated); !valid || id != "alpha" {
		t.Fatalf("session authentication = %q, %v", id, valid)
	}
	cookie.Value += "tampered"
	tampered := httptest.NewRequest(http.MethodGet, "https://tenant.example/accounts/alpha/", nil)
	tampered.AddCookie(cookie)
	if _, valid := auth.Authenticate(tampered); valid {
		t.Fatal("tampered session authenticated")
	}
}

func TestTenantBearerIdentityComesFromSecretNotPath(t *testing.T) {
	auth := newTestTenantAuthenticator(t)
	request := httptest.NewRequest(http.MethodGet, "https://tenant.example/accounts/bravo/api/v2/state", nil)
	request.Header.Set("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if id, valid := auth.Authenticate(request); !valid || id != "alpha" {
		t.Fatalf("bearer authentication = %q, %v", id, valid)
	}
}

func TestTenantLoginRejectsCrossOriginAndWrongShardToken(t *testing.T) {
	auth := newTestTenantAuthenticator(t)
	for _, test := range []struct {
		name    string
		origin  string
		account string
		want    int
	}{
		{"cross origin", "https://attacker.example", "alpha", http.StatusForbidden},
		{"wrong shard", "https://tenant.example", "bravo", http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := `{"accountId":"` + test.account + `","token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
			request := httptest.NewRequest(http.MethodPost, "https://tenant.example/tenant/login", strings.NewReader(body))
			request.Host = "tenant.example"
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			auth.LoginHandler().ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, body %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestTenantSessionExpires(t *testing.T) {
	auth := newTestTenantAuthenticator(t)
	auth.sessionTTL = -time.Second
	value := auth.newSession("alpha", time.Now())
	if _, valid := auth.authenticateSession(value, time.Now()); valid {
		t.Fatal("expired tenant session authenticated")
	}
}

func newTestTenantAuthenticator(t *testing.T) *TenantAuthenticator {
	t.Helper()
	auth, err := NewTenantAuthenticator(LoadedTenantConfig{
		SessionKey: []byte(strings.Repeat("s", 32)),
		Accounts: []LoadedTenantAccount{
			{ID: "alpha", Token: strings.Repeat("a", 32)},
			{ID: "bravo", Token: strings.Repeat("b", 32)},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}
