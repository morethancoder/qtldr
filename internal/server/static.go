package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// static serves the built UI. index.html gets the per-run token as a meta
// tag; hashed assets are cached, index.html never is.
func (s *Server) static() http.Handler {
	files := http.FileServerFS(s.opt.Assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "index.html" {
			s.serveIndex(w)
			return
		}
		if _, err := fs.Stat(s.opt.Assets, p); err != nil {
			s.serveIndex(w) // client-side routes use the hash, but be forgiving
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) serveIndex(w http.ResponseWriter) {
	b, err := fs.ReadFile(s.opt.Assets, "index.html")
	if err != nil {
		http.Error(w, "the web UI is not built; run `make web`", http.StatusInternalServerError)
		return
	}
	meta := []byte(`<meta name="qtldr-token" content="` + s.token + `"></head>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(bytes.Replace(b, []byte("</head>"), meta, 1))
}
