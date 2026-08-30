package Accounts

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// DashboardOriginPolicy decides which browser origins may talk to account
// shards and the tenant login. Same-host requests are always allowed. When the
// dashboard is served by CitadelOpsFrontend from another origin, that origin
// must be allowlisted here; allowlisted cross-origin requests receive CORS
// headers (with credentials) and their preflights are answered.
type DashboardOriginPolicy struct {
	allowed map[string]struct{}
}

// NewDashboardOriginPolicy validates and normalizes absolute HTTP(S) origins.
// A nil policy (or one without entries) allows only same-host requests.
func NewDashboardOriginPolicy(origins []string) (*DashboardOriginPolicy, error) {
	policy := &DashboardOriginPolicy{allowed: map[string]struct{}{}}
	for _, raw := range origins {
		for _, candidate := range strings.Split(raw, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			normalized, err := normalizeOrigin(candidate)
			if err != nil {
				return nil, err
			}
			policy.allowed[normalized] = struct{}{}
		}
	}
	return policy, nil
}

func normalizeOrigin(candidate string) (string, error) {
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") ||
		parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return "", fmt.Errorf("dashboard origin %q must be an absolute http(s) origin without path, query, or credentials", candidate)
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host), nil
}

// Allowed reports whether the request's Origin is acceptable and whether CORS
// headers must be written for it (only for allowlisted cross-origin requests).
func (policy *DashboardOriginPolicy) Allowed(request *http.Request) (allowed bool, crossOrigin bool) {
	if sameRequestOrigin(request) {
		return true, false
	}
	if policy == nil || len(policy.allowed) == 0 {
		return false, false
	}
	normalized, err := normalizeOrigin(strings.TrimSpace(request.Header.Get("Origin")))
	if err != nil {
		return false, false
	}
	_, ok := policy.allowed[normalized]
	return ok, ok
}

// Apply writes the response headers for an allowlisted cross-origin request
// and answers a preflight. It returns true when the request was a preflight
// that has been fully answered.
func (policy *DashboardOriginPolicy) Apply(writer http.ResponseWriter, request *http.Request) (preflightAnswered bool) {
	allowed, crossOrigin := policy.Allowed(request)
	if !allowed || !crossOrigin {
		return false
	}
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	writer.Header().Set("Access-Control-Allow-Origin", origin)
	writer.Header().Set("Access-Control-Allow-Credentials", "true")
	writer.Header().Add("Vary", "Origin")
	if request.Method == http.MethodOptions && request.Header.Get("Access-Control-Request-Method") != "" {
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-Requested-With")
		writer.Header().Set("Access-Control-Max-Age", "600")
		writer.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}
