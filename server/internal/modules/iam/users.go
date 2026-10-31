package iam

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/modules/auth"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type userView struct {
	ID          ids.ID     `json:"id"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	Email       string     `json:"email"`
	Phone       string     `json:"phone"`
	AvatarURL   string     `json:"avatarUrl"`
	DeptID      *ids.ID    `json:"deptId"`
	DeptName    string     `json:"deptName"`
	Status      int16      `json:"status"`
	RoleIDs     []ids.ID   `json:"roleIds"`
	PositionIDs []ids.ID   `json:"positionIds"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	Version     int        `json:"version"`
}

type userRow struct {
	ID          int64
	Username    string
	Nickname    string
	Email       string
	Phone       string
	AvatarURL   string
	DeptID      *int64
	DeptName    string
	Status      int16
	LastLoginAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
}

const userColumns = `u.id, u.username, u.nickname, u.email, u.phone, u.avatar_url, u.dept_id,
	COALESCE(d.name, '') AS dept_name, u.status, u.last_login_at, u.created_at, u.updated_at, u.version`

type userFilter struct {
	Keyword string `form:"keyword" binding:"max=64"`
	DeptID  string `form:"deptId"`
	Status  int16  `form:"status" binding:"omitempty,oneof=1 2"`
	// namesOnly restricts keyword matching to username and nickname, so
	// permission-less endpoints cannot be used to probe phone numbers.
	namesOnly bool
}

// scopedUsers starts a query over users visible to the caller.
func scopedUsers(ctx context.Context, db *gorm.DB) *gorm.DB {
	q := db.WithContext(ctx).Table("sys_users AS u").
		Joins("LEFT JOIN sys_departments d ON d.id = u.dept_id").
		Where("u.deleted_at IS NULL")
	return actor.Scope(ctx, q, userScope)
}

func applyUserFilter(q *gorm.DB, f userFilter) (*gorm.DB, error) {
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		if f.namesOnly {
			q = q.Where("(u.username ILIKE ? OR u.nickname ILIKE ?)", like, like)
		} else {
			q = q.Where("(u.username ILIKE ? OR u.nickname ILIKE ? OR u.phone ILIKE ?)", like, like, like)
		}
	}
	if f.DeptID != "" {
		id, err := ids.Parse(f.DeptID)
		if err != nil {
			return nil, fieldError("deptId", "无效的部门")
		}
		q = q.Where(`u.dept_id IN (SELECT id FROM sys_departments WHERE path LIKE
			(SELECT path FROM sys_departments WHERE id = ?) || '%')`, int64(id))
	}
	if f.Status != 0 {
		q = q.Where("u.status = ?", f.Status)
	}
	return q, nil
}

