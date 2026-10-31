package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap/zapcore"
)

// Validate reports every invalid setting at once.
func (c Config) Validate() error {
	return errors.Join(
		c.App.validate(),
		c.HTTP.validate(),
		c.Database.validate(),
		c.Redis.validate(),
		c.Log.validate(),
		c.Metrics.validate(c.HTTP.Addr),
		c.Auth.validate(c.App.Env),
		c.Bootstrap.validate(),
		c.File.validate(c.App.Env),
	)
}

func (a AuthConfig) validate(env Env) error {
	errs := []error{
		positive("auth.idle_timeout", a.IdleTimeout),
		positive("auth.absolute_timeout", a.AbsoluteTimeout),
		positive("auth.touch_interval", a.TouchInterval),
		positive("auth.snapshot_ttl", a.SnapshotTTL),
		positive("auth.reconcile_interval", a.ReconcileInterval),
		positive("auth.lock_timeout", a.LockTimeout),
		positive("auth.login_window", a.LoginWindow),
	}
	if a.CookieName == "" {
		errs = append(errs, errors.New("auth.cookie_name is required"))
	}
	if strings.HasPrefix(a.CookieName, "__Host-") && !a.CookieSecure {
		errs = append(errs, errors.New("auth.cookie_name with the __Host- prefix requires auth.cookie_secure"))
	}
	if env == EnvProd {
		errs = append(errs, a.validateProd())
	}
	if a.IdleTimeout > a.AbsoluteTimeout {
		errs = append(errs, errors.New("auth.idle_timeout must not exceed auth.absolute_timeout"))
	}
	if a.TouchInterval >= a.IdleTimeout {
		errs = append(errs, errors.New("auth.touch_interval must be shorter than auth.idle_timeout"))
	}
	if len(a.AllowedOrigins) == 0 {
		errs = append(errs, errors.New("auth.allowed_origins must list the frontend origin"))
	}
	for _, o := range a.AllowedOrigins {
		errs = append(errs, origin("auth.allowed_origins", o))
	}
	if a.LoginMaxPerIP <= 0 || a.LoginMaxPerAccount <= 0 {
		errs = append(errs, errors.New("auth.login_max_per_ip and auth.login_max_per_account must be positive"))
	}
	return errors.Join(errs...)
}

// validateProd enforces production-only hardening.
func (a AuthConfig) validateProd() error {
	var errs []error
	if !a.CookieSecure {
		errs = append(errs, errors.New("auth.cookie_secure must be true in prod"))
	}
	if !strings.HasPrefix(a.CookieName, "__Host-") {
		errs = append(errs, errors.New("auth.cookie_name must use the __Host- prefix in prod"))
	}
	for _, o := range a.AllowedOrigins {
		if u, err := url.Parse(o); err == nil && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1") {
			errs = append(errs, fmt.Errorf("auth.allowed_origins must not contain %q in prod", o))
		}
	}
	return errors.Join(errs...)
}

func (b BootstrapConfig) validate() error {
	if b.AdminUsername == "" {
		return errors.New("bootstrap.admin_username is required")
	}
	if b.AdminPassword != "" && (len(b.AdminPassword) < 12 || len(b.AdminPassword) > 72) {
		return errors.New("bootstrap.admin_password must be 12–72 characters")
	}
	return nil
}

// origin accepts exactly scheme://host[:port] with http or https.
func origin(key, value string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("%s entry %q must be scheme://host[:port]", key, value)
	}
	return nil
}

func (a AppConfig) validate() error {
	var errs []error
	if a.Name == "" {
		errs = append(errs, errors.New("app.name is required"))
	}
	switch a.Env {
	case EnvDev, EnvTest, EnvProd:
	default:
		errs = append(errs, fmt.Errorf("app.env must be one of dev, test, prod; got %q", a.Env))
	}
	return errors.Join(errs...)
}

