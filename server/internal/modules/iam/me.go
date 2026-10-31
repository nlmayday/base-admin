package iam

import (
	"context"
	"net/url"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type meUser struct {
	ID        ids.ID  `json:"id"`
	Username  string  `json:"username"`
	Nickname  string  `json:"nickname"`
	Email     string  `json:"email"`
	Phone     string  `json:"phone"`
	AvatarURL string  `json:"avatarUrl"`
	DeptID    *ids.ID `json:"deptId"`
	DeptName  string  `json:"deptName"`
	Version   int     `json:"version"`
}

type meRole struct {
	ID   ids.ID `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type meView struct {
	User               meUser   `json:"user"`
	Roles              []meRole `json:"roles"`
	Permissions        []string `json:"permissions"`
	IsSuperAdmin       bool     `json:"isSuperAdmin"`
	MustChangePassword bool     `json:"mustChangePassword"`
}

// Me renders the actor in ctx; it is also the auth module's MeFunc. User
// data comes from PostgreSQL, authorization data from the snapshot.
func (m *Module) Me(ctx context.Context) (any, error) {
	snap := actor.From(ctx).Snapshot
	var u struct {
		ID        int64
		Username  string
		Nickname  string
		Email     string
		Phone     string
		AvatarURL string
		DeptID    *int64
		DeptName  string
		Version   int
	}
	res := m.db.WithContext(ctx).Raw(`SELECT u.id, u.username, u.nickname, u.email, u.phone, u.avatar_url, u.dept_id,
			COALESCE(d.name, '') AS dept_name, u.version
		FROM sys_users u LEFT JOIN sys_departments d ON d.id = u.dept_id
		WHERE u.id = ? AND u.deleted_at IS NULL`, snap.UserID).Scan(&u)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, httpx.NotFound("用户不存在")
	}
	roles := []meRole{}
	if len(snap.RoleIDs) > 0 {
		var rows []struct {
			ID   int64
			Code string
			Name string
		}
		if err := m.db.WithContext(ctx).Raw("SELECT id, code, name FROM sys_roles WHERE id IN ? ORDER BY sort, id",
			snap.RoleIDs).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			roles = append(roles, meRole{ID: ids.ID(r.ID), Code: r.Code, Name: r.Name})
		}
	}
	return meView{
		User: meUser{ID: ids.ID(u.ID), Username: u.Username, Nickname: u.Nickname, Email: u.Email, Phone: u.Phone,
			AvatarURL: u.AvatarURL, DeptID: optionalID(u.DeptID), DeptName: u.DeptName, Version: u.Version},
		Roles: roles, Permissions: snap.Permissions, IsSuperAdmin: snap.IsSuper, MustChangePassword: snap.MustChangePassword,
	}, nil
}

func (m *Module) getMe(c *gin.Context) (any, error) { return m.Me(c.Request.Context()) }

type profileRequest struct {
	Nickname  string `json:"nickname" binding:"max=64"`
	Email     string `json:"email" binding:"omitempty,email,max=128"`
	Phone     string `json:"phone" binding:"max=32"`
	AvatarURL string `json:"avatarUrl" binding:"max=512"`
	Version   *int   `json:"version" binding:"required"`
}

func (m *Module) updateProfile(c *gin.Context) (any, error) {
	var req profileRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if !validAvatar(req.AvatarURL) {
		return nil, fieldError("avatarUrl", "头像地址须为 http(s) 地址或站内路径")
	}
	ctx := c.Request.Context()
	uid := actor.From(ctx).UserID()
	err := updateVersioned(m.db.WithContext(ctx), "sys_users", uid, *req.Version, map[string]any{
		"nickname": req.Nickname, "email": req.Email, "phone": req.Phone, "avatar_url": req.AvatarURL, "updated_by": uid,
	}, "")
	if err != nil {
		return nil, err
	}
	return m.Me(ctx)
}

type routeMeta struct {
	Title     string `json:"title"`
	Icon      string `json:"icon,omitempty"`
	Rank      int    `json:"rank"`
	ShowLink  bool   `json:"showLink"`
	KeepAlive bool   `json:"keepAlive,omitempty"`
	// ShowParent keeps a directory visible in the sidebar even when it has a
	// single child; pure-admin would otherwise show the child in its place.
	ShowParent bool `json:"showParent,omitempty"`
}

type asyncRoute struct {
	Path      string        `json:"path"`
	Name      string        `json:"name,omitempty"`
	Component string        `json:"component,omitempty"`
	Meta      routeMeta     `json:"meta"`
	Children  []*asyncRoute `json:"children,omitempty"`
}

// myMenus returns the menus the caller may view, in pure-admin async route
// format. Directories without visible children are dropped.
func (m *Module) myMenus(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	snap := actor.From(ctx).Snapshot
	rows, err := loadMenus(ctx, m.db)
	if err != nil {
		return nil, err
	}
	children := map[int64][]menuRow{}
	var roots []menuRow
	present := map[int64]bool{}
	for _, r := range rows {
		present[r.ID] = true
	}
	for _, r := range rows {
		if r.ParentID != nil && present[*r.ParentID] {
			children[*r.ParentID] = append(children[*r.ParentID], r)
		} else {
			roots = append(roots, r)
		}
	}
	return buildRoutes(roots, children, snap), nil
}

func buildRoutes(rows []menuRow, children map[int64][]menuRow, snap *actor.Snapshot) []*asyncRoute {
	out := []*asyncRoute{}
	for _, r := range rows {
		route := &asyncRoute{
			Path: r.Path, Name: routeName(r.Code),
			Meta: routeMeta{Title: r.Title, Icon: r.Icon, Rank: r.Sort, ShowLink: !r.Hidden, KeepAlive: r.KeepAlive},
		}
		switch r.Type {
		case "dir":
			route.Children = buildRoutes(children[r.ID], children, snap)
			if len(route.Children) == 0 {
				continue
			}
			for _, child := range route.Children {
				child.Meta.ShowParent = true
			}
		case "menu":
			if r.PermissionCode == nil || !snap.HasPermission(*r.PermissionCode) {
				continue
			}
			route.Component = r.Component
		case "link":
			if r.PermissionCode != nil && !snap.HasPermission(*r.PermissionCode) {
				continue
			}
			route.Path = r.ExternalURL
		}
		out = append(out, route)
	}
	return out
}

// routeName turns a menu code such as "iam.user" into "IamUser".
func routeName(code string) string {
	var b strings.Builder
	upper := true
	for _, r := range code {
		if r == '.' || r == '-' || r == '_' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// validAvatar accepts an empty value, a site-relative path or an http(s) URL.
func validAvatar(s string) bool {
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//") {
		return true
	}
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
