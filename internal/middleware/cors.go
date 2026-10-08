package middleware

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type corsMatcher struct {
	allowAll         bool
	exactOrigins     map[string]struct{}
	exactHosts       map[string]struct{}
	wildcardSuffixes []string
}

func newCORSMatcher(allowed []string) *corsMatcher {
	m := &corsMatcher{
		exactOrigins: make(map[string]struct{}),
		exactHosts:   make(map[string]struct{}),
	}

	for _, item := range allowed {
		clean := strings.ToLower(strings.TrimSpace(item))
		if clean == "" {
			continue
		}
		if clean == "*" {
			m.allowAll = true
			continue
		}

		if strings.Contains(clean, "://") {
			m.exactOrigins[clean] = struct{}{}
			if u, err := url.Parse(clean); err == nil && u.Host != "" {
				m.exactHosts[u.Host] = struct{}{}
			}
		} else {
			m.exactHosts[clean] = struct{}{}
			if strings.HasPrefix(clean, ".") {
				m.wildcardSuffixes = append(m.wildcardSuffixes, clean)
			} else {
				m.wildcardSuffixes = append(m.wildcardSuffixes, "."+clean)
			}
		}
	}

	return m
}

func (m *corsMatcher) isAllowed(raw string) bool {
	if raw == "" || m.allowAll {
		return true
	}

	u, err := url.Parse(raw)
	var originStr, hostStr string
	if err == nil && u.Host != "" {
		hostStr = strings.ToLower(u.Host)
		originStr = strings.ToLower(fmt.Sprintf("%s://%s", u.Scheme, u.Host))
	} else {
		hostStr = strings.ToLower(raw)
		originStr = strings.ToLower(raw)
	}

	if _, ok := m.exactOrigins[originStr]; ok {
		return true
	}
	if _, ok := m.exactHosts[hostStr]; ok {
		return true
	}

	hostOnly := hostStr
	if colonIdx := strings.IndexByte(hostOnly, ':'); colonIdx != -1 {
		hostOnly = hostOnly[:colonIdx]
	}
	if _, ok := m.exactHosts[hostOnly]; ok {
		return true
	}

	for _, suffix := range m.wildcardSuffixes {
		if strings.HasSuffix(hostOnly, suffix) {
			return true
		}
	}

	return false
}

// CORS validates Origin and Referer against allowed domains/origins,
// sets standard CORS response headers, and handles preflight OPTIONS requests.
func CORS(allowedDomains []string) func(http.Handler) http.Handler {
	matcher := newCORSMatcher(allowedDomains)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			referer := r.Header.Get("Referer")

			// Check Origin header if present
			if origin != "" {
				if !matcher.isAllowed(origin) {
					writeError(w, http.StatusForbidden, fmt.Sprintf("origin not allowed: %s", origin), "FORBIDDEN_ORIGIN")
					return
				}
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			} else if referer != "" {
				// If Origin is not sent (e.g. <img> tags), validate Referer domain
				if !matcher.isAllowed(referer) {
					writeError(w, http.StatusForbidden, "referer domain not allowed", "FORBIDDEN_REFERER")
					return
				}
			}

			// Handle preflight OPTIONS request
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "X-API-KEY, Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
