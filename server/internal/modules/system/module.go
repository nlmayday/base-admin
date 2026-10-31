// Package system implements the data dictionary, config params with change
// history and the typed built-in settings (docs/p1-p3-plan.md section 3).
package system

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/platform/httpx"
)

// Deps are the module's dependencies.
type Deps struct {
	DB *gorm.DB
	// Redis caches enabled dictionary items. nil disables the cache.
	Redis  *redis.Client
	Logger *zap.Logger
}

// Module is the system configuration module.
type Module struct {
	db     *gorm.DB
	redis  *redis.Client
	logger *zap.Logger
	values *valueCache
}

var _ module.Module = (*Module)(nil)

// New creates the system module.
func New(deps Deps) *Module {
	logger := deps.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Module{
		db:     deps.DB,
		redis:  deps.Redis,
		logger: logger.Named(Name),
		values: newValueCache(valueCacheTTL, time.Now),
	}
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Permissions implements module.Module.
func (m *Module) Permissions() []perm.Def { return slices.Clone(permissionDefs) }

// Menus implements module.Module.
func (m *Module) Menus() []menu.Def { return slices.Clone(menuDefs) }

// Routes implements module.Module.
func (m *Module) Routes(r *module.Router) {
	types := r.Group("/dict-types")
	types.GET("", access.Perm(permDictView), handle(m.listDictTypes)).Doc(module.Doc{Summary: "字典类型列表", Query: dictTypeFilter{}, Response: httpx.Page[dictTypeView]{}})
	types.POST("", access.Perm(permDictCreate), handle(m.createDictType)).Doc(module.Doc{Summary: "新增字典类型", Body: dictTypeRequest{}, Response: dictTypeView{}})
	types.PUT("/:id", access.Perm(permDictUpdate), handle(m.updateDictType)).Doc(module.Doc{Summary: "编辑字典类型", Body: dictTypeRequest{}, Response: dictTypeView{}})
	types.DELETE("/:id", access.Perm(permDictDelete), handle(m.deleteDictType)).Doc(module.Doc{Summary: "删除字典类型"})
	types.GET("/:id/items", access.Perm(permDictView), handle(m.listDictItems)).Doc(module.Doc{Summary: "字典项列表", Query: dictItemFilter{}, Response: []dictItemView{}})
	types.POST("/:id/items", access.Perm(permDictCreate), handle(m.createDictItem)).Doc(module.Doc{Summary: "新增字典项", Body: dictItemRequest{}, Response: dictItemView{}})

	items := r.Group("/dict-items")
	items.PUT("/:id", access.Perm(permDictUpdate), handle(m.updateDictItem)).Doc(module.Doc{Summary: "编辑字典项", Body: dictItemRequest{}, Response: dictItemView{}})
	items.DELETE("/:id", access.Perm(permDictDelete), handle(m.deleteDictItem)).Doc(module.Doc{Summary: "删除字典项"})

	r.GET("/dicts/:code/items", access.Authenticated(), handle(m.dictOptions)).Doc(module.Doc{Summary: "字典选项", Response: []DictOption{}})

	configs := r.Group("/configs")
	configs.GET("", access.Perm(permConfigView), handle(m.listConfigs)).Doc(module.Doc{Summary: "参数列表", Query: configFilter{}, Response: httpx.Page[configView]{}})
	configs.GET("/groups", access.Perm(permConfigView), handle(m.configGroups)).Doc(module.Doc{Summary: "参数分组", Description: "includeBuiltin=true 时包含内置参数的分组", Response: []string{}})
	configs.POST("", access.Perm(permConfigCreate), handle(m.createConfig)).Doc(module.Doc{Summary: "新增参数", Body: configRequest{}, Response: configView{}})
	configs.PUT("/:id", access.Perm(permConfigUpdate), handle(m.updateConfig)).Doc(module.Doc{Summary: "编辑参数", Body: configRequest{}, Response: configView{}})
	configs.DELETE("/:id", access.Perm(permConfigDelete), handle(m.deleteConfig)).Doc(module.Doc{Summary: "删除参数"})
	configs.GET("/:id/changes", access.Perm(permConfigView), handle(m.listConfigChanges)).Doc(module.Doc{Summary: "参数变更记录", Response: httpx.Page[configChangeView]{}})

	settings := r.Group("/settings")
	settings.GET("", access.Perm(permSettingsView), handle(m.getSettings)).Doc(module.Doc{Summary: "基础设置", Response: settingsView{}})
	settings.PUT("", access.Perm(permSettingsUpdate), handle(m.updateSettings)).Doc(module.Doc{Summary: "修改基础设置", Body: settingsRequest{}, Response: settingsView{}})
	settings.GET("/public", access.Public(), handle(m.publicSettings)).Doc(module.Doc{Summary: "公开站点设置", Response: PublicSettings{}})
}

// Seed inserts the built-in settings. It never overwrites a value; only the
// descriptive metadata and the built-in flag are kept in sync with code.
func (m *Module) Seed(_ context.Context, tx *gorm.DB) error {
	for _, s := range builtinSettings {
		err := tx.Exec(`INSERT INTO sys_configs (key, value, value_type, group_name, name, description, editable, builtin)
			VALUES (?, ?, ?, ?, ?, ?, true, true)
			ON CONFLICT (key) DO UPDATE SET value_type = EXCLUDED.value_type, group_name = EXCLUDED.group_name,
				name = EXCLUDED.name, description = EXCLUDED.description, builtin = true, editable = true`,
			s.Key, s.Value, s.ValueType, s.Group, s.Name, s.Description).Error
		if err != nil {
			return fmt.Errorf("seed setting %s: %w", s.Key, err)
		}
	}
	m.values.clear()
	return nil
}