func (m *Module) listUsers(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	var f userFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		return nil, httpx.BadRequest("查询参数错误").WithCause(err)
	}
	q, err := applyUserFilter(scopedUsers(c.Request.Context(), m.db), f)
	if err != nil {
		return nil, err
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []userRow
	if err := q.Select(userColumns).Order("u.id DESC").Offset(pq.Offset()).Limit(pq.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	views, err := m.toUserViews(c.Request.Context(), rows)
	if err != nil {
		return nil, err
	}
	return httpx.Page[userView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func (m *Module) toUserViews(ctx context.Context, rows []userRow) ([]userView, error) {
	userIDs := make([]int64, len(rows))
	for i, r := range rows {
		userIDs[i] = r.ID
	}
	roles, err := idsByUser(ctx, m.db, "sys_user_roles", "role_id", userIDs)
	if err != nil {
		return nil, err
	}
	positions, err := idsByUser(ctx, m.db, "sys_user_positions", "position_id", userIDs)
	if err != nil {
		return nil, err
	}
	views := make([]userView, len(rows))
	for i, r := range rows {
		views[i] = userView{
			ID: ids.ID(r.ID), Username: r.Username, Nickname: r.Nickname, Email: r.Email, Phone: r.Phone,
			AvatarURL: r.AvatarURL, DeptID: optionalID(r.DeptID), DeptName: r.DeptName, Status: r.Status,
			RoleIDs: ids.FromInt64s(roles[r.ID]), PositionIDs: ids.FromInt64s(positions[r.ID]),
			LastLoginAt: r.LastLoginAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version,
		}
	}
	return views, nil
}

// idsByUser loads a user→ids association; table and column are constants.
func idsByUser(ctx context.Context, db *gorm.DB, table, column string, userIDs []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	if len(userIDs) == 0 {
		return out, nil
	}
	var rows []struct{ UserID, Ref int64 }
	err := db.WithContext(ctx).Raw("SELECT user_id, "+column+" AS ref FROM "+table+
		" WHERE user_id IN ? ORDER BY "+column, userIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.UserID] = append(out[r.UserID], r.Ref)
	}
	return out, nil
}

func (m *Module) getUser(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	return m.loadUserView(c.Request.Context(), id)
}

func (m *Module) loadUserView(ctx context.Context, id int64) (userView, error) {
	var rows []userRow
	if err := scopedUsers(ctx, m.db).Select(userColumns).Where("u.id = ?", id).Scan(&rows).Error; err != nil {
		return userView{}, err
	}
	if len(rows) == 0 {
		return userView{}, httpx.NotFound("用户不存在")
	}
	views, err := m.toUserViews(ctx, rows)
	if err != nil {
		return userView{}, err
	}
	return views[0], nil
}

// requireVisibleUser locks nothing; it only proves the caller may see uid.
func requireVisibleUser(ctx context.Context, tx *gorm.DB, uid int64) error {
	var n int64
	if err := scopedUsers(ctx, tx).Where("u.id = ?", uid).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return httpx.NotFound("用户不存在")
	}
	return nil
}

type createUserRequest struct {
	Username    string   `json:"username" binding:"required"`
	Password    string   `json:"password" binding:"required,min=8,max=72"`
	Nickname    string   `json:"nickname" binding:"max=64"`
	Email       string   `json:"email" binding:"omitempty,email,max=128"`
	Phone       string   `json:"phone" binding:"max=32"`
	DeptID      *ids.ID  `json:"deptId"`
	Status      int16    `json:"status" binding:"omitempty,oneof=1 2"`
	RoleIDs     []ids.ID `json:"roleIds" binding:"max=50"`
	PositionIDs []ids.ID `json:"positionIds" binding:"max=50"`
}

func (m *Module) createUser(c *gin.Context) (any, error) {
	var req createUserRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if !usernamePattern.MatchString(req.Username) {
		return nil, fieldError("username", "用户名须为 3–32 位字母、数字或 _ . -")
	}
	if req.Status == 0 {
		req.Status = actor.StatusEnabled
	}
	ctx := c.Request.Context()
	if err := m.core.ValidatePassword(ctx, "password", req.Password); err != nil {
		return nil, err
	}
	hash, err := m.core.HashPassword(ctx, req.Password)
	if err != nil {
		return nil, err
	}
	var id int64
	err = database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		if err := m.checkUserRefs(ctx, tx, req.DeptID, req.RoleIDs, req.PositionIDs); err != nil {
			return err
		}
		// The administrator chose the password, so the user must replace it.
		res := tx.Raw(`INSERT INTO sys_users (username, password_hash, nickname, email, phone, dept_id, status,
				must_change_password, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, true, ?, ?) RETURNING id`,
			req.Username, hash, req.Nickname, req.Email, req.Phone, int64Ptr(req.DeptID), req.Status,
			actor.From(ctx).UserID(), actor.From(ctx).UserID()).Scan(&id)
		if res.Error != nil {
			if database.IsUniqueViolation(res.Error) {
				return fieldError("username", "用户名已存在")
			}
			return res.Error
		}
		if err := replaceUserLinks(tx, id, ids.Int64s(req.RoleIDs), ids.Int64s(req.PositionIDs)); err != nil {
			return err
		}
		return ensureOutranks(ctx, tx, id)
	})
	if err != nil {
		return nil, err
	}
	return m.loadUserView(ctx, id)
}

// checkUserRefs validates the department, roles and positions of a user.
func (m *Module) checkUserRefs(ctx context.Context, tx *gorm.DB, deptID *ids.ID, roleIDs, positionIDs []ids.ID) error {
	if err := guardDeptInScope(ctx, tx, int64Ptr(deptID), "deptId"); err != nil {
		return err
	}
	if err := requireAll(tx, "sys_roles", ids.Int64s(roleIDs), "roleIds", "角色不存在"); err != nil {
		return err
	}
	return requireAll(tx, "sys_positions", ids.Int64s(positionIDs), "positionIds", "岗位不存在")
}

