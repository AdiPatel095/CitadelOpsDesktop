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
	value, _ := auth.newSession("alpha", time.Now())
	if _, valid := auth.authenticateSession(value, time.Now()); valid {
		t.Fatal("expired tenant session authenticated")
	}
}

func TestDashboardBootstrapIsSingleUseAndNeverBearerAuth(t *testing.T) {
	auth := newTestTenantAuthenticator(t)
	bootstrap := strings.Repeat("x", 48)
	if err := auth.SetDashboardBootstrap("alpha", bootstrap, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	bearer := httptest.NewRequest(http.MethodGet, "https://tenant.example/accounts/alpha/api/v2/state", nil)
	bearer.Header.Set("Authorization", "Bearer "+bootstrap)
	if _, valid := auth.Authenticate(bearer); valid {
		t.Fatal("dashboard bootstrap authenticated as an API bearer")
	}

	login := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "https://tenant.example/tenant/login",
			strings.NewReader(`{"accountId":"alpha","token":"`+bootstrap+`"}`))
		request.Host = "tenant.example"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "https://tenant.example")
		response := httptest.NewRecorder()
		auth.LoginHandler().ServeHTTP(response, request)
		return response
	}
	first := login()
	if first.Code != http.StatusOK {
		t.Fatalf("first bootstrap login = %d %s", first.Code, first.Body.String())
	}
	if second := login(); second.Code != http.StatusUnauthorized {
		t.Fatalf("reused bootstrap login = %d %s", second.Code, second.Body.String())
	}
}

