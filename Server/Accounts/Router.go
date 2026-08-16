package Accounts

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Authenticator resolves the account identity from an authenticated session.
// The requested path never selects an account by itself.
type Authenticator interface {
	Authenticate(*http.Request) (AccountID, bool)
}

type AuthenticateFunc func(*http.Request) (AccountID, bool)

func (function AuthenticateFunc) Authenticate(request *http.Request) (AccountID, bool) {
	return function(request)
}

// Handler serves /accounts/{id}/api/* and the optional shared frontend only
// when the authenticated account exactly matches the path shard. A mismatch is
// reported as not-found so one tenant cannot enumerate another tenant.
func (supervisor *Supervisor) Handler(authenticator Authenticator, frontend http.Handler) http.Handler {
	return supervisor.HandlerWithOrigins(authenticator, frontend, nil)
}

// HandlerWithOrigins is Handler with an explicit dashboard origin policy, so a
// dashboard served by CitadelOpsFrontend from another origin can reach the
// runtime shard live (CORS with credentials, preflights answered) while every
// other origin is still rejected.
func (supervisor *Supervisor) HandlerWithOrigins(authenticator Authenticator, frontend http.Handler, origins *DashboardOriginPolicy) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedID, remainder, ok := accountPath(request.URL.Path)
		if !ok {
			http.NotFound(writer, request)
			return
		}
		if origins.Apply(writer, request) {
			return
		}
		authenticatedID, authenticated := AccountID(""), false
		if authenticator != nil {
			authenticatedID, authenticated = authenticator.Authenticate(request)
		}
		if !authenticated {
			if request.Method == http.MethodGet &&
				!strings.HasPrefix(remainder, "api/") &&
				strings.Contains(request.Header.Get("Accept"), "text/html") {
				next := request.URL.EscapedPath()
				http.Redirect(writer, request, "/tenant/login?account="+url.QueryEscape(string(requestedID))+"&next="+url.QueryEscape(next), http.StatusSeeOther)
				return
			}
			writeRouterError(writer, http.StatusUnauthorized, "authentication_required")
			return
		}
		if authenticatedID != requestedID {
			http.NotFound(writer, request)
			return
		}
		// The desktop API intentionally accepts only localhost origins. Tenant
		// origins are arbitrary hosted domains, so validate them at this account
		// boundary before forwarding the already-approved request.
		if allowed, _ := origins.Allowed(request); !allowed {
			writeRouterError(writer, http.StatusForbidden, "origin_rejected")
			return
		}
		application, exists := supervisor.Application(requestedID)
		if !exists {
			http.NotFound(writer, request)
			return
		}

		forwarded := request.Clone(request.Context())
		forwardedURL := *request.URL
		forwarded.URL = &forwardedURL
		forwarded.URL.Path = "/" + remainder
		forwarded.URL.RawPath = ""
		if strings.HasPrefix(remainder, "api/") {
			forwarded.Header.Del("Origin")
			application.API.Handler().ServeHTTP(writer, forwarded)
			return
		}
		if frontend == nil {
			http.NotFound(writer, request)
			return
		}
		frontend.ServeHTTP(writer, forwarded)
	})
}

func accountPath(path string) (AccountID, string, bool) {
	const prefix = "/accounts/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	requested, remainder, _ := strings.Cut(strings.TrimPrefix(path, prefix), "/")
	decoded, err := url.PathUnescape(requested)
	if err != nil || decoded != requested {
		return "", "", false
	}
	id, err := ParseAccountID(decoded)
	if err != nil || string(id) != decoded {
		return "", "", false
	}
	return id, remainder, true
}

func writeRouterError(writer http.ResponseWriter, status int, code string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = fmt.Fprintf(writer, `{"error":{"code":%q}}`, code)
}
