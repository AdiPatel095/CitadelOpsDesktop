package Accounts

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tenantSessionCookie              = "citadelops_tenant_session"
	dynamicTenantSessionTTL          = 15 * time.Minute
	maximumActiveDashboardBootstraps = 256
)

type TenantAuthenticator struct {
	mu            sync.RWMutex
	credentials   map[AccountID]tenantCredential
	bootstraps    map[[sha256.Size]byte]dashboardBootstrap
	generations   map[AccountID]uint64
	sessionKey    []byte
	secureCookies bool
	sessionTTL    time.Duration
	origins       *DashboardOriginPolicy
}

// SetDashboardOrigins installs the origin policy consulted by the login
// handler; nil keeps same-host only.
func (auth *TenantAuthenticator) SetDashboardOrigins(policy *DashboardOriginPolicy) {
	if auth == nil {
		return
	}
	auth.mu.Lock()
	auth.origins = policy
	auth.mu.Unlock()
}

func (auth *TenantAuthenticator) dashboardOrigins() *DashboardOriginPolicy {
	if auth == nil {
		return nil
	}
	auth.mu.RLock()
	defer auth.mu.RUnlock()
	return auth.origins
}

type tenantCredential struct {
	tokenHash      [sha256.Size]byte
	hasToken       bool
	generation     uint64
	grantExpiresAt time.Time
}

// dashboardBootstrap is a browser-facing, one-use bridge into an HttpOnly
// tenant session. Only its digest is retained, and it is never accepted as an
// API bearer credential.
type dashboardBootstrap struct {
	runtimeID AccountID
	expiresAt time.Time
}

func NewTenantAuthenticator(config LoadedTenantConfig, secureCookies bool) (*TenantAuthenticator, error) {
	if len(config.Accounts) == 0 {
		return nil, fmt.Errorf("tenant authenticator needs at least one account")
	}
	auth, err := NewDynamicTenantAuthenticator(config.SessionKey, secureCookies)
	if err != nil {
		return nil, err
	}
	// Static hosted installs keep their historical operator-login duration.
	// Dynamic cells use the shorter session set by NewDynamicTenantAuthenticator.
	auth.sessionTTL = defaultTenantSessionHours * time.Hour
	seen := make(map[AccountID]struct{}, len(config.Accounts))
	for _, account := range config.Accounts {
		if account.ID == "" || account.Token == "" {
			return nil, fmt.Errorf("tenant account credentials are incomplete")
		}
		if _, exists := seen[account.ID]; exists {
			return nil, fmt.Errorf("tenant account %q has duplicate credentials", account.ID)
		}
		if err := auth.RegisterRuntime(account.ID); err != nil {
			return nil, err
		}
		if err := auth.SetDashboardGrant(account.ID, account.Token, time.Time{}); err != nil {
			return nil, err
		}
		seen[account.ID] = struct{}{}
	}
	return auth, nil
}

// NewDynamicTenantAuthenticator creates an empty runtime credential registry.
// Hosted orchestration registers each runtime only after its private
// application boundary exists, then installs a dashboard grant by hash.
func NewDynamicTenantAuthenticator(sessionKey []byte, secureCookies bool) (*TenantAuthenticator, error) {
	key := append([]byte(nil), sessionKey...)
	if len(key) == 0 {
		key = make([]byte, sha256.Size)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("create tenant session key: %w", err)
		}
	}
	if len(key) < sha256.Size {
		return nil, fmt.Errorf("tenant session key must contain at least %d bytes", sha256.Size)
	}
	return &TenantAuthenticator{
		credentials: map[AccountID]tenantCredential{},
		bootstraps:  map[[sha256.Size]byte]dashboardBootstrap{},
		generations: map[AccountID]uint64{},
		sessionKey:  key, secureCookies: secureCookies,
		sessionTTL: dynamicTenantSessionTTL,
	}, nil
}

func (auth *TenantAuthenticator) RegisterRuntime(id AccountID) error {
	if auth == nil {
		return fmt.Errorf("tenant authenticator is unavailable")
	}
	parsed, err := ParseAccountID(string(id))
	if err != nil {
		return err
	}
	if parsed != id {
		return fmt.Errorf("runtime id must be canonical")
	}
	auth.mu.Lock()
	defer auth.mu.Unlock()
	if _, exists := auth.credentials[id]; exists {
		return nil
	}
	generation := auth.generations[id] + 1
	if generation == 0 {
		generation = 1
	}
	auth.generations[id] = generation
	auth.credentials[id] = tenantCredential{generation: generation}
	return nil
}