func TestDashboardGrantRotationKeepsShortSessionUntilRuntimeRevocation(t *testing.T) {
	auth, err := NewDynamicTenantAuthenticator([]byte(strings.Repeat("s", 32)), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.RegisterRuntime("alpha"); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetDashboardGrant("alpha", strings.Repeat("a", 32), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	session, expiresAt := auth.newSession("alpha", time.Now())
	if delta := time.Until(expiresAt); delta < 14*time.Minute || delta > 16*time.Minute {
		t.Fatalf("dynamic tenant session duration = %s", delta)
	}
	if err := auth.SetDashboardGrant("alpha", strings.Repeat("z", 32), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, valid := auth.authenticateSession(session, time.Now()); !valid {
		t.Fatal("dashboard grant rotation invalidated the short browser session")
	}
	auth.RevokeRuntime("alpha")
	if _, valid := auth.authenticateSession(session, time.Now()); valid {
		t.Fatal("revoked runtime kept accepting its browser session")
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

func TestBearerLoginReturnsSessionTokenAndSetsNoCookie(t *testing.T) {
	auth, err := NewDynamicTenantAuthenticator([]byte(strings.Repeat("s", 32)), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.RegisterRuntime("alpha"); err != nil {
		t.Fatal(err)
	}
	grant := strings.Repeat("a", 32)
	if err := auth.SetDashboardGrant("alpha", grant, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "https://tenant.example/tenant/login", strings.NewReader(`{"accountId":"alpha","token":"`+grant+`","credential":"bearer"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	auth.LoginHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) != 0 || response.Header().Get("Set-Cookie") != "" {
		t.Fatalf("bearer login = %d, cookies %v", response.Code, response.Result().Cookies())
	}
	var result struct {
		AccountID    string    `json:"accountId"`
		Path         string    `json:"path"`
		ExpiresAt    time.Time `json:"expiresAt"`
		ExpiresIn    int       `json:"expiresIn"`
		SessionToken string    `json:"sessionToken"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.AccountID != "alpha" || result.Path != "/accounts/alpha/" || !strings.HasPrefix(result.SessionToken, "v2.alpha.") || result.ExpiresIn != int(auth.sessionTTL.Seconds()) || result.ExpiresAt.IsZero() || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("invalid bearer response contract")
	}
	authenticated := httptest.NewRequest(http.MethodGet, "https://tenant.example/accounts/alpha/api/v2/state", nil)
	authenticated.Header.Set("Authorization", "Bearer "+result.SessionToken)
	if id, valid := auth.Authenticate(authenticated); !valid || id != "alpha" {
		t.Fatalf("bearer identity = %q, %t", id, valid)
	}
}

func TestCookieLoginResponseIsUnchanged(t *testing.T) {
	auth := newTestTenantAuthenticator(t)
	request := httptest.NewRequest(http.MethodPost, "https://tenant.example/tenant/login", strings.NewReader(`{"accountId":"alpha","token":"`+strings.Repeat("a", 32)+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	auth.LoginHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var result map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 || result["accountId"] != "alpha" || result["path"] != "/accounts/alpha/" || result["expiresAt"] == "" {
		t.Fatalf("cookie response = %v", result)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != tenantSessionCookie || cookie.Path != "/accounts/alpha/" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != int(auth.sessionTTL.Seconds()) {
		t.Fatal("cookie attributes changed")
	}
	authenticated := httptest.NewRequest(http.MethodGet, "https://tenant.example/accounts/alpha/", nil)
	authenticated.AddCookie(cookie)
	if id, valid := auth.Authenticate(authenticated); !valid || id != "alpha" {
		t.Fatal("cookie did not authenticate")
	}
	// Form posts ignore the JSON-only credential option and retain the redirect and cookie.
	form := httptest.NewRequest(http.MethodPost, "https://tenant.example/tenant/login", strings.NewReader("accountId=alpha&token="+strings.Repeat("a", 32)+"&credential=bearer"))
	form.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	formResponse := httptest.NewRecorder()
	auth.LoginHandler().ServeHTTP(formResponse, form)
	if formResponse.Code != http.StatusSeeOther || len(formResponse.Result().Cookies()) != 1 {
		t.Fatal("form login changed")
	}
}

func TestLoginRejectsUnknownCredentialMode(t *testing.T) {
	for _, mode := range []string{"cookie", "x"} {
		t.Run(mode, func(t *testing.T) {
			auth := newTestTenantAuthenticator(t)
			request := httptest.NewRequest(http.MethodPost, "https://tenant.example/tenant/login", strings.NewReader(`{"accountId":"alpha","token":"`+strings.Repeat("a", 32)+`","credential":"`+mode+`"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			auth.LoginHandler().ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"invalid_login"`) {
				t.Fatalf("status = %d", response.Code)
			}
		})
	}
}

func TestBearerSessionFollowsSessionRules(t *testing.T) {
	auth, err := NewDynamicTenantAuthenticator([]byte(strings.Repeat("s", 32)), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.RegisterRuntime("alpha"); err != nil {
		t.Fatal(err)
	}
	session, _ := auth.newSession("alpha", time.Now())
	check := func(token string, want bool) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "https://tenant.example/accounts/alpha/", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		request.AddCookie(&http.Cookie{Name: tenantSessionCookie, Value: session})
		id, valid := auth.Authenticate(request)
		if valid != want || (valid && id != "alpha") {
			t.Fatalf("authentication = %q, %t; want %t", id, valid, want)
		}
	}
	check(session, true)
	check(session+"tampered", false)
	auth.sessionTTL = -time.Second
	expired, _ := auth.newSession("alpha", time.Now())
	check(expired, false)
	auth.RevokeRuntime("alpha")
	check(session, false)
	if err := auth.RegisterRuntime("alpha"); err != nil {
		t.Fatal(err)
	}
	check(session, false)
	grant := "v2." + strings.Repeat("q", 29) + ".x.y.z"
	static, err := NewTenantAuthenticator(LoadedTenantConfig{SessionKey: []byte(strings.Repeat("s", 32)), Accounts: []LoadedTenantAccount{{ID: "alpha", Token: grant}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "https://tenant.example/accounts/alpha/", nil)
	request.Header.Set("Authorization", "Bearer "+grant)
	if id, valid := static.Authenticate(request); !valid || id != "alpha" {
		t.Fatal("session-shaped grant rejected")
	}
}

func TestSessionSubprotocolAuthenticatesOnlyWebSocketUpgrades(t *testing.T) {
	auth := newTestTenantAuthenticator(t)
	session, _ := auth.newSession("alpha", time.Now())
	bootstrap := strings.Repeat("x", 48)
	if err := auth.SetDashboardBootstrap("alpha", bootstrap, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, method, upgrade string
		protocols             []string
		cookie                bool
		bearer                string
		want                  bool
	}{
		{name: "valid", method: "GET", upgrade: "websocket", protocols: []string{"citadelops.v2, " + sessionSubprotocolPrefix + session}, want: true},
		{name: "case insensitive", method: "GET", upgrade: "WebSocket", protocols: []string{sessionSubprotocolPrefix + session}, want: true},
		{name: "no upgrade", method: "GET", protocols: []string{sessionSubprotocolPrefix + session}},
		{name: "post", method: "POST", upgrade: "websocket", protocols: []string{sessionSubprotocolPrefix + session}},
		{name: "duplicate", method: "GET", upgrade: "websocket", protocols: []string{sessionSubprotocolPrefix + session + ", " + sessionSubprotocolPrefix + session}},
		{name: "split valid", method: "GET", upgrade: "websocket", protocols: []string{"citadelops.v2", sessionSubprotocolPrefix + session}, want: true},
		{name: "split duplicate", method: "GET", upgrade: "websocket", protocols: []string{sessionSubprotocolPrefix + session, sessionSubprotocolPrefix + session}},
		{name: "empty", method: "GET", upgrade: "websocket", protocols: []string{sessionSubprotocolPrefix}},
		{name: "bootstrap", method: "GET", upgrade: "websocket", protocols: []string{sessionSubprotocolPrefix + bootstrap}},
		{name: "invalid blocks cookie", method: "GET", upgrade: "websocket", protocols: []string{sessionSubprotocolPrefix + "invalid"}, cookie: true},
		{name: "duplicate blocks cookie", method: "GET", upgrade: "websocket", protocols: []string{sessionSubprotocolPrefix + session, sessionSubprotocolPrefix + session}, cookie: true},
		{name: "empty blocks cookie", method: "GET", upgrade: "websocket", protocols: []string{sessionSubprotocolPrefix}, cookie: true},
		{name: "no session uses cookie", method: "GET", upgrade: "websocket", protocols: []string{"citadelops.v2"}, cookie: true, want: true},
		{name: "invalid bearer has precedence", method: "GET", upgrade: "websocket", protocols: []string{sessionSubprotocolPrefix + session}, bearer: "invalid", cookie: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "https://tenant.example/accounts/alpha/", nil)
			if test.upgrade != "" {
				request.Header.Set("Upgrade", test.upgrade)
			}
			for _, value := range test.protocols {
				request.Header.Add("Sec-WebSocket-Protocol", value)
			}
			if test.cookie {
				request.AddCookie(&http.Cookie{Name: tenantSessionCookie, Value: session})
			}
			if test.bearer != "" {
				request.Header.Set("Authorization", "Bearer "+test.bearer)
			}
			id, valid := auth.Authenticate(request)
			if valid != test.want || (valid && id != "alpha") {
				t.Fatalf("authentication = %q, %t; want %t", id, valid, test.want)
			}
		})
	}
}

func TestStripSessionSubprotocols(t *testing.T) {
	for _, test := range []struct {
		name   string
		values []string
		want   string
	}{
		{"mixed", []string{"citadelops.v2, citadelops.session.secret"}, "citadelops.v2"},
		{"session only", []string{"citadelops.session.secret"}, ""},
		{"ordered across lines", []string{"other, citadelops.session.one", "citadelops.v2, final, citadelops.session.two"}, "other, citadelops.v2, final"},
		{"absent", nil, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			header := make(http.Header)
			for _, value := range test.values {
				header.Add("Sec-WebSocket-Protocol", value)
			}
			stripSessionSubprotocols(header)
			if got := header.Get("Sec-WebSocket-Protocol"); got != test.want {
				t.Fatalf("protocols = %q, want %q", got, test.want)
			}
			if test.want == "" {
				if _, exists := header["Sec-Websocket-Protocol"]; exists {
					t.Fatal("empty header retained")
				}
			}
		})
	}
}
