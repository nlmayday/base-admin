package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/datascope"
)

// SuperAdminRole is the built-in role that bypasses permission checks.
const SuperAdminRole = "super_admin"

// ErrUserNotFound reports that the user does not exist or was deleted.
var ErrUserNotFound = errors.New("user not found")

// Snapshots loads auth snapshots from Redis, rebuilding them from
// PostgreSQL on a miss. Concurrent rebuilds of the same key are merged.
type Snapshots struct {
	db       *gorm.DB
	rdb      *redis.Client
	sessions *SessionStore
	ttl      time.Duration
	group    singleflight.Group
}

// NewSnapshots creates a snapshot loader.
func NewSnapshots(db *gorm.DB, rdb *redis.Client, sessions *SessionStore, ttl time.Duration) *Snapshots {
	return &Snapshots{db: db, rdb: rdb, sessions: sessions, ttl: ttl}
}

// Load returns the snapshot of uid for auth version ver.
func (s *Snapshots) Load(ctx context.Context, ver, uid int64) (*actor.Snapshot, error) {
	key := snapshotKey(ver, uid)
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if err == nil {
		var snap actor.Snapshot
		if err := json.Unmarshal(raw, &snap); err == nil {
			return &snap, nil
		}
	} else if !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	v, err, _ := s.group.Do(key, func() (any, error) { return s.rebuild(ctx, ver, uid) })
	if err != nil {
		return nil, err
	}
	return v.(*actor.Snapshot), nil
}

func (s *Snapshots) rebuild(ctx context.Context, ver, uid int64) (*actor.Snapshot, error) {
	snap, err := Build(ctx, s.db, uid)
	if err != nil {
		return nil, err
	}
	snap.Version = ver
	raw, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("encode snapshot: %w", err)
	}
	// Jitter spreads expiries; it needs no cryptographic randomness.
	jitter := time.Duration(rand.Int64N(int64(time.Minute))) //nolint:gosec // not security-relevant
	if err := s.rdb.Set(ctx, snapshotKey(ver, uid), raw, s.ttl+jitter).Err(); err != nil {
		return nil, fmt.Errorf("cache snapshot: %w", err)
	}
	// Heal a mirror that regressed (design doc 6.5.5 C2).
	if _, err := s.sessions.RaiseGen(ctx, uid, snap.SessionGen); err != nil {
		return nil, err
	}
	return snap, nil
}

type userRow struct {
	ID                 int64
	Status             int16
	SessionGen         int64
	DeptID             *int64
	DeptPath           string
	MustChangePassword bool
}

type roleRow struct {
	ID        int64
	Code      string
	DataScope string
}

// Build reads the authorization view of uid from PostgreSQL.
func Build(ctx context.Context, db *gorm.DB, uid int64) (*actor.Snapshot, error) {
	db = db.WithContext(ctx)
	var u userRow
	res := db.Raw(`SELECT u.id, u.status, u.session_gen, u.dept_id, COALESCE(d.path, '') AS dept_path,
			u.must_change_password
		FROM sys_users u LEFT JOIN sys_departments d ON d.id = u.dept_id
		WHERE u.id = ? AND u.deleted_at IS NULL`, uid).Scan(&u)
	if res.Error != nil {
		return nil, fmt.Errorf("load user: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, ErrUserNotFound
	}

	var roles []roleRow
	if err := db.Raw(`SELECT r.id, r.code, r.data_scope FROM sys_roles r
		JOIN sys_user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = ? AND r.status = 1 ORDER BY r.id`, uid).Scan(&roles).Error; err != nil {
		return nil, fmt.Errorf("load roles: %w", err)
	}

	snap := &actor.Snapshot{
		UserID: u.ID, Status: u.Status, SessionGen: u.SessionGen, DeptID: u.DeptID,
		MustChangePassword: u.MustChangePassword, RoleIDs: []int64{}, Permissions: []string{},
	}
	roleIDs := make([]int64, 0, len(roles))
	for _, r := range roles {
		roleIDs = append(roleIDs, r.ID)
		snap.IsSuper = snap.IsSuper || r.Code == SuperAdminRole
	}
	snap.RoleIDs = roleIDs

	perms, err := loadPermissions(db, roleIDs, snap.IsSuper)
	if err != nil {
		return nil, err
	}
	snap.Permissions = perms

	scopes, err := loadScopes(db, roles)
	if err != nil {
		return nil, err
	}
	var dept *datascope.UserDept
	if u.DeptID != nil {
		dept = &datascope.UserDept{ID: *u.DeptID, Path: u.DeptPath}
	}
	if snap.IsSuper {
		snap.DataScope = datascope.Resolved{All: true}
	} else {
		snap.DataScope = datascope.Merge(dept, scopes)
	}
	return snap, nil
}

// loadPermissions returns active codes granted to roleIDs; the super admin
// gets every active code.
func loadPermissions(db *gorm.DB, roleIDs []int64, super bool) ([]string, error) {
	perms := []string{}
	var err error
	switch {
	case super:
		err = db.Raw(`SELECT code FROM sys_permissions WHERE status = 'active'`).Scan(&perms).Error
	case len(roleIDs) > 0:
		err = db.Raw(`SELECT DISTINCT rp.permission_code FROM sys_role_permissions rp
			JOIN sys_permissions p ON p.code = rp.permission_code AND p.status = 'active'
			WHERE rp.role_id IN ?`, roleIDs).Scan(&perms).Error
	}
	if err != nil {
		return nil, fmt.Errorf("load permissions: %w", err)
	}
	slices.Sort(perms)
	return perms, nil
}

func loadScopes(db *gorm.DB, roles []roleRow) ([]datascope.RoleScope, error) {
	var customIDs []int64
	for _, r := range roles {
		if datascope.Type(r.DataScope) == datascope.Custom {
			customIDs = append(customIDs, r.ID)
		}
	}
	custom := map[int64][]int64{}
	if len(customIDs) > 0 {
		var rows []struct{ RoleID, DeptID int64 }
		if err := db.Raw(`SELECT role_id, dept_id FROM sys_role_depts WHERE role_id IN ?`, customIDs).Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("load custom scopes: %w", err)
		}
		for _, row := range rows {
			custom[row.RoleID] = append(custom[row.RoleID], row.DeptID)
		}
	}
	scopes := make([]datascope.RoleScope, 0, len(roles))
	for _, r := range roles {
		scopes = append(scopes, datascope.RoleScope{Type: datascope.Type(r.DataScope), CustomDeptIDs: custom[r.ID]})
	}
	return scopes, nil
}
