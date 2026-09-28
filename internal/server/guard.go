package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
)

// guard rejects requests whose Host or Origin is not this server (DNS
// rebinding, cross-site requests) and mutating requests without the token.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.allowedHost(r.Host) {
			http.Error(w, "unexpected Host header; open qtldr at the URL `qtldr serve` printed", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !s.allowedOrigin(origin) {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
		if mutating(r.Method) && subtle.ConstantTimeCompare([]byte(r.Header.Get(TokenHeader)), []byte(s.token)) != 1 {
			http.Error(w, "missing or wrong "+TokenHeader+"; reload the page", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func mutating(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

// allowedHost: 127.0.0.1:<port> or localhost:<port> of the bound address.
func (s *Server) allowedHost(host string) bool {
	addr, _ := s.addr.Load().(string)
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return host == "127.0.0.1:"+port || host == "localhost:"+port
}

func (s *Server) allowedOrigin(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Scheme == "http" && s.allowedHost(u.Host)
}
