package iam

import (
	"context"
	"errors"
	"net/http"

	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/datascope"
	"base-admin/server/internal/modules/auth"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
)

// Data scope specs of the scoped IAM resources.
var (
	userScope = datascope.Spec{DeptColumn: "u.dept_id", OwnerColumn: "u.id"}
	deptScope = datascope.Spec{DeptColumn: "d.id"}
)

const (
	superGuardLock = "base-admin:super-admin-guard"
	deptTreeLock   = "base-admin:dept-tree"
)

var errEscalation = httpx.NewError(http.StatusForbidden, auth.CodeForbidden, "不能管理或授予超出自身的权限或数据范围")

// The central anti-escalation rule (design doc 17, item 5): a caller who is
// not a super admin may only manage a user whose effective permissions and
// data scope are a subset of the caller's own. It is checked before a change
// (may the caller touch this user?) and after the change inside the same
// transaction (did the change make the user outrank the caller?), which
// covers role assignment, role widening, department moves and self-edits.

// EnsureManageable applies the anti-escalation rule for other modules, e.g.
// ending a user's sessions (auth.WithTargetGuard).
func (m *Module) EnsureManageable(ctx context.Context, uid int64) error {
	return ensureOutranks(ctx, m.db.WithContext(ctx), uid)
}

// ensureOutranks requires the caller to outrank user uid as seen by tx.
func ensureOutranks(ctx context.Context, tx *gorm.DB, uid int64) error {
	caller := actor.From(ctx).Snapshot
	if caller.IsSuper {
		return nil
	}
	target, err := auth.Build(ctx, tx, uid)
	if errors.Is(err, auth.ErrUserNotFound) {
		return httpx.NotFound("用户不存在")
	}
	if err != nil {
		return err
	}
	if target.IsSuper {
		return errEscalation
	}
	for _, code := range target.Permissions {
		if !caller.HasPermission(code) {
			return errEscalation
		}
	}
	covered, err := scopeCovered(tx, caller, target.DataScope)
	if err != nil {
		return err
	}
	if !covered {
		return errEscalation
	}
	return nil
}

// scopeCovered reports whether every department in target is visible to the
// caller. A "self" scope only reaches the holder's own rows, which the
// caller can already see whenever it may manage that user.
func scopeCovered(tx *gorm.DB, caller *actor.Snapshot, target datascope.Resolved) (bool, error) {
	if caller.DataScope.All {
		return true, nil
	}
	if target.All {
		return false, nil
	}
	cond, args := datascope.Condition(caller.DataScope, caller.UserID, deptScope)
	if len(target.DeptIDs) > 0 {
		var visible int64
		q := tx.Table("sys_departments AS d").Where("d.id IN ?", target.DeptIDs).Where(cond, args...)
		if err := q.Count(&visible).Error; err != nil {
			return false, err
		}
		if int(visible) != len(target.DeptIDs) {
			return false, nil
		}
	}
	for _, prefix := range target.DeptSubtree {
		var outside int64
		q := tx.Table("sys_departments AS d").Where("d.path LIKE ?", escapeLike(prefix)+"%").Where("NOT "+cond, args...)
		if err := q.Count(&outside).Error; err != nil {
			return false, err
		}
		if outside > 0 {
			return false, nil
		}
	}
	return true, nil
}

// guardRoleHolders requires the caller to outrank every holder of a role,
// e.g. after the role's scope or permissions changed in tx.
func guardRoleHolders(ctx context.Context, tx *gorm.DB, roleID int64) error {
	if actor.From(ctx).Snapshot.IsSuper {
		return nil
	}
	var holders []int64
	if err := tx.Raw(`SELECT ur.user_id FROM sys_user_roles ur JOIN sys_users u ON u.id = ur.user_id
		WHERE ur.role_id = ? AND u.deleted_at IS NULL`, roleID).Scan(&holders).Error; err != nil {
		return err
	}
	for _, uid := range holders {
		if err := ensureOutranks(ctx, tx, uid); err != nil {
			return err
		}
	}
	return nil
}

