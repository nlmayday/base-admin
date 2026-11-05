package module

import (
	"context"
	"fmt"
	"slices"

	"gorm.io/gorm"

	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/platform/database"
)

// syncLockKey serializes startup synchronization across instances.
const syncLockKey = "base-admin:module-sync"

// Sync writes declared permissions, menus and roles (RoleProvider) to the
// database and runs every module's seed, in one transaction guarded by
// database.LockTx (design doc 4.4). It bumps the auth version so cached snapshots pick up changes.
func Sync(ctx context.Context, db *gorm.DB, mods []Module) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := database.LockTx(tx, syncLockKey); err != nil {
			return fmt.Errorf("acquire sync lock: %w", err)
		}
		added, err := syncPermissions(tx, mods)
		if err != nil {
			return err
		}
		if err := syncMenus(tx, mods); err != nil {
			return err
		}
		if err := syncRoles(tx, mods, added); err != nil {
			return err
		}
		for _, m := range mods {
			if err := m.Seed(ctx, tx); err != nil {
				return fmt.Errorf("seed module %s: %w", m.Name(), err)
			}
		}
		if err := tx.Exec("UPDATE sys_auth_version SET version = version + 1 WHERE id = 1").Error; err != nil {
			return fmt.Errorf("bump auth version: %w", err)
		}
		return nil
	})
}

// syncPermissions upserts the declared permissions and returns the codes
// that were not in the table before (for syncRoles).
func syncPermissions(tx *gorm.DB, mods []Module) (map[string]bool, error) {
	var existing []string
	if err := tx.Raw("SELECT code FROM sys_permissions").Scan(&existing).Error; err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	known := make(map[string]bool, len(existing))
	for _, c := range existing {
		known[c] = true
	}
	added := map[string]bool{}
	var codes []string
	for _, m := range mods {
		for i, d := range m.Permissions() {
			codes = append(codes, d.Code)
			err := tx.Exec(`
				INSERT INTO sys_permissions (code, module, name, menu_code, requires, sort, status, synced_at)
				VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, 'active', CURRENT_TIMESTAMP(6)) `+database.OnConflictUpdate(tx, "code")+`
					module = excluded.module, name = excluded.name, menu_code = excluded.menu_code,
					requires = excluded.requires, sort = excluded.sort, status = 'active', synced_at = CURRENT_TIMESTAMP(6)`,
				d.Code, m.Name(), d.Name, d.Menu, database.TextArray(d.EffectiveRequires()), i,
			).Error
			if err != nil {
				return nil, fmt.Errorf("upsert permission %s: %w", d.Code, err)
			}
			if !known[d.Code] {
				added[d.Code] = true
			}
		}
	}
	// Removed codes are deprecated, never deleted, so existing grants survive
	// and the change is visible to administrators.
	sql, args := notIn(`UPDATE sys_permissions SET status = 'deprecated', synced_at = CURRENT_TIMESTAMP(6)
		WHERE status = 'active'`, codes)
	if err := tx.Exec(sql, args...).Error; err != nil {
		return nil, fmt.Errorf("deprecate removed permissions: %w", err)
	}
	return added, nil
}

func syncMenus(tx *gorm.DB, mods []Module) error {
	defs := orderedMenus(mods)
	codes := make([]string, 0, len(defs))
	for _, md := range defs {
		codes = append(codes, md.def.Code)
		if err := upsertMenu(tx, md.module, md.def); err != nil {
			return err
		}
	}
	sql, args := notIn(`DELETE FROM sys_menus WHERE source = 'module'`, codes)
	if err := tx.Exec(sql, args...).Error; err != nil {
		return fmt.Errorf("remove undeclared module menus: %w", err)
	}
	return nil
}

// upsertMenu overwrites code-owned fields and keeps administrator-editable
// ones (title, icon, sort, hidden) after the first insert.
func upsertMenu(tx *gorm.DB, module string, d menu.Def) error {
	// The parent is looked up first: MySQL cannot read the table an INSERT
	// writes in a subquery of its VALUES.
	if d.Parent == d.Code {
		// PostgreSQL's CHECK (id <> parent_id) has no MySQL counterpart.
		return fmt.Errorf("menu %s cannot be its own parent", d.Code)
	}
	var parentID *int64
	if d.Parent != "" {
		var ids []int64
		if err := tx.Raw("SELECT id FROM sys_menus WHERE code = ?", d.Parent).Scan(&ids).Error; err != nil {
			return fmt.Errorf("find parent of menu %s: %w", d.Code, err)
		}
		if len(ids) > 0 {
			parentID = &ids[0]
		}
	}
	err := tx.Exec(`
		INSERT INTO sys_menus (code, parent_id, type, title, path, component, icon, sort, hidden,
			keep_alive, permission_code, external_url, source, module)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, 'module', ?) `+database.OnConflictUpdate(tx, "code")+`
			parent_id = excluded.parent_id, type = excluded.type, path = excluded.path,
			component = excluded.component, keep_alive = excluded.keep_alive,
			permission_code = excluded.permission_code, external_url = excluded.external_url,
			source = 'module', module = excluded.module, updated_at = CURRENT_TIMESTAMP(6)`,
		d.Code, parentID, string(d.Type), d.Title, d.Path, d.Component, d.Icon, d.Sort, d.Hidden,
		d.KeepAlive, d.Permission, d.ExternalURL, module,
	).Error
	if err != nil {
		return fmt.Errorf("upsert menu %s: %w", d.Code, err)
	}
	return nil
}

type moduleMenu struct {
	module string
	def    menu.Def
}

// orderedMenus returns menus with every parent before its children.
// Validation has already rejected missing parents and cycles.
func orderedMenus(mods []Module) []moduleMenu {
	byCode := map[string]moduleMenu{}
	var codes []string
	for _, m := range mods {
		for _, d := range m.Menus() {
			byCode[d.Code] = moduleMenu{module: m.Name(), def: d}
			codes = append(codes, d.Code)
		}
	}
	slices.Sort(codes)

	done := map[string]bool{}
	out := make([]moduleMenu, 0, len(codes))
	var visit func(code string)
	visit = func(code string) {
		mm, ok := byCode[code]
		if !ok || done[code] {
			return
		}
		done[code] = true
		visit(mm.def.Parent)
		out = append(out, mm)
	}
	for _, c := range codes {
		visit(c)
	}
	return out
}

// Catalog builds the permission catalog of all modules.
func Catalog(mods []Module) perm.Catalog {
	var defs []perm.Def
	for _, m := range mods {
		defs = append(defs, m.Permissions()...)
	}
	return perm.NewCatalog(defs)
}

// notIn restricts a statement to rows whose code is not in codes; with no
// codes every row matches (NOT IN of an empty list would match none).
func notIn(sql string, codes []string) (string, []any) {
	if len(codes) == 0 {
		return sql, nil
	}
	return sql + " AND code NOT IN ?", []any{codes}
}
