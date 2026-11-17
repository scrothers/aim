package httpapi

import (
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Guard headers and values required on mutating API requests.
const (
	requestHeader = "X-AIM-Request"
	requestValue  = "1"
)

// hostOnly lowercases h and strips any port and IPv6 brackets.
func hostOnly(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return strings.Trim(h, "[]")
}

func allowedHostSet(extra []string) map[string]struct{} {
	set := map[string]struct{}{"localhost": {}, "127.0.0.1": {}, "::1": {}}
	if name, err := os.Hostname(); err == nil && name != "" {
		set[hostOnly(name)] = struct{}{}
	}
	for _, h := range extra {
		if h = hostOnly(h); h != "" {
			set[h] = struct{}{}
		}
	}
	return set
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

func deny(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"title":"` + http.StatusText(status) + `","status":` + strconv.Itoa(status) + `,"detail":"` + msg + `"}`))
}

// guards rejects requests whose Host header is not allowed (defeating DNS
// rebinding) and mutating /api requests that lack the JSON content type and
// custom header a cross-site form or simple fetch cannot send.
func guards(extraHosts []string) func(http.Handler) http.Handler {
	hosts := allowedHostSet(extraHosts)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := hosts[hostOnly(r.Host)]; !ok {
				deny(w, http.StatusMisdirectedRequest, "host not allowed; see --http-allowed-hosts")
				return
			}
			if isMutating(r.Method) && strings.HasPrefix(r.URL.Path, "/api/") {
				mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || mt != "application/json" {
					deny(w, http.StatusForbidden, "Content-Type must be application/json")
					return
				}
				if r.Header.Get(requestHeader) != requestValue {
					deny(w, http.StatusForbidden, "missing X-AIM-Request header")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
