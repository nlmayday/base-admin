package module

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gorm.io/gorm"

	"base-admin/server/internal/core/perm"
)

// RoleDef declares a role a module needs, such as 运营 or 客服, with its
// default permissions. Sync creates the role once; afterwards administrators
// own it (name, permissions, users), so a permission they take away stays
// away. Permissions added to Permissions later are granted when they are
// new codes, i.e. on the first start of the version that declares them.
type RoleDef struct {
	// Code identifies the role (^[a-z][a-z0-9_-]{1,31}$), e.g. game_ops.
	Code string
	Name string
	// DataScope is all, dept_and_children, dept or self (default).
	DataScope string
	Remark    string
	// Permissions are permission codes or prefixes ending in "*":
	// "game:*" (the whole module), "game:item:*" (one resource). The codes
	// they require (the view code of a resource) are granted too.
	Permissions []string
}

// RoleProvider is implemented by modules that declare roles (optional).
type RoleProvider interface {
	Roles() []RoleDef
}

var (
	roleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)
	roleScopes      = []string{"all", "dept_and_children", "dept", "self"}
)

func moduleRoles(mods []Module) []RoleDef {
	var out []RoleDef
	for _, m := range mods {
		if p, ok := m.(RoleProvider); ok {
			out = append(out, p.Roles()...)
		}
	}
	return out
}

// validateRoles checks declared roles against the declared permissions.
func validateRoles(rep *Report, mods []Module, defs map[string]perm.Def) {
	seen := map[string]bool{}
	for _, r := range moduleRoles(mods) {
		switch {
		case !roleCodePattern.MatchString(r.Code):
			rep.errorf("role code %q is invalid", r.Code)
		case seen[r.Code]:
			rep.errorf("role %s is declared twice", r.Code)
		case strings.TrimSpace(r.Name) == "" || len([]rune(r.Name)) > 64:
			rep.errorf("role %s needs a name of up to 64 characters", r.Code)
		case r.DataScope != "" && !slices.Contains(roleScopes, r.DataScope):
			rep.errorf("role %s: data scope %q is not one of %s", r.Code, r.DataScope, strings.Join(roleScopes, ", "))
		case len([]rune(r.Remark)) > 255:
			rep.errorf("role %s: remark is longer than 255 characters", r.Code)
		}
		seen[r.Code] = true
		for _, p := range r.Permissions {
			if len(expandPermissions([]string{p}, defs)) == 0 {
				rep.errorf("role %s: permission %q matches no declared permission", r.Code, p)
			}
		}
	}
}

// expandPermissions resolves codes and prefixes with what they require,
// sorted.
func expandPermissions(patterns []string, defs map[string]perm.Def) []string {
	set := map[string]bool{}
	var add func(code string)
	add = func(code string) {
		d, ok := defs[code]
		if !ok || set[code] {
			return
		}
		set[code] = true
		for _, r := range d.EffectiveRequires() {
			add(r)
		}
	}
	for _, p := range patterns {
		if prefix, ok := strings.CutSuffix(p, "*"); ok {
			for code := range defs {
				if strings.HasPrefix(code, prefix) {
					add(code)
				}
			}
			continue
		}
		add(p)
	}
	out := make([]string, 0, len(set))
	for code := range set {
		out = append(out, code)
	}
	slices.Sort(out)
	return out
}

// syncRoles creates the declared roles that do not exist and grants their
// permissions: all of them to a new role, the codes new in this sync
// (added) to an existing one.
func syncRoles(tx *gorm.DB, mods []Module, added map[string]bool) error {
	roles := moduleRoles(mods)
	if len(roles) == 0 {
		return nil
	}
	defs := map[string]perm.Def{}
	for _, m := range mods {
		for _, d := range m.Permissions() {
			defs[d.Code] = d
		}
	}
	for _, r := range roles {
		var ids []int64
		if err := tx.Raw("SELECT id FROM sys_roles WHERE code = ?", r.Code).Scan(&ids).Error; err != nil {
			return fmt.Errorf("find role %s: %w", r.Code, err)
		}
		created := len(ids) == 0
		if created {
			scope := r.DataScope
			if scope == "" {
				scope = "self"
			}
			if err := tx.Exec(`INSERT INTO sys_roles (code, name, data_scope, remark) VALUES (?, ?, ?, ?)`,
				r.Code, r.Name, scope, r.Remark).Error; err != nil {
				return fmt.Errorf("create role %s: %w", r.Code, err)
			}
			if err := tx.Raw("SELECT id FROM sys_roles WHERE code = ?", r.Code).Scan(&ids).Error; err != nil || len(ids) == 0 {
				return fmt.Errorf("find created role %s: %w", r.Code, err)
			}
		}
		for _, code := range expandPermissions(r.Permissions, defs) {
			if !created && !added[code] {
				continue
			}
			var n int64
			if err := tx.Raw("SELECT COUNT(*) FROM sys_role_permissions WHERE role_id = ? AND permission_code = ?",
				ids[0], code).Scan(&n).Error; err != nil {
				return fmt.Errorf("check grant %s of role %s: %w", code, r.Code, err)
			}
			if n > 0 {
				continue
			}
			if err := tx.Exec("INSERT INTO sys_role_permissions (role_id, permission_code) VALUES (?, ?)",
				ids[0], code).Error; err != nil {
				return fmt.Errorf("grant %s to role %s: %w", code, r.Code, err)
			}
		}
	}
	return nil
}