func (h HTTPConfig) validate() error {
	errs := []error{
		hostPort("http.addr", h.Addr),
		positive("http.read_header_timeout", h.ReadHeaderTimeout),
		positive("http.read_timeout", h.ReadTimeout),
		positive("http.write_timeout", h.WriteTimeout),
		positive("http.idle_timeout", h.IdleTimeout),
		positive("http.request_timeout", h.RequestTimeout),
		positive("http.shutdown_timeout", h.ShutdownTimeout),
	}
	if h.RequestTimeout >= h.WriteTimeout {
		errs = append(errs, errors.New("http.request_timeout must be shorter than http.write_timeout"))
	}
	if h.MaxBodyBytes <= 0 {
		errs = append(errs, errors.New("http.max_body_bytes must be positive"))
	}
	if h.DrainDelay < 0 {
		errs = append(errs, errors.New("http.drain_delay must not be negative"))
	}
	if h.DrainDelay+h.ShutdownTimeout > maxStopDuration {
		errs = append(errs, fmt.Errorf("http.drain_delay + http.shutdown_timeout must not exceed %s", maxStopDuration))
	}
	for _, p := range h.TrustedProxies {
		errs = append(errs, ipOrPrefix("http.trusted_proxies", p))
	}
	return errors.Join(errs...)
}

func (d DatabaseConfig) validate() error {
	errs := []error{
		positive("database.conn_max_lifetime", d.ConnMaxLifetime),
		positive("database.conn_max_idle_time", d.ConnMaxIdleTime),
		positive("database.connect_timeout", d.ConnectTimeout),
		positive("database.slow_query_threshold", d.SlowQueryThreshold),
	}
	if d.URL == "" {
		errs = append(errs, errors.New("database.url is required (set ADMIN_DATABASE_URL)"))
	}
	if d.MaxOpenConns <= 0 {
		errs = append(errs, errors.New("database.max_open_conns must be positive"))
	}
	if d.MaxIdleConns < 0 || d.MaxIdleConns > d.MaxOpenConns {
		errs = append(errs, errors.New("database.max_idle_conns must be between 0 and database.max_open_conns"))
	}
	if d.StatementTimeout < 0 {
		errs = append(errs, errors.New("database.statement_timeout must not be negative"))
	}
	return errors.Join(errs...)
}

func (r RedisConfig) validate() error {
	errs := []error{
		hostPort("redis.addr", r.Addr),
		positive("redis.dial_timeout", r.DialTimeout),
		positive("redis.read_timeout", r.ReadTimeout),
		positive("redis.write_timeout", r.WriteTimeout),
	}
	if r.DB < 0 {
		errs = append(errs, errors.New("redis.db must not be negative"))
	}
	if r.PoolSize <= 0 {
		errs = append(errs, errors.New("redis.pool_size must be positive"))
	}
	return errors.Join(errs...)
}

func (l LogConfig) validate() error {
	var errs []error
	if _, err := zapcore.ParseLevel(l.Level); err != nil {
		errs = append(errs, fmt.Errorf("log.level: %w", err))
	}
	if l.Format != "json" && l.Format != "console" {
		errs = append(errs, fmt.Errorf("log.format must be json or console; got %q", l.Format))
	}
	return errors.Join(errs...)
}

func (m MetricsConfig) validate(httpAddr string) error {
	if !m.Enabled {
		return nil
	}
	if err := hostPort("metrics.addr", m.Addr); err != nil {
		return err
	}
	if m.Addr == httpAddr {
		return errors.New("metrics.addr must differ from http.addr")
	}
	return nil
}

const (
	// minDuration catches values that are technically positive but
	// nonsensical, such as a timeout of a few nanoseconds.
	minDuration = time.Millisecond
	// maxStopDuration bounds how long a stop signal may take to honour.
	maxStopDuration = 5 * time.Minute
)

func positive(key string, d time.Duration) error {
	if d < minDuration {
		return fmt.Errorf("%s must be at least %s", key, minDuration)
	}
	return nil
}

func hostPort(key, addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%s must be host:port; got %q", key, addr)
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return fmt.Errorf("%s has an invalid port in %q", key, addr)
	}
	return nil
}

// ipOrPrefix accepts a single IP or CIDR. Zoned addresses are rejected
// because gin cannot use them, and catch-all prefixes such as 0.0.0.0/0
// would let any client spoof X-Forwarded-For.
func ipOrPrefix(key, value string) error {
	if addr, err := netip.ParseAddr(value); err == nil {
		if addr.Zone() != "" {
			return fmt.Errorf("%s entry %q must not have a zone", key, value)
		}
		return nil
	}
	if prefix, err := netip.ParsePrefix(value); err == nil {
		if prefix.Bits() == 0 {
			return fmt.Errorf("%s entry %q would trust every client", key, value)
		}
		return nil
	}
	return fmt.Errorf("%s entry %q is not an IP or CIDR", key, value)
}