func requireAll(tx *gorm.DB, table string, list []int64, field, msg string) error {
	if len(list) == 0 {
		return nil
	}
	var n int64
	if err := tx.Table(table).Where("id IN ?", list).Count(&n).Error; err != nil {
		return err
	}
	if int(n) != len(uniq(list)) {
		return fieldError(field, msg)
	}
	return nil
}

func replaceUserLinks(tx *gorm.DB, uid int64, roleIDs, positionIDs []int64) error {
	if err := tx.Exec("DELETE FROM sys_user_roles WHERE user_id = ?", uid).Error; err != nil {
		return err
	}
	for _, rid := range uniq(roleIDs) {
		if err := tx.Exec("INSERT INTO sys_user_roles (user_id, role_id) VALUES (?, ?)", uid, rid).Error; err != nil {
			return err
		}
	}
	if err := tx.Exec("DELETE FROM sys_user_positions WHERE user_id = ?", uid).Error; err != nil {
		return err
	}
	for _, pid := range uniq(positionIDs) {
		if err := tx.Exec("INSERT INTO sys_user_positions (user_id, position_id) VALUES (?, ?)", uid, pid).Error; err != nil {
			return err
		}
	}
	return nil
}

type updateUserRequest struct {
	Nickname    string   `json:"nickname" binding:"max=64"`
	Email       string   `json:"email" binding:"omitempty,email,max=128"`
	Phone       string   `json:"phone" binding:"max=32"`
	DeptID      *ids.ID  `json:"deptId"`
	RoleIDs     []ids.ID `json:"roleIds" binding:"max=50"`
	PositionIDs []ids.ID `json:"positionIds" binding:"max=50"`
	Version     *int     `json:"version" binding:"required"`
}

func (m *Module) updateUser(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req updateUserRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	err = m.core.Versions.Mutate(ctx, func(tx *gorm.DB) error {
		if err := lockUserRow(ctx, tx, id); err != nil {
			return err
		}
		if err := ensureOutranks(ctx, tx, id); err != nil {
			return err
		}
		keepsSuper, err := includesSuperRole(tx, ids.Int64s(req.RoleIDs))
		if err != nil {
			return err
		}
		if err := guardSuperTarget(ctx, tx, id, !keepsSuper); err != nil {
			return err
		}
		if err := m.checkUserRefs(ctx, tx, req.DeptID, req.RoleIDs, req.PositionIDs); err != nil {
			return err
		}
		err = updateVersioned(tx, "sys_users", id, *req.Version, map[string]any{
			"nickname": req.Nickname, "email": req.Email, "phone": req.Phone,
			"dept_id": int64Ptr(req.DeptID), "updated_by": actor.From(ctx).UserID(),
		}, "")
		if err != nil {
			return err
		}
		if err := replaceUserLinks(tx, id, ids.Int64s(req.RoleIDs), ids.Int64s(req.PositionIDs)); err != nil {
			return err
		}
		// After the change the caller must still outrank the user; this
		// blocks role assignments and department moves that widen access.
		return ensureOutranks(ctx, tx, id)
	})
	if err != nil {
		return nil, err
	}
	return m.loadUserView(ctx, id)
}

func includesSuperRole(tx *gorm.DB, roleIDs []int64) (bool, error) {
	if len(roleIDs) == 0 {
		return false, nil
	}
	return exists(tx, "SELECT 1 FROM sys_roles WHERE id IN ? AND code = ?", roleIDs, auth.SuperAdminRole)
}

type statusRequest struct {
	Status  int16 `json:"status" binding:"required,oneof=1 2"`
	Version *int  `json:"version" binding:"required"`
}

