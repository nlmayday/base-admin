package iam

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/datascope"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/modules/auth"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type roleView struct {
	ID        ids.ID    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	DataScope string    `json:"dataScope"`
	DeptIDs   []ids.ID  `json:"deptIds"`
	Status    int16     `json:"status"`
	Builtin   bool      `json:"builtin"`
	Sort      int       `json:"sort"`
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
	Version   int       `json:"version"`
}

type roleRow struct {
	ID        int64
	Code      string
	Name      string
	DataScope string
	Status    int16
	IsBuiltin bool
	Sort      int
	Remark    string
	CreatedAt time.Time
	Version   int
}

func (m *Module) listRoles(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	q := m.db.WithContext(c.Request.Context()).Table("sys_roles")
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		q = q.Where("code ILIKE ? OR name ILIKE ?", like, like)
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []roleRow
	if err := q.Order("sort, id").Offset(pq.Offset()).Limit(pq.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	views, err := m.toRoleViews(c.Request.Context(), rows)
	if err != nil {
		return nil, err
	}
	return httpx.Page[roleView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func (m *Module) toRoleViews(ctx context.Context, rows []roleRow) ([]roleView, error) {
	roleIDs := make([]int64, len(rows))
	for i, r := range rows {
		roleIDs[i] = r.ID
	}
	depts := map[int64][]int64{}
	if len(roleIDs) > 0 {
		var links []struct{ RoleID, DeptID int64 }
		if err := m.db.WithContext(ctx).Raw("SELECT role_id, dept_id FROM sys_role_depts WHERE role_id IN ? ORDER BY dept_id",
			roleIDs).Scan(&links).Error; err != nil {
			return nil, err
		}
		for _, l := range links {
			depts[l.RoleID] = append(depts[l.RoleID], l.DeptID)
		}
	}
	views := make([]roleView, len(rows))
	for i, r := range rows {
		views[i] = roleView{
			ID: ids.ID(r.ID), Code: r.Code, Name: r.Name, DataScope: r.DataScope, DeptIDs: ids.FromInt64s(depts[r.ID]),
			Status: r.Status, Builtin: r.IsBuiltin, Sort: r.Sort, Remark: r.Remark, CreatedAt: r.CreatedAt, Version: r.Version,
		}
	}
	return views, nil
}

func (m *Module) loadRole(ctx context.Context, tx *gorm.DB, id int64) (roleRow, error) {
	var rows []roleRow
	if err := tx.WithContext(ctx).Table("sys_roles").Where("id = ?", id).Scan(&rows).Error; err != nil {
		return roleRow{}, err
	}
	if len(rows) == 0 {
		return roleRow{}, httpx.NotFound("角色不存在")
	}
	return rows[0], nil
}

func (m *Module) roleView(ctx context.Context, id int64) (roleView, error) {
	r, err := m.loadRole(ctx, m.db, id)
	if err != nil {
		return roleView{}, err
	}
	views, err := m.toRoleViews(ctx, []roleRow{r})
	if err != nil {
		return roleView{}, err
	}
	return views[0], nil
}

type option struct {
	ID   ids.ID `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

func (m *Module) roleOptions(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	opts, err := loadOptions(ctx, m.db, "sys_roles")
	if err != nil || actor.From(ctx).Snapshot.IsSuper {
		return opts, err
	}
	visible := make([]option, 0, len(opts))
	for _, o := range opts {
		if o.Code != auth.SuperAdminRole {
			visible = append(visible, o)
		}
	}
	return visible, nil
}

func loadOptions(ctx context.Context, db *gorm.DB, table string) ([]option, error) {
	var rows []struct {
		ID   int64
		Code string
		Name string
	}
	if err := db.WithContext(ctx).Table(table).Select("id, code, name").Where("status = 1").Order("sort, id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]option, len(rows))
	for i, r := range rows {
		out[i] = option{ID: ids.ID(r.ID), Code: r.Code, Name: r.Name}
	}
	return out, nil
}

type roleRequest struct {
	Code      string   `json:"code"`
	Name      string   `json:"name" binding:"required,max=64"`
	DataScope string   `json:"dataScope"`
	DeptIDs   []ids.ID `json:"deptIds" binding:"max=200"`
	Status    int16    `json:"status" binding:"omitempty,oneof=1 2"`
	Sort      int      `json:"sort"`
	Remark    string   `json:"remark" binding:"max=255"`
	Version   *int     `json:"version"`
}

func (r *roleRequest) normalize() error {
	if r.DataScope == "" {
		r.DataScope = string(datascope.Self)
	}
	if !datascope.Type(r.DataScope).Valid() {
		return fieldError("dataScope", "无效的数据范围")
	}
	if r.DataScope != string(datascope.Custom) {
		r.DeptIDs = nil
	}
	if r.Status == 0 {
		r.Status = actor.StatusEnabled
	}
	return nil
}

func (m *Module) createRole(c *gin.Context) (any, error) {
	var req roleRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if !codePattern.MatchString(req.Code) {
		return nil, fieldError("code", "编码须为小写字母开头的 2–32 位小写字母、数字或下划线")
	}
	if err := req.normalize(); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	var id int64
	err := database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		if err := guardRoleDefinition(ctx, tx, req.DataScope, ids.Int64s(req.DeptIDs)); err != nil {
			return err
		}
		res := tx.Raw(`INSERT INTO sys_roles (code, name, data_scope, status, sort, remark, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`, req.Code, req.Name, req.DataScope, req.Status, req.Sort,
			req.Remark, actor.From(ctx).UserID(), actor.From(ctx).UserID()).Scan(&id)
		if res.Error != nil {
			if database.IsUniqueViolation(res.Error) {
				return fieldError("code", "角色编码已存在")
			}
			return res.Error
		}
		return replaceRoleDepts(tx, id, ids.Int64s(req.DeptIDs))
	})
	if err != nil {
		return nil, err
	}
	return m.roleView(ctx, id)
}

func replaceRoleDepts(tx *gorm.DB, roleID int64, deptIDs []int64) error {
	if err := requireAll(tx, "sys_departments", deptIDs, "deptIds", "部门不存在"); err != nil {
		return err
	}
	if err := tx.Exec("DELETE FROM sys_role_depts WHERE role_id = ?", roleID).Error; err != nil {
		return err
	}
	for _, d := range uniq(deptIDs) {
		if err := tx.Exec("INSERT INTO sys_role_depts (role_id, dept_id) VALUES (?, ?)", roleID, d).Error; err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) updateRole(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req roleRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if req.Version == nil {
		return nil, fieldError("version", "不能为空")
	}
	if err := req.normalize(); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	err = m.core.Versions.Mutate(ctx, func(tx *gorm.DB) error {
		role, err := m.loadRole(ctx, tx, id)
		if err != nil {
			return err
		}
		if role.IsBuiltin {
			return protected("内置角色不可修改")
		}
		if err := guardManageRole(ctx, tx, id); err != nil {
			return err
		}
		if err := guardRoleDefinition(ctx, tx, req.DataScope, ids.Int64s(req.DeptIDs)); err != nil {
			return err
		}
		err = updateVersioned(tx, "sys_roles", id, *req.Version, map[string]any{
			"name": req.Name, "data_scope": req.DataScope, "status": req.Status, "sort": req.Sort,
			"remark": req.Remark, "updated_by": actor.From(ctx).UserID(),
		}, "")
		if err != nil {
			return err
		}
		if err := replaceRoleDepts(tx, id, ids.Int64s(req.DeptIDs)); err != nil {
			return err
		}
		// Holders (possibly the caller) must not end up outranking the caller.
		return guardRoleHolders(ctx, tx, id)
	})
	if err != nil {
		return nil, err
	}
	return m.roleView(ctx, id)
}

func (m *Module) deleteRole(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	return nil, m.core.Versions.Mutate(ctx, func(tx *gorm.DB) error {
		role, err := m.loadRole(ctx, tx, id)
		if err != nil {
			return err
		}
		if role.IsBuiltin {
			return protected("内置角色不可删除")
		}
		if err := guardManageRole(ctx, tx, id); err != nil {
			return err
		}
		assigned, err := exists(tx, "SELECT 1 FROM sys_user_roles WHERE role_id = ?", id)
		if err != nil {
			return err
		}
		if assigned {
			return inUse("角色已分配给用户，不能删除")
		}
		return tx.Exec("DELETE FROM sys_roles WHERE id = ?", id).Error
	})
}

type grantPayload struct {
	Codes []string `json:"codes" binding:"max=1000"`
}

func (m *Module) getRolePermissions(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	if _, err := m.loadRole(ctx, m.db, id); err != nil {
		return nil, err
	}
	codes, err := roleCodes(m.db.WithContext(ctx), id)
	if err != nil {
		return nil, err
	}
	return grantPayload{Codes: codes}, nil
}

func roleCodes(tx *gorm.DB, roleID int64) ([]string, error) {
	codes := []string{}
	err := tx.Raw(`SELECT permission_code FROM sys_role_permissions WHERE role_id = ? ORDER BY permission_code`, roleID).
		Scan(&codes).Error
	return codes, err
}

func (m *Module) setRolePermissions(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req grantPayload
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	codes := slices.Compact(slices.Sorted(slices.Values(req.Codes)))
	ctx := c.Request.Context()
	err = m.core.Versions.Mutate(ctx, func(tx *gorm.DB) error {
		role, err := m.loadRole(ctx, tx, id)
		if err != nil {
			return err
		}
		if role.IsBuiltin {
			return protected("内置角色的权限不可修改")
		}
		if err := validateGrant(ctx, tx, id, codes); err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM sys_role_permissions WHERE role_id = ?", id).Error; err != nil {
			return err
		}
		for _, code := range codes {
			if err := tx.Exec("INSERT INTO sys_role_permissions (role_id, permission_code) VALUES (?, ?)", id, code).Error; err != nil {
				return err
			}
		}
		return guardRoleHolders(ctx, tx, id)
	})
	if err != nil {
		return nil, err
	}
	return grantPayload{Codes: codes}, nil
}

// validateGrant checks the dependency closure against active codes (design
// doc 5.3) and that a non-super caller only grants codes it holds itself.
func validateGrant(ctx context.Context, tx *gorm.DB, roleID int64, codes []string) error {
	var rows []struct {
		Code     string
		Requires database.TextArray
	}
	if err := tx.Raw("SELECT code, requires FROM sys_permissions WHERE status = 'active'").Scan(&rows).Error; err != nil {
		return err
	}
	defs := make([]perm.Def, len(rows))
	for i, r := range rows {
		defs[i] = perm.Def{Code: r.Code, Requires: []string(r.Requires)}
	}
	missing, unknown := perm.NewCatalog(defs).MissingDependencies(codes)
	if len(missing) > 0 || len(unknown) > 0 {
		return httpx.NewError(422, CodeGrantDependency, "权限依赖不完整").
			WithDetails(gin.H{"missing": nonNil(missing), "unknown": nonNil(unknown)})
	}
	snap := actor.From(ctx).Snapshot
	if snap.IsSuper {
		return nil
	}
	current, err := roleCodes(tx, roleID)
	if err != nil {
		return err
	}
	for _, code := range codes {
		if !snap.HasPermission(code) && !slices.Contains(current, code) {
			return errEscalation
		}
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
