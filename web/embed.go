// Package web embeds the public landing page and the operator console
// static assets (PRD M1: static assets served from Go; no backend
// architecture change).
package web

import "embed"

//go:embed index.html landing.html styles.css app.js
var FS embed.FS
