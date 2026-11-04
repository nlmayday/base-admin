package database

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/config"
)

// MySQLCollation is the collation of every MySQL table and connection. It is
// case- and accent-sensitive, so equality, uniqueness and ordering behave as
// they do on PostgreSQL; case-insensitive matching is explicit (lower()).
const MySQLCollation = "utf8mb4_0900_as_cs"

// MySQLSQLMode makes MySQL behave like PostgreSQL where the application's
// SQL depends on it: strict mode rejects invalid or truncated values instead
// of storing something else, PIPES_AS_CONCAT makes || concatenate strings
// and ANSI_QUOTES makes "x" an identifier. Shared SQL therefore quotes
// string literals with single quotes only.
const MySQLSQLMode = "STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO," +
	"ONLY_FULL_GROUP_BY,NO_ENGINE_SUBSTITUTION,PIPES_AS_CONCAT,ANSI_QUOTES"

const mysqlDefaultPort = "3306"

// mysqlConfig turns mysql://user:pass@host:port/db?params into a driver
// config. Sessions run in UTC so that CURRENT_TIMESTAMP and DATETIME
// columns hold UTC, matching timestamptz on PostgreSQL. Query parameters
// are passed to the driver (for example tls=true).
func mysqlConfig(cfg config.DatabaseConfig, appName string, multiStatements bool) (*mysql.Config, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		// url.Parse errors quote the input; never echo the password.
		return nil, errors.New("parse database url: malformed mysql url")
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" || strings.Contains(dbName, "/") {
		return nil, errors.New("parse database url: the mysql url must name one database, as mysql://user:pass@host:3306/name")
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), mysqlDefaultPort)
	}

	for _, k := range []string{"parseTime", "loc", "collation", "charset", "multiStatements", "time_zone", "sql_mode", "transaction_isolation", "clientFoundRows"} {
		if u.Query().Has(k) {
			return nil, fmt.Errorf("parse database url: mysql parameter %s is set by the application and must not be given", k)
		}
	}
	// The driver parses its own options from the query; credentials are set
	// afterwards so that no character in a password can confuse the DSN.
	mc, err := mysql.ParseDSN("tcp(" + host + ")/" + dbName + "?" + u.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("parse database url: mysql parameters: %w", err)
	}
	mc.User = u.User.Username()
	mc.Passwd, _ = u.User.Password()
	if mc.Params == nil {
		mc.Params = map[string]string{}
	}
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Collation = MySQLCollation
	mc.Timeout = cfg.ConnectTimeout
	mc.MultiStatements = multiStatements
	mc.Params["time_zone"] = "'+00:00'"
	// PostgreSQL's isolation level: a statement sees rows committed before
	// it started, which the lock-then-read patterns (LockTx, FOR UPDATE)
	// rely on. MySQL's REPEATABLE READ would keep the snapshot of the first
	// read in the transaction.
	mc.Params["transaction_isolation"] = "'READ-COMMITTED'"
	// RowsAffected counts matched rows, as on PostgreSQL, not only rows
	// whose values changed.
	mc.ClientFoundRows = true
	mc.Params["sql_mode"] = "'" + MySQLSQLMode + "'"
	if cfg.StatementTimeout > 0 {
		// max_execution_time bounds read-only SELECTs only.
		mc.Params["max_execution_time"] = strconv.FormatInt(cfg.StatementTimeout.Milliseconds(), 10)
	}
	mc.ConnectionAttributes = "program_name:" + appName
	return mc, nil
}

// mysqlDialector keeps the driver error inside GORM's translated errors:
// the stock translator replaces *mysql.MySQLError with gorm.ErrDuplicatedKey
// and loses the index name that UniqueConstraint reports.
type mysqlDialector struct{ gormmysql.Dialector }

func (d mysqlDialector) Translate(err error) error {
	translated := d.Dialector.Translate(err)
	if translated == err { //nolint:errorlint // identity check: was the error translated?
		return err
	}
	return fmt.Errorf("%w: %w", translated, err)
}

var _ gorm.ErrorTranslator = mysqlDialector{}
