// Package migrations embeds the SQL migration files kept at the repository
// root so the server can apply them without external assets.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
