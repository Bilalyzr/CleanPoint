package api

import (
	"io/fs"
	"net/http"

	"cleanpoint/web"
)

// webUI serves the embedded operator console (PRD M1: static assets served
// from Go; the SPA talks to the same-origin /api/v1 endpoints with a bearer
// token, so no CORS surface is introduced).
func (s *Server) webUI() http.Handler {
	sub, err := fs.Sub(web.FS, ".")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/app/", http.FileServer(http.FS(sub)))
}

func (s *Server) webIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := web.FS.ReadFile("index.html")
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal", "console assets missing")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	}
}
