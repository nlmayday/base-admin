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

// Sync writes declared permissions and menus to the database and runs every
// module's seed, in one transaction guarded by an advisory lock (design doc
// 4.4). It bumps the auth version so cached snapshots pick up changes.
func Sync(ctx context.Context, db *gorm.DB, mods []Module) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", syncLockKey).Error; err != nil {
			return fmt.Errorf("acquire sync lock: %w", err)
		}
		if err := syncPermissions(tx, mods); err != nil {
			return err
		}
		if err := syncMenus(tx, mods); err != nil {
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

func syncPermissions(tx *gorm.DB, mods []Module) error {
	var codes []string
	for _, m := range mods {
		for i, d := range m.Permissions() {
			codes = append(codes, d.Code)
			err := tx.Exec(`
				INSERT INTO sys_permissions (code, module, name, menu_code, requires, sort, status, synced_at)
				VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, 'active', now())
				ON CONFLICT (code) DO UPDATE SET
					module = EXCLUDED.module, name = EXCLUDED.name, menu_code = EXCLUDED.menu_code,
					requires = EXCLUDED.requires, sort = EXCLUDED.sort, status = 'active', synced_at = now()`,
				d.Code, m.Name(), d.Name, d.Menu, database.TextArray(d.EffectiveRequires()), i,
			).Error
			if err != nil {
				return fmt.Errorf("upsert permission %s: %w", d.Code, err)
			}
		}
	}
	// Removed codes are deprecated, never deleted, so existing grants survive
	// and the change is visible to administrators.
	err := tx.Exec(`UPDATE sys_permissions SET status = 'deprecated', synced_at = now()
		WHERE status = 'active' AND code <> ALL(?::text[])`, database.TextArray(codes)).Error
	if err != nil {
		return fmt.Errorf("deprecate removed permissions: %w", err)
	}
	return nil
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
	err := tx.Exec(`DELETE FROM sys_menus WHERE source = 'module' AND code <> ALL(?::text[])`,
		database.TextArray(codes)).Error
	if err != nil {
		return fmt.Errorf("remove undeclared module menus: %w", err)
	}
	return nil
}

// upsertMenu overwrites code-owned fields and keeps administrator-editable
// ones (title, icon, sort, hidden) after the first insert.
func upsertMenu(tx *gorm.DB, module string, d menu.Def) error {
	err := tx.Exec(`
		INSERT INTO sys_menus (code, parent_id, type, title, path, component, icon, sort, hidden,
			keep_alive, permission_code, external_url, source, module)
		VALUES (?, (SELECT id FROM sys_menus WHERE code = NULLIF(?, '')), ?, ?, ?, ?, ?, ?, ?,
			?, NULLIF(?, ''), ?, 'module', ?)
		ON CONFLICT (code) DO UPDATE SET
			parent_id = EXCLUDED.parent_id, type = EXCLUDED.type, path = EXCLUDED.path,
			component = EXCLUDED.component, keep_alive = EXCLUDED.keep_alive,
			permission_code = EXCLUDED.permission_code, external_url = EXCLUDED.external_url,
			source = 'module', module = EXCLUDED.module, updated_at = now()`,
		d.Code, d.Parent, string(d.Type), d.Title, d.Path, d.Component, d.Icon, d.Sort, d.Hidden,
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
