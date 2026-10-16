package api

import (
	"net/http"

	"cleanpoint/web"
)

// landingPage serves the public project landing page at "/" (no token —
// static marketing content plus an unauthenticated /healthz status probe).
func (s *Server) landingPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := web.FS.ReadFile("landing.html")
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal", "landing assets missing")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	}
}

// webUI serves the embedded operator console under /app/ (same-origin API
// calls with a bearer token; no CORS surface). The embedded FS also contains
// landing.html, which the file server refuses to expose: only app assets are
// whitelisted.
func (s *Server) webUI() http.Handler {
	allowed := map[string]bool{"app.js": true, "styles.css": true, "index.html": true,
		"landing.css": true, "landing.js": true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("path")
		if !allowed[name] {
			if name == "" || name == "/" {
				name = "index.html" // /app/ serves the console shell
			} else {
				writeError(w, r, http.StatusNotFound, "not_found", "unknown console asset")
				return
			}
		}
		data, err := web.FS.ReadFile(name)
		if err != nil {
			writeError(w, r, http.StatusNotFound, "not_found", "console asset missing")
			return
		}
		switch {
		case len(name) > 3 && name[len(name)-3:] == ".js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case len(name) > 4 && name[len(name)-4:] == "html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
		case len(name) > 4 && name[len(name)-4:] == ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		}
		_, _ = w.Write(data)
	})
}
