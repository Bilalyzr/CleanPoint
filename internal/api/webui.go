package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"cleanpoint/web"
)

// landingPage serves the public project landing page at "/" (no token).
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

// webUI serves two asset families:
//   - /landing/{file}  -> landing page assets (landing.css, landing.js)
//   - /app/{path...}   -> the built React console (console-dist/) with an
//     SPA fallback: unknown non-file paths return index.html so client-side
//     routing works on deep links.
func (s *Server) webUI() http.Handler {
	landingAllowed := map[string]bool{"landing.css": true, "landing.js": true}
	dist, err := fs.Sub(web.Dist, "console-dist")
	if err != nil {
		panic(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/landing/") {
			name := r.PathValue("path")
			if !landingAllowed[name] {
				writeError(w, r, http.StatusNotFound, "not_found", "unknown landing asset")
				return
			}
			data, err := web.FS.ReadFile(name)
			if err != nil {
				writeError(w, r, http.StatusNotFound, "not_found", "asset missing")
				return
			}
			switch {
			case strings.HasSuffix(name, ".js"):
				w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			case strings.HasSuffix(name, ".css"):
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
			}
			_, _ = w.Write(data)
			return
		}

		// console bundle under /app/
		name := strings.TrimPrefix(r.URL.Path, "/app/")
		if name == "" {
			name = "index.html"
		}
		name = path.Clean("/"+name)[1:] // no traversal
		if name == "" {
			name = "index.html"
		}
		data, err := fs.ReadFile(dist, name)
		if err != nil {
			data, err = fs.ReadFile(dist, "index.html") // SPA fallback
			if err != nil {
				writeError(w, r, http.StatusInternalServerError, "internal", "console bundle missing")
				return
			}
			name = "index.html"
		}
		switch {
		case strings.HasSuffix(name, ".js"):
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case strings.HasSuffix(name, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(name, ".svg"):
			w.Header().Set("Content-Type", "image/svg+xml")
		case strings.HasSuffix(name, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
		}
		_, _ = w.Write(data)
	})
}
