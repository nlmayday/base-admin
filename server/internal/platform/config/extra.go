package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ExtraDatabaseConfig is another database the business modules use, such as
// a game database, declared as database.extra.<name>. Its URL is a secret
// and comes from ADMIN_DATABASE_EXTRA_<NAME>_URL (postgres:// or mysql://);
// setting that variable alone declares the database with default settings.
// The connection keeps the server's session defaults: none of the settings
// of the main database (UTC, sql_mode, isolation level) are applied.
type ExtraDatabaseConfig struct {
	URL             string        `mapstructure:"url"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	ConnectTimeout  time.Duration `mapstructure:"connect_timeout"`
	// Required makes startup and /health/ready fail while the database is
	// unreachable. Otherwise the API starts without it and the business
	// module sees the errors.
	Required bool `mapstructure:"required"`
}

// Defaults of an extra database; zero values in the file take these.
var extraDefaults = ExtraDatabaseConfig{
	MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: 30 * time.Minute,
	ConnMaxIdleTime: 5 * time.Minute, ConnectTimeout: 5 * time.Second,
}

var (
	extraNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)
	extraEnvPattern  = regexp.MustCompile(`^` + envPrefix + `_DATABASE_EXTRA_([A-Z][A-Z0-9_]*)_URL$`)
)

// ExtraURLEnv is the environment variable that holds the URL of extra
// database name.
func ExtraURLEnv(name string) string {
	return envPrefix + "_DATABASE_EXTRA_" + strings.ToUpper(name) + "_URL"
}

// withExtraEnv applies the ADMIN_DATABASE_EXTRA_<NAME>_URL variables and the
// defaults to the extra databases.
func withExtraEnv(extra map[string]ExtraDatabaseConfig) map[string]ExtraDatabaseConfig {
	out := make(map[string]ExtraDatabaseConfig, len(extra))
	for name, c := range extra {
		out[name] = c
	}
	for _, kv := range os.Environ() {
		key, value, _ := strings.Cut(kv, "=")
		if m := extraEnvPattern.FindStringSubmatch(key); m != nil {
			name := strings.ToLower(m[1])
			c := out[name]
			c.URL = value
			out[name] = c
		}
	}
	for name, c := range out {
		if c.MaxOpenConns == 0 {
			c.MaxOpenConns = extraDefaults.MaxOpenConns
		}
		if c.MaxIdleConns == 0 {
			c.MaxIdleConns = min(extraDefaults.MaxIdleConns, c.MaxOpenConns)
		}
		if c.ConnMaxLifetime == 0 {
			c.ConnMaxLifetime = extraDefaults.ConnMaxLifetime
		}
		if c.ConnMaxIdleTime == 0 {
			c.ConnMaxIdleTime = extraDefaults.ConnMaxIdleTime
		}
		if c.ConnectTimeout == 0 {
			c.ConnectTimeout = extraDefaults.ConnectTimeout
		}
		out[name] = c
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validateExtra(extra map[string]ExtraDatabaseConfig) error {
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	var errs []error
	for _, name := range names {
		c := extra[name]
		key := "database.extra." + name
		switch {
		case !extraNamePattern.MatchString(name):
			errs = append(errs, fmt.Errorf("%s: the name must be lower-case letters, digits or _ (at most 31)", key))
			continue
		case c.URL == "":
			errs = append(errs, fmt.Errorf("%s.url is required (set %s)", key, ExtraURLEnv(name)))
		case !strings.HasPrefix(c.URL, "postgres://") && !strings.HasPrefix(c.URL, "postgresql://") && !strings.HasPrefix(c.URL, "mysql://"):
			errs = append(errs, fmt.Errorf("%s.url must start with postgres:// or mysql://", key))
		}
		if c.MaxOpenConns <= 0 {
			errs = append(errs, fmt.Errorf("%s.max_open_conns must be positive", key))
		}
		if c.MaxIdleConns < 0 || c.MaxIdleConns > c.MaxOpenConns {
			errs = append(errs, fmt.Errorf("%s.max_idle_conns must be between 0 and max_open_conns", key))
		}
		if c.ConnMaxLifetime < 0 || c.ConnMaxIdleTime < 0 || c.ConnectTimeout < 0 {
			errs = append(errs, fmt.Errorf("%s: durations must not be negative", key))
		}
	}
	return errors.Join(errs...)
}