// guardManageRole forbids a non-super caller from changing a role that
// grants permissions the caller lacks.
func guardManageRole(ctx context.Context, tx *gorm.DB, roleID int64) error {
	snap := actor.From(ctx).Snapshot
	if snap.IsSuper {
		return nil
	}
	codes, err := roleCodes(tx, roleID)
	if err != nil {
		return err
	}
	for _, code := range codes {
		if !snap.HasPermission(code) {
			return errEscalation
		}
	}
	return nil
}

// guardRoleDefinition limits the scope a non-super caller can configure: no
// "all" without having it, and custom departments only within the caller's
// own scope.
func guardRoleDefinition(ctx context.Context, tx *gorm.DB, scope string, deptIDs []int64) error {
	snap := actor.From(ctx).Snapshot
	if snap.IsSuper || snap.DataScope.All {
		return nil
	}
	if scope == string(datascope.All) {
		return errEscalation
	}
	if scope != string(datascope.Custom) || len(deptIDs) == 0 {
		return nil
	}
	covered, err := scopeCovered(tx, snap, datascope.Resolved{DeptIDs: uniq(deptIDs)})
	if err != nil {
		return err
	}
	if !covered {
		return errEscalation
	}
	return nil
}

// isSuperUser reports whether uid holds the super admin role.
func isSuperUser(tx *gorm.DB, uid int64) (bool, error) {
	return exists(tx, `SELECT 1 FROM sys_user_roles ur JOIN sys_roles r ON r.id = ur.role_id
		WHERE ur.user_id = ? AND r.code = ?`, uid, auth.SuperAdminRole)
}

// guardSuperTarget keeps the last enabled super admin from being disabled,
// deleted or stripped of the role, serializing the check with an advisory
// lock. Callers must already hold the user row lock, so the lock order is
// always row lock first, then advisory lock.
func guardSuperTarget(ctx context.Context, tx *gorm.DB, uid int64, losesSuper bool) error {
	super, err := isSuperUser(tx, uid)
	if err != nil || !super {
		return err
	}
	if !actor.From(ctx).Snapshot.IsSuper {
		return protected("只有超级管理员可以修改超级管理员")
	}
	if !losesSuper {
		return nil
	}
	if err := database.LockTx(tx, superGuardLock); err != nil {
		return err
	}
	others, err := exists(tx, `SELECT 1 FROM sys_users u
		JOIN sys_user_roles ur ON ur.user_id = u.id JOIN sys_roles r ON r.id = ur.role_id
		WHERE r.code = ? AND u.status = 1 AND u.deleted_at IS NULL AND u.id <> ?`, auth.SuperAdminRole, uid)
	if err != nil {
		return err
	}
	if !others {
		return protected("不能移除最后一个可用的超级管理员")
	}
	return nil
}

// guardNotSelf rejects destructive operations on the caller's own account.
func guardNotSelf(ctx context.Context, uid int64, msg string) error {
	if actor.From(ctx).UserID() == uid {
		return protected(msg)
	}
	return nil
}

// guardDeptInScope requires a department the caller can see. Users without
// an "all" scope cannot place records outside their scope or at the root.
func guardDeptInScope(ctx context.Context, tx *gorm.DB, deptID *int64, field string) error {
	snap := actor.From(ctx).Snapshot
	if deptID == nil {
		if snap.DataScope.All {
			return nil
		}
		return fieldError(field, "必须选择数据权限范围内的部门")
	}
	q := actor.Scope(ctx, tx.Table("sys_departments AS d").Where("d.id = ?", *deptID), deptScope)
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return fieldError(field, "部门不存在或不在数据权限范围内")
	}
	return nil
}

// lockDeptTree serializes structural department changes so concurrent moves
// cannot create cycles or stale materialized paths.
func lockDeptTree(tx *gorm.DB) error {
	return database.LockTx(tx, deptTreeLock)
}

// lockUserRow locks a visible user before any advisory lock is taken.
func lockUserRow(ctx context.Context, tx *gorm.DB, uid int64) error {
	if err := requireVisibleUser(ctx, tx, uid); err != nil {
		return err
	}
	return tx.Exec("SELECT 1 FROM sys_users WHERE id = ? FOR UPDATE", uid).Error
}