func (m *Module) setUserStatus(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req statusRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	uid := actor.From(ctx).UserID()
	if req.Status == actor.StatusEnabled {
		err = m.core.Versions.Mutate(ctx, func(tx *gorm.DB) error {
			if err := lockUserRow(ctx, tx, id); err != nil {
				return err
			}
			if err := ensureOutranks(ctx, tx, id); err != nil {
				return err
			}
			if err := guardSuperTarget(ctx, tx, id, false); err != nil {
				return err
			}
			return updateVersioned(tx, "sys_users", id, *req.Version, map[string]any{"status": req.Status, "updated_by": uid}, "")
		})
	} else {
		err = m.revokeUser(ctx, id, func(tx *gorm.DB) error {
			if err := m.prepareDestructive(ctx, tx, id, "不能停用自己"); err != nil {
				return err
			}
			// Checked before the generation raise, so a stale version has
			// no side effect (the row is already locked).
			return requireVersion(tx, id, *req.Version)
		}, func(tx *gorm.DB) error {
			return tx.Exec("UPDATE sys_users SET status = 2, version = version + 1, updated_at = now(), updated_by = ? WHERE id = ?",
				uid, id).Error
		})
	}
	if err != nil {
		return nil, err
	}
	return m.loadUserView(ctx, id)
}

// prepareDestructive runs the checks shared by disable and delete before any
// session is revoked.
func (m *Module) prepareDestructive(ctx context.Context, tx *gorm.DB, id int64, selfMsg string) error {
	if err := guardNotSelf(ctx, id, selfMsg); err != nil {
		return err
	}
	if err := requireVisibleUser(ctx, tx, id); err != nil {
		return err
	}
	if err := ensureOutranks(ctx, tx, id); err != nil {
		return err
	}
	return guardSuperTarget(ctx, tx, id, true)
}

// revokeUser ends all sessions of a visible user together with a change.
// The cheap scoped check runs first, so out-of-scope ids return 404 without
// taking a row lock.
func (m *Module) revokeUser(ctx context.Context, id int64, prepare, apply func(tx *gorm.DB) error) error {
	if err := requireVisibleUser(ctx, m.db, id); err != nil {
		return err
	}
	_, err := m.core.Revoker.Revoke(ctx, id, auth.RevokeSpec{
		Prepare: prepare,
		Apply:   func(tx *gorm.DB, _ int64) error { return apply(tx) },
	})
	return err
}

func requireVersion(tx *gorm.DB, id int64, version int) error {
	var current int
	if err := tx.Raw("SELECT version FROM sys_users WHERE id = ?", id).Scan(&current).Error; err != nil {
		return err
	}
	if current != version {
		return errVersionConflict
	}
	return nil
}

type resetPasswordRequest struct {
	Password string `json:"password" binding:"required,min=8,max=72"`
}

func (m *Module) resetPassword(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req resetPasswordRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	if err := requireVisibleUser(ctx, m.db, id); err != nil {
		return nil, err
	}
	err = m.core.ResetPassword(ctx, id, req.Password, func(tx *gorm.DB) error {
		if err := requireVisibleUser(ctx, tx, id); err != nil {
			return err
		}
		// Resetting the password of a more privileged user is a takeover.
		if err := ensureOutranks(ctx, tx, id); err != nil {
			return err
		}
		return guardSuperTarget(ctx, tx, id, false)
	})
	return nil, err
}

func (m *Module) deleteUser(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	err = m.revokeUser(ctx, id, func(tx *gorm.DB) error {
		return m.prepareDestructive(ctx, tx, id, "不能删除自己")
	}, func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE sys_users SET deleted_at = now(), status = 2, version = version + 1,
			updated_by = ? WHERE id = ?`, actor.From(ctx).UserID(), id).Error; err != nil {
			return err
		}
		return replaceUserLinks(tx, id, nil, nil)
	})
	return nil, err
}

type userOption struct {
	ID       ids.ID `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

func (m *Module) userOptions(c *gin.Context) (any, error) {
	q, err := applyUserFilter(scopedUsers(c.Request.Context(), m.db), userFilter{Keyword: c.Query("keyword"), namesOnly: true})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID       int64
		Username string
		Nickname string
	}
	if err := q.Select("u.id, u.username, u.nickname").Where("u.status = 1").Order("u.username").Limit(50).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]userOption, len(rows))
	for i, r := range rows {
		out[i] = userOption{ID: ids.ID(r.ID), Username: r.Username, Nickname: r.Nickname}
	}
	return out, nil
}

func optionalID(p *int64) *ids.ID {
	if p == nil {
		return nil
	}
	id := ids.ID(*p)
	return &id
}

func int64Ptr(p *ids.ID) *int64 {
	if p == nil {
		return nil
	}
	n := int64(*p)
	return &n
}

func uniq(in []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
