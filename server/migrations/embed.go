// Package migrations embeds the versioned SQL schema migrations so that the
// binary can apply them without access to the source tree.
//
// Files follow golang-migrate naming: NNNNNN_name.up.sql / .down.sql.
package migrations

import "embed"

// FS holds every migration file.
//
//go:embed *.sql
var FS embed.FS
