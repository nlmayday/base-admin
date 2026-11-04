// Package config defines, loads and validates the application configuration.
//
// Values come from built-in defaults, an optional YAML file and ADMIN_*
// environment variables, in increasing order of precedence. Secrets may only
// be supplied through environment variables.
package config

import "time"

// Env is the deployment environment.
type Env string

// Supported environments.
const (
	EnvDev  Env = "dev"
	EnvTest Env = "test"
	EnvProd Env = "prod"
)

// Config is the root configuration.
type Config struct {
	App       AppConfig       `mapstructure:"app"`
	HTTP      HTTPConfig      `mapstructure:"http"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Log       LogConfig       `mapstructure:"log"`
	Metrics   MetricsConfig   `mapstructure:"metrics"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Bootstrap BootstrapConfig `mapstructure:"bootstrap"`
	File      FileConfig      `mapstructure:"file"`
	// Biz is the business modules' own configuration (biz.go).
	Biz Biz `mapstructure:"biz"`
}

// AuthConfig holds session and login protection settings (design doc 6).
type AuthConfig struct {
	// CookieName should be "__Host-sid" in production; that prefix requires
	// CookieSecure.
	CookieName   string `mapstructure:"cookie_name"`
	CookieSecure bool   `mapstructure:"cookie_secure"`
	// AllowedOrigins are the browser origins (scheme://host[:port]) allowed
	// to send state-changing requests.
	AllowedOrigins  []string      `mapstructure:"allowed_origins"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	AbsoluteTimeout time.Duration `mapstructure:"absolute_timeout"`
	// TouchInterval limits how often sliding expiry is written back.
	TouchInterval time.Duration `mapstructure:"touch_interval"`
	SnapshotTTL   time.Duration `mapstructure:"snapshot_ttl"`
	// ReconcileInterval is how often PG and Redis auth versions are compared.
	ReconcileInterval time.Duration `mapstructure:"reconcile_interval"`
	// LockTimeout bounds waiting for a user row lock in login/revocation.
	LockTimeout        time.Duration `mapstructure:"lock_timeout"`
	LoginWindow        time.Duration `mapstructure:"login_window"`
	LoginMaxPerIP      int           `mapstructure:"login_max_per_ip"`
	LoginMaxPerAccount int           `mapstructure:"login_max_per_account"`
}

// BootstrapConfig creates the first super administrator when no user exists.
type BootstrapConfig struct {
	AdminUsername string `mapstructure:"admin_username"`
	// AdminPassword is a secret and must come from ADMIN_BOOTSTRAP_ADMIN_PASSWORD.
	AdminPassword string `mapstructure:"admin_password"`
}

// AppConfig holds application identity settings.
type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  Env    `mapstructure:"env"`
}

// HTTPConfig holds API server settings.
type HTTPConfig struct {
	Addr              string        `mapstructure:"addr"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	// RequestTimeout is the context deadline given to each request handler.
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
	MaxBodyBytes   int64         `mapstructure:"max_body_bytes"`
	// DrainDelay is how long /health/ready reports draining before the
	// server stops accepting connections, so load balancers can react.
	DrainDelay      time.Duration `mapstructure:"drain_delay"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	// TrustedProxies lists proxy IPs or CIDRs allowed to set client IP
	// headers. Empty means no proxy is trusted.
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

// DatabaseConfig holds the settings of the main database (PostgreSQL or
// MySQL) and of the extra databases of the business modules.
type DatabaseConfig struct {
	// URL is a secret and must come from ADMIN_DATABASE_URL.
	URL             string        `mapstructure:"url"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	ConnectTimeout  time.Duration `mapstructure:"connect_timeout"`
	// StatementTimeout is applied per session; zero disables it.
	StatementTimeout   time.Duration `mapstructure:"statement_timeout"`
	SlowQueryThreshold time.Duration `mapstructure:"slow_query_threshold"`
	// Extra are other databases of the business modules, by name
	// (extra.go).
	Extra map[string]ExtraDatabaseConfig `mapstructure:"extra"`
}

// RedisConfig holds Redis settings.
type RedisConfig struct {
	Addr string `mapstructure:"addr"`
	// Password is a secret and must come from ADMIN_REDIS_PASSWORD.
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	// RequireSafeEvictionPolicy rejects allkeys-* eviction policies at
	// startup (design doc 6.5.6).
	RequireSafeEvictionPolicy bool `mapstructure:"require_safe_eviction_policy"`
	// DeclaredEvictionPolicy must be set when the server disables CONFIG
	// (common on managed Redis), so the policy can still be checked.
	DeclaredEvictionPolicy string `mapstructure:"declared_eviction_policy"`
}

// LogConfig holds logger settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// MetricsConfig holds Prometheus endpoint settings. Metrics are served on a
// separate listener that should not be exposed publicly.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Addr    string `mapstructure:"addr"`
}
