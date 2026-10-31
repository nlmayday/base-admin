package config

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

const envPrefix = "ADMIN"

// secretKeys must never be read from a config file.
var secretKeys = []string{
	"database.url", "redis.password", "bootstrap.admin_password",
	"file.s3.access_key", "file.s3.secret_key", "file.signing_key",
}

// Load builds the configuration from defaults, the optional file at path and
// ADMIN_* environment variables, then validates it.
func Load(path string) (Config, error) {
	v := viper.New()
	setDefaults(v)
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config file %q: %w", path, err)
		}
		if err := rejectSecretsInFile(v); err != nil {
			return Config{}, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg, strictDecoding); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	cfg.File = cfg.File.withDevSigningKey(cfg.App.Env)
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

// strictDecoding rejects unknown keys (typos would otherwise silently fall
// back to defaults) and bare numbers for durations (YAML `timeout: 5` would
// otherwise mean 5ns).
func strictDecoding(dc *mapstructure.DecoderConfig) {
	dc.ErrorUnused = true
	dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
		rejectNumericDuration,
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	)
}

func rejectNumericDuration(from, to reflect.Type, data any) (any, error) {
	if to != reflect.TypeFor[time.Duration]() || from == to {
		return data, nil
	}
	switch from.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return nil, fmt.Errorf("duration %v must be a string with a unit, such as \"5s\"", data)
	default:
		return data, nil
	}
}

func rejectSecretsInFile(v *viper.Viper) error {
	for _, key := range secretKeys {
		if v.InConfig(key) {
			envName := envPrefix + "_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
			return fmt.Errorf("%s must not be set in the config file; use %s", key, envName)
		}
	}
	return nil
}

// setDefaults registers every key so that environment overrides are picked
// up by Unmarshal.
func setDefaults(v *viper.Viper) {
	for key, value := range defaultValues() {
		v.SetDefault(key, value)
	}
}

// defaultValues are the built-in defaults by key; configs/config.example.yaml
// lists the same keys and values (config_example_test.go).
func defaultValues() map[string]any {
	defaults := map[string]any{
		"app.name": "base-admin",
		"app.env":  string(EnvDev),

		"http.addr":                ":8080",
		"http.read_header_timeout": 5 * time.Second,
		"http.read_timeout":        15 * time.Second,
		"http.write_timeout":       30 * time.Second,
		"http.idle_timeout":        60 * time.Second,
		"http.request_timeout":     20 * time.Second,
		"http.max_body_bytes":      int64(2 << 20),
		"http.drain_delay":         time.Duration(0),
		"http.shutdown_timeout":    30 * time.Second,
		"http.trusted_proxies":     []string{},

		"database.url":                  "",
		"database.max_open_conns":       20,
		"database.max_idle_conns":       10,
		"database.conn_max_lifetime":    30 * time.Minute,
		"database.conn_max_idle_time":   5 * time.Minute,
		"database.connect_timeout":      5 * time.Second,
		"database.statement_timeout":    15 * time.Second,
		"database.slow_query_threshold": 200 * time.Millisecond,

		"redis.addr":                         "127.0.0.1:6379",
		"redis.password":                     "",
		"redis.db":                           0,
		"redis.pool_size":                    20,
		"redis.dial_timeout":                 3 * time.Second,
		"redis.read_timeout":                 2 * time.Second,
		"redis.write_timeout":                2 * time.Second,
		"redis.require_safe_eviction_policy": true,
		"redis.declared_eviction_policy":     "",

		"log.level":  "info",
		"log.format": "json",

		"metrics.enabled": true,
		"metrics.addr":    "127.0.0.1:9090",

		"auth.cookie_name":           "sid",
		"auth.cookie_secure":         true,
		"auth.allowed_origins":       []string{"http://localhost:8848", "http://127.0.0.1:8848"},
		"auth.idle_timeout":          2 * time.Hour,
		"auth.absolute_timeout":      12 * time.Hour,
		"auth.touch_interval":        time.Minute,
		"auth.snapshot_ttl":          10 * time.Minute,
		"auth.reconcile_interval":    10 * time.Second,
		"auth.lock_timeout":          3 * time.Second,
		"auth.login_window":          5 * time.Minute,
		"auth.login_max_per_ip":      50,
		"auth.login_max_per_account": 10,

		"bootstrap.admin_username": "admin",
		"bootstrap.admin_password": "",
	}
	for key, value := range fileDefaults() {
		defaults[key] = value
	}
	return defaults
}