// SetDashboardGrant replaces the runtime's connection token without retaining
// the secret. Grant rotation does not invalidate a short-lived browser session;
// removing the runtime still does so immediately.
func (auth *TenantAuthenticator) SetDashboardGrant(id AccountID, token string, expiresAt time.Time) error {
	if auth == nil {
		return fmt.Errorf("tenant authenticator is unavailable")
	}
	if len(token) < minimumTenantTokenLength {
		return fmt.Errorf("dashboard grant must contain at least %d characters", minimumTenantTokenLength)
	}
	if !expiresAt.IsZero() && !expiresAt.After(time.Now()) {
		return fmt.Errorf("dashboard grant expiration must be in the future")
	}
	digest := sha256.Sum256([]byte(token))
	auth.mu.Lock()
	defer auth.mu.Unlock()
	credential, exists := auth.credentials[id]
	if !exists {
		return fmt.Errorf("runtime %q is not registered", id)
	}
	for otherID, other := range auth.credentials {
		if otherID != id && other.hasToken && subtle.ConstantTimeCompare(digest[:], other.tokenHash[:]) == 1 {
			return fmt.Errorf("dashboard grant is already bound to another runtime")
		}
	}
	for bootstrapDigest := range auth.bootstraps {
		if subtle.ConstantTimeCompare(digest[:], bootstrapDigest[:]) == 1 {
			return fmt.Errorf("dashboard grant conflicts with an active bootstrap")
		}
	}
	if credential.hasToken && subtle.ConstantTimeCompare(digest[:], credential.tokenHash[:]) == 1 {
		credential.grantExpiresAt = expiresAt.UTC()
		auth.credentials[id] = credential
		return nil
	}
	credential.tokenHash = digest
	credential.hasToken = true
	credential.grantExpiresAt = expiresAt.UTC()
	auth.credentials[id] = credential
	return nil
}

// SetDashboardBootstrap installs one short-lived, single-use login token for
// an exact runtime. The raw token is discarded after hashing.
func (auth *TenantAuthenticator) SetDashboardBootstrap(id AccountID, token string, expiresAt time.Time) error {
	if auth == nil {
		return fmt.Errorf("tenant authenticator is unavailable")
	}
	if len(token) < minimumTenantTokenLength || strings.TrimSpace(token) != token {
		return fmt.Errorf("dashboard bootstrap must contain at least %d characters", minimumTenantTokenLength)
	}
	now := time.Now().UTC()
	expiresAt = expiresAt.UTC()
	if !expiresAt.After(now) {
		return fmt.Errorf("dashboard bootstrap expiration must be in the future")
	}
	digest := sha256.Sum256([]byte(token))
	auth.mu.Lock()
	defer auth.mu.Unlock()
	if _, exists := auth.credentials[id]; !exists {
		return fmt.Errorf("runtime %q is not registered", id)
	}
	for _, credential := range auth.credentials {
		if credential.hasToken && subtle.ConstantTimeCompare(digest[:], credential.tokenHash[:]) == 1 {
			return fmt.Errorf("dashboard bootstrap conflicts with a dashboard grant")
		}
	}
	for candidate, bootstrap := range auth.bootstraps {
		if !bootstrap.expiresAt.After(now) {
			delete(auth.bootstraps, candidate)
			continue
		}
		if subtle.ConstantTimeCompare(digest[:], candidate[:]) == 1 {
			return fmt.Errorf("dashboard bootstrap is already active")
		}
	}
	if len(auth.bootstraps) >= maximumActiveDashboardBootstraps {
		return fmt.Errorf("too many active dashboard bootstraps")
	}
	auth.bootstraps[digest] = dashboardBootstrap{runtimeID: id, expiresAt: expiresAt}
	return nil
}

// hasDashboardGrantDigest reports whether a credential is already reserved for
// dashboard access, including a one-use bootstrap. Callers pass a digest so
// the raw token never crosses the authenticator boundary.
func (auth *TenantAuthenticator) hasDashboardGrantDigest(candidate [sha256.Size]byte) bool {
	if auth == nil {
		return false
	}
	auth.mu.RLock()
	defer auth.mu.RUnlock()
	for _, credential := range auth.credentials {
		if credential.hasToken && subtle.ConstantTimeCompare(candidate[:], credential.tokenHash[:]) == 1 {
			return true
		}
	}
	for digest, bootstrap := range auth.bootstraps {
		if bootstrap.expiresAt.After(time.Now()) && subtle.ConstantTimeCompare(candidate[:], digest[:]) == 1 {
			return true
		}
	}
	return false
}

