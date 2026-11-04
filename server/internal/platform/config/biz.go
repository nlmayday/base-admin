package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"
)

// Biz is the configuration of the business modules of a downstream
// project: the `biz` section of the config file, which the strict checks of
// the scaffold leave alone, handed to the modules as is. Each module decodes
// its part with Decode, strictly (unknown keys are errors there).
//
// Secrets come from environment variables, never from the file:
// ADMIN_BIZ_<KEY> sets biz.<key>, and a double underscore separates levels,
// so ADMIN_BIZ_GAME__INNER_TOKEN sets biz.game.inner_token. Keys whose last
// part is password, secret or token, or ends with _password, _secret or
// _token, are refused in the file. Environment values are strings; Decode
// converts them to the field types ("30" to an int, "5s" to a duration).
type Biz map[string]any

const bizEnvPrefix = envPrefix + "_BIZ_"

// Decode decodes the section at key (dot-separated, "" for all of biz) into
// out, a pointer to a struct with mapstructure tags. A missing section
// leaves out unchanged, so defaults set before the call remain.
func (b Biz) Decode(key string, out any) error {
	var section any = map[string]any(b)
	if key != "" {
		for _, part := range strings.Split(key, ".") {
			m, ok := section.(map[string]any)
			if !ok {
				return fmt.Errorf("biz.%s: not a section", key)
			}
			if section, ok = m[part]; !ok {
				return nil
			}
		}
	}
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: out, ErrorUnused: true, WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(rejectNumericDuration,
			mapstructure.StringToTimeDurationHookFunc(), mapstructure.StringToSliceHookFunc(",")),
	})
	if err != nil {
		return err
	}
	if err := dec.Decode(section); err != nil {
		return fmt.Errorf("decode biz.%s: %w", key, err)
	}
	return nil
}

// String returns the string at key (dot-separated), or "".
func (b Biz) String(key string) string {
	var v any = map[string]any(b)
	for _, part := range strings.Split(key, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v = m[part]
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// loadBiz builds the biz section from the file section and the ADMIN_BIZ_*
// environment variables.
func loadBiz(fromFile any) (Biz, error) {
	b := Biz{}
	if m, ok := fromFile.(map[string]any); ok {
		b = deepCopy(m)
	}
	if err := rejectBizSecrets("biz", b); err != nil {
		return nil, err
	}
	for _, kv := range os.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		rest, ok := strings.CutPrefix(name, bizEnvPrefix)
		if !ok || rest == "" {
			continue
		}
		path := strings.Split(strings.ToLower(rest), "__")
		if err := setPath(b, path, value); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return b, nil
}

func setPath(m map[string]any, path []string, value string) error {
	for i, part := range path {
		if part == "" {
			return fmt.Errorf("empty key part")
		}
		if i == len(path)-1 {
			if _, isMap := m[part].(map[string]any); isMap {
				return fmt.Errorf("biz.%s is a section, not a value", strings.Join(path, "."))
			}
			m[part] = value
			return nil
		}
		next, ok := m[part].(map[string]any)
		if !ok {
			if _, exists := m[part]; exists {
				return fmt.Errorf("biz.%s is a value, not a section", strings.Join(path[:i+1], "."))
			}
			next = map[string]any{}
			m[part] = next
		}
		m = next
	}
	return nil
}

// secretSuffixes mark biz keys that hold secrets.
var secretSuffixes = []string{"password", "secret", "token"}

func isSecretKey(k string) bool {
	k = strings.ToLower(k)
	for _, s := range secretSuffixes {
		if k == s || strings.HasSuffix(k, "_"+s) {
			return true
		}
	}
	return false
}

func rejectBizSecrets(prefix string, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		path := prefix + "." + k
		if sub, ok := m[k].(map[string]any); ok {
			if err := rejectBizSecrets(path, sub); err != nil {
				return err
			}
			continue
		}
		if isSecretKey(k) {
			env := envPrefix + "_" + strings.ToUpper(strings.ReplaceAll(strings.TrimPrefix(path, "biz."), ".", "__"))
			return fmt.Errorf("%s looks like a secret and must not be set in the config file; use %s",
				path, strings.Replace(env, envPrefix+"_", bizEnvPrefix, 1))
		}
	}
	return nil
}

func deepCopy(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			v = deepCopy(sub)
		}
		out[k] = v
	}
	return out
}
