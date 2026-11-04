package database

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// Dialect names the database server the application runs on. PostgreSQL is
// the primary target; MySQL 8.4+ is supported with the same schema in
// migrations/mysql. The dialect follows from the scheme of the database URL
// (postgres:// or mysql://), so there is no separate setting to keep in sync.
type Dialect string

// The supported dialects. The values equal GORM's dialector names.
const (
	Postgres Dialect = "postgres"
	MySQL    Dialect = "mysql"
)

// ParseDialect reads the dialect from a database URL.
func ParseDialect(rawURL string) (Dialect, error) {
	scheme, _, ok := strings.Cut(rawURL, "://")
	if !ok {
		return "", fmt.Errorf("database url must start with postgres:// or mysql://")
	}
	switch strings.ToLower(scheme) {
	case "postgres", "postgresql":
		return Postgres, nil
	case "mysql":
		return MySQL, nil
	default:
		return "", fmt.Errorf("unsupported database url scheme %q (want postgres or mysql)", scheme)
	}
}

// DialectOf reports the dialect of a GORM handle.
func DialectOf(db *gorm.DB) Dialect {
	if d := db.Dialector; d != nil && d.Name() == string(MySQL) {
		return MySQL
	}
	return Postgres
}

// SQLDialect reports the dialect of a pool opened by OpenSQL.
func SQLDialect(db *sql.DB) Dialect {
	switch db.Driver().(type) {
	case *mysql.MySQLDriver, mysql.MySQLDriver:
		return MySQL
	default:
		return Postgres
	}
}

// IsMySQL reports whether db runs on MySQL.
func IsMySQL(db *gorm.DB) bool { return DialectOf(db) == MySQL }