func (auth *TenantAuthenticator) RevokeRuntime(id AccountID) {
	if auth == nil {
		return
	}
	auth.mu.Lock()
	delete(auth.credentials, id)
	for digest, bootstrap := range auth.bootstraps {
		if bootstrap.runtimeID == id {
			delete(auth.bootstraps, digest)
		}
	}
	auth.mu.Unlock()
}

func (auth *TenantAuthenticator) Authenticate(request *http.Request) (AccountID, bool) {
	if auth == nil || request == nil {
		return "", false
	}
	if authorization := strings.TrimSpace(request.Header.Get("Authorization")); authorization != "" {
		const prefix = "Bearer "
		if len(authorization) > len(prefix) && strings.EqualFold(authorization[:len(prefix)], prefix) {
			return auth.authenticateToken(authorization[len(prefix):])
		}
	}
	for _, cookie := range request.Cookies() {
		if cookie.Name == tenantSessionCookie {
			if id, valid := auth.authenticateSession(cookie.Value, time.Now()); valid {
				return id, true
			}
		}
	}
	return "", false
}

func (auth *TenantAuthenticator) authenticateToken(token string) (AccountID, bool) {
	candidate := sha256.Sum256([]byte(token))
	var matched AccountID
	matches := 0
	now := time.Now()
	// Scan the bounded process group instead of indexing by caller-controlled
	// account ID so bearer authentication does not trust the route shard.
	auth.mu.RLock()
	defer auth.mu.RUnlock()
	for id, credential := range auth.credentials {
		if credential.hasToken &&
			(credential.grantExpiresAt.IsZero() || now.Before(credential.grantExpiresAt)) &&
			subtle.ConstantTimeCompare(candidate[:], credential.tokenHash[:]) == 1 {
			matched = id
			matches++
		}
	}
	return matched, matches == 1
}

// authenticateLoginToken accepts either the runtime's operator/dashboard
// grant or a one-use browser bootstrap. Bootstraps are atomically consumed and
// deliberately remain outside Authenticate, so they cannot call shard APIs.
func (auth *TenantAuthenticator) authenticateLoginToken(id AccountID, token string, now time.Time) bool {
	candidate := sha256.Sum256([]byte(token))
	auth.mu.Lock()
	defer auth.mu.Unlock()
	credential, exists := auth.credentials[id]
	if !exists {
		return false
	}
	if credential.hasToken &&
		(credential.grantExpiresAt.IsZero() || now.Before(credential.grantExpiresAt)) &&
		subtle.ConstantTimeCompare(candidate[:], credential.tokenHash[:]) == 1 {
		return true
	}
	matched := false
	for digest, bootstrap := range auth.bootstraps {
		if !bootstrap.expiresAt.After(now) {
			delete(auth.bootstraps, digest)
			continue
		}
		if bootstrap.runtimeID == id && subtle.ConstantTimeCompare(candidate[:], digest[:]) == 1 {
			delete(auth.bootstraps, digest)
			matched = true
		}
	}
	return matched
}

func (auth *TenantAuthenticator) authenticateSession(value string, now time.Time) (AccountID, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 5 || parts[0] != "v2" {
		return "", false
	}
	id, err := ParseAccountID(parts[1])
	if err != nil || string(id) != parts[1] {
		return "", false
	}
	generation, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil || generation == 0 {
		return "", false
	}
	expiresUnix, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || expiresUnix <= now.Unix() {
		return "", false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil {
		return "", false
	}
	payload := strings.Join(parts[:4], ".")
	expected := auth.sign(payload)
	if !hmac.Equal(signature, expected) {
		return "", false
	}
	auth.mu.RLock()
	credential, exists := auth.credentials[id]
	auth.mu.RUnlock()
	if !exists || credential.generation != generation {
		return "", false
	}
	return id, true
}

func (auth *TenantAuthenticator) newSession(id AccountID, now time.Time) (string, time.Time) {
	auth.mu.RLock()
	credential, exists := auth.credentials[id]
	auth.mu.RUnlock()
	if !exists {
		return "", time.Time{}
	}
	expiresAt := now.Add(auth.sessionTTL).UTC()
	payload := "v2." + string(id) + "." + strconv.FormatUint(credential.generation, 10) + "." + strconv.FormatInt(expiresAt.Unix(), 10)
	return payload + "." + base64.RawURLEncoding.EncodeToString(auth.sign(payload)), expiresAt
}

func (auth *TenantAuthenticator) sign(payload string) []byte {
	digest := hmac.New(sha256.New, auth.sessionKey)
	_, _ = digest.Write([]byte(payload))
	return digest.Sum(nil)
}

func (auth *TenantAuthenticator) LoginHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setTenantSecurityHeaders(writer)
		if auth.dashboardOrigins().Apply(writer, request) {
			return
		}
		switch request.Method {
		case http.MethodGet:
			auth.renderLogin(writer, request)
		case http.MethodPost:
			auth.login(writer, request)
		default:
			writer.Header().Set("Allow", "GET, POST")
			writeRouterError(writer, http.StatusMethodNotAllowed, "method_not_allowed")
		}
	})
}

