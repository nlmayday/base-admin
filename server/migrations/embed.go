// Package migrations embeds the versioned SQL schema migrations so that the
// binary can apply them without access to the source tree.
//
// Files follow golang-migrate naming: NNNNNN_name.up.sql / .down.sql. The
// files in this directory are the scaffold's own migrations; app/ holds the
// application's, numbered independently so that merging scaffold updates
// never collides with them.
//
// Both are written for PostgreSQL. The mysql/ subdirectory of each holds the
// MySQL version of the same migrations, with the same numbers: the scaffold
// keeps both in step, an application maintains the dialects it deploys on.
package migrations

import (
	"embed"
	"io/fs"
)

// FS holds the scaffold migrations for PostgreSQL.
//
//go:embed *.sql
var FS embed.FS

//go:embed mysql/*.sql
var mysqlFiles embed.FS

// MySQL holds the scaffold migrations for MySQL.
var MySQL = mustSub(mysqlFiles, "mysql")

//go:embed app
var appFiles embed.FS

// App holds the application migrations for PostgreSQL.
var App = mustSub(appFiles, "app")

// AppMySQL holds the application migrations for MySQL.
var AppMySQL = mustSub(appFiles, "app/mysql")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
