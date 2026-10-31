// Package migrations embeds the versioned SQL schema migrations so that the
// binary can apply them without access to the source tree.
//
// Files follow golang-migrate naming: NNNNNN_name.up.sql / .down.sql. The
// files in this directory are the scaffold's own migrations; app/ holds the
// application's, numbered independently so that merging scaffold updates
// never collides with them.
package migrations

import (
	"embed"
	"io/fs"
)

// FS holds the scaffold migrations.
//
//go:embed *.sql
var FS embed.FS

//go:embed app
var appFiles embed.FS

// App holds the application migrations.
var App = mustSub(appFiles, "app")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