func (auth *TenantAuthenticator) renderLogin(writer http.ResponseWriter, request *http.Request) {
	account := strings.TrimSpace(request.URL.Query().Get("account"))
	if parsed, err := ParseAccountID(account); err == nil {
		account = string(parsed)
	} else {
		account = ""
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(writer, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>CitadelOps Tenant</title><style>body{font:16px system-ui;background:#111827;color:#f9fafb;margin:0;display:grid;min-height:100vh;place-items:center}form{width:min(24rem,calc(100%% - 2rem));display:grid;gap:1rem;padding:2rem;border:1px solid #374151;border-radius:1rem;background:#1f2937}h1{margin:0}label{display:grid;gap:.4rem}input,button{font:inherit;padding:.75rem;border-radius:.5rem;border:1px solid #4b5563}button{cursor:pointer;background:#2563eb;color:white;border:0}</style></head><body><form method="post" action="/tenant/login"><h1>CitadelOps</h1><label>Account shard<input name="accountId" autocomplete="username" required value="%s"></label><label>Access token<input type="password" name="token" autocomplete="current-password" required></label><button type="submit">Open tenant</button></form></body></html>`, html.EscapeString(account))
}

func (auth *TenantAuthenticator) login(writer http.ResponseWriter, request *http.Request) {
	if allowed, _ := auth.dashboardOrigins().Allowed(request); !allowed {
		writeRouterError(writer, http.StatusForbidden, "origin_rejected")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 4096)
	accountValue, token, next, isJSON, err := readTenantLogin(request)
	if err != nil {
		writeRouterError(writer, http.StatusBadRequest, "invalid_login")
		return
	}
	id, parseErr := ParseAccountID(accountValue)
	now := time.Now().UTC()
	if parseErr != nil || !auth.authenticateLoginToken(id, token, now) {
		// Keep account existence and credential failures indistinguishable.
		writeRouterError(writer, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	path := safeTenantRedirect(id, next)
	session, expiresAt := auth.newSession(id, now)
	if session == "" {
		writeRouterError(writer, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: tenantSessionCookie, Value: session,
		Path: "/accounts/" + string(id) + "/", MaxAge: int(auth.sessionTTL.Seconds()),
		HttpOnly: true, Secure: auth.secureCookies, SameSite: http.SameSiteStrictMode,
	})
	if !isJSON {
		http.Redirect(writer, request, path, http.StatusSeeOther)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"accountId": string(id), "path": path, "expiresAt": expiresAt})
}

func readTenantLogin(request *http.Request) (account string, token string, next string, isJSON bool, err error) {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0]))
	if mediaType == "application/json" {
		var input struct {
			AccountID string `json:"accountId"`
			Token     string `json:"token"`
			Next      string `json:"next,omitempty"`
		}
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if decodeErr := decoder.Decode(&input); decodeErr != nil {
			return "", "", "", true, decodeErr
		}
		if eofErr := requireJSONEOF(decoder); eofErr != nil {
			return "", "", "", true, eofErr
		}
		return input.AccountID, input.Token, input.Next, true, nil
	}
	if parseErr := request.ParseForm(); parseErr != nil {
		return "", "", "", false, parseErr
	}
	return request.Form.Get("accountId"), request.Form.Get("token"), request.Form.Get("next"), false, nil
}

func safeTenantRedirect(id AccountID, candidate string) string {
	fallback := "/accounts/" + string(id) + "/"
	parsed, err := url.Parse(strings.TrimSpace(candidate))
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fallback
	}
	path := parsed.EscapedPath()
	prefix := "/accounts/" + string(id) + "/"
	if !strings.HasPrefix(path, prefix) || strings.Contains(path, "\\") || strings.HasPrefix(path, "//") {
		return fallback
	}
	return path
}

func sameRequestOrigin(request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host != "" && strings.EqualFold(parsed.Host, request.Host)
}

func setTenantSecurityHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("X-Frame-Options", "DENY")
}
