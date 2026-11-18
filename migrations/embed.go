// Package migrations embeds the SQLite schema used by the server.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
