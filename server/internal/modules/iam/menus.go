package iam

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type menuRow struct {
	ID             int64
	Code           string
	ParentID       *int64
	Type           string
	Title          string
	Path           string
	Component      string
	Icon           string
	Sort           int
	Hidden         bool
	KeepAlive      bool
	PermissionCode *string
	ExternalURL    string
	Source         string
	Version        int
}

type menuNode struct {
	ID             ids.ID  `json:"id"`
	Code           string  `json:"code"`
	ParentID       *ids.ID `json:"parentId"`
	Type           string  `json:"type"`
	Title          string  `json:"title"`
	Path           string  `json:"path"`
	Component      string  `json:"component"`
	Icon           string  `json:"icon"`
	Sort           int     `json:"sort"`
	Hidden         bool    `json:"hidden"`
	KeepAlive      bool    `json:"keepAlive"`
	PermissionCode string  `json:"permissionCode"`
	ExternalURL    string  `json:"externalUrl"`
	Source         string  `json:"source"`
	Version        int     `json:"version"`
	// Permissions are the button-level codes grouped under this menu
	// (read-only; they are declared by modules).
	Permissions []menuPermission `json:"permissions"`
	Children    []*menuNode      `json:"children"`
}

type menuPermission struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func loadMenus(ctx context.Context, db *gorm.DB) ([]menuRow, error) {
	var rows []menuRow
	err := db.WithContext(ctx).Table("sys_menus").Order("sort, id").Scan(&rows).Error
	return rows, err
}

func (r menuRow) node() *menuNode {
	code := ""
	if r.PermissionCode != nil {
		code = *r.PermissionCode
	}
	return &menuNode{
		ID: ids.ID(r.ID), Code: r.Code, ParentID: optionalID(r.ParentID), Type: r.Type, Title: r.Title, Path: r.Path,
		Component: r.Component, Icon: r.Icon, Sort: r.Sort, Hidden: r.Hidden, KeepAlive: r.KeepAlive,
		PermissionCode: code, ExternalURL: r.ExternalURL, Source: r.Source, Version: r.Version,
		Permissions: []menuPermission{}, Children: []*menuNode{},
	}
}

func (m *Module) menuTree(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	rows, err := loadMenus(ctx, m.db)
	if err != nil {
		return nil, err
	}
	var perms []struct {
		MenuCode string
		Code     string
		Name     string
	}
	if err := m.db.WithContext(ctx).Raw(`SELECT menu_code, code, name FROM sys_permissions
		WHERE status = 'active' AND menu_code IS NOT NULL ORDER BY module, sort, code`).Scan(&perms).Error; err != nil {
		return nil, err
	}
	byMenu := map[string][]menuPermission{}
	for _, p := range perms {
		byMenu[p.MenuCode] = append(byMenu[p.MenuCode], menuPermission{Code: p.Code, Name: p.Name})
	}
	nodes := map[int64]*menuNode{}
	for _, r := range rows {
		n := r.node()
		if ps, ok := byMenu[r.Code]; ok {
			n.Permissions = ps
		}
		nodes[r.ID] = n
	}
	roots := []*menuNode{}
	for _, r := range rows {
		if r.ParentID != nil {
			if p, ok := nodes[*r.ParentID]; ok {
				p.Children = append(p.Children, nodes[r.ID])
				continue
			}
		}
		roots = append(roots, nodes[r.ID])
	}
	return roots, nil
}

type menuRequest struct {
	ParentID    *ids.ID `json:"parentId"`
	Type        string  `json:"type"`
	Title       string  `json:"title" binding:"required,max=64"`
	Path        string  `json:"path" binding:"max=255"`
	Icon        string  `json:"icon" binding:"max=64"`
	Sort        int     `json:"sort"`
	Hidden      bool    `json:"hidden"`
	ExternalURL string  `json:"externalUrl" binding:"max=512"`
	Version     *int    `json:"version"`
}

// validateCustom checks the fields administrators control on custom menus.
func (r menuRequest) validateCustom() error {
	switch r.Type {
	case "dir":
		if !strings.HasPrefix(r.Path, "/") {
			return fieldError("path", "目录路径须以 / 开头")
		}
	case "link":
		u, err := url.Parse(r.ExternalURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fieldError("externalUrl", "外链须为 http(s) 地址")
		}
	default:
		return fieldError("type", "自定义菜单只能是目录或外链")
	}
	return nil
}

func (m *Module) loadMenu(tx *gorm.DB, id int64) (menuRow, error) {
	var rows []menuRow
	if err := tx.Table("sys_menus").Where("id = ?", id).Scan(&rows).Error; err != nil {
		return menuRow{}, err
	}
	if len(rows) == 0 {
		return menuRow{}, httpx.NotFound("菜单不存在")
	}
	return rows[0], nil
}

func (m *Module) createMenu(c *gin.Context) (any, error) {
	var req menuRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if err := req.validateCustom(); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	var id int64
	err := database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		if err := m.checkMenuParent(tx, 0, req.ParentID); err != nil {
			return err
		}
		code, err := customMenuCode()
		if err != nil {
			return err
		}
		uid := actor.From(ctx).UserID()
		return tx.Raw(`INSERT INTO sys_menus (code, parent_id, type, title, path, icon, sort, hidden, external_url,
				source, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'custom', ?, ?) RETURNING id`,
			code, int64Ptr(req.ParentID), req.Type, req.Title, req.Path, req.Icon, req.Sort, req.Hidden,
			req.ExternalURL, uid, uid).Scan(&id).Error
	})
	if err != nil {
		return nil, err
	}
	row, err := m.loadMenu(m.db.WithContext(ctx), id)
	if err != nil {
		return nil, err
	}
	return row.node(), nil
}

func customMenuCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "custom." + hex.EncodeToString(b), nil
}

// maxMenuDepth bounds parent walks even if a cycle exists in the data.
const maxMenuDepth = 32

// checkMenuParent rejects missing parents, non-directory parents and cycles.
func (m *Module) checkMenuParent(tx *gorm.DB, self int64, parentID *ids.ID) error {
	if parentID == nil {
		return nil
	}
	parent, err := m.loadMenu(tx, int64(*parentID))
	if err != nil {
		return fieldError("parentId", "上级菜单不存在")
	}
	if parent.Type != "dir" {
		return fieldError("parentId", "上级菜单必须是目录")
	}
	for depth, node := 0, parent; ; depth++ {
		if depth > maxMenuDepth {
			return fieldError("parentId", "菜单层级过深或存在循环")
		}
		if node.ID == self {
			return fieldError("parentId", "不能移动到自身或下级菜单之下")
		}
		if node.ParentID == nil {
			return nil
		}
		if node, err = m.loadMenu(tx, *node.ParentID); err != nil {
			return err
		}
	}
}

func (m *Module) updateMenu(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req menuRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if req.Version == nil {
		return nil, fieldError("version", "不能为空")
	}
	ctx := c.Request.Context()
	err = database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		current, err := m.loadMenu(tx, id)
		if err != nil {
			return err
		}
		sets := map[string]any{"title": req.Title, "icon": req.Icon, "sort": req.Sort, "hidden": req.Hidden,
			"updated_by": actor.From(ctx).UserID()}
		if current.Source == "custom" {
			req.Type = current.Type
			if err := req.validateCustom(); err != nil {
				return err
			}
			if err := m.checkMenuParent(tx, id, req.ParentID); err != nil {
				return err
			}
			sets["parent_id"] = int64Ptr(req.ParentID)
			sets["path"] = req.Path
			sets["external_url"] = req.ExternalURL
		}
		return updateVersioned(tx, "sys_menus", id, *req.Version, sets, "")
	})
	if err != nil {
		return nil, err
	}
	row, err := m.loadMenu(m.db.WithContext(ctx), id)
	if err != nil {
		return nil, err
	}
	return row.node(), nil
}

func (m *Module) deleteMenu(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	return nil, database.RunTx(c.Request.Context(), m.db, func(tx *gorm.DB) error {
		current, err := m.loadMenu(tx, id)
		if err != nil {
			return err
		}
		if current.Source != "custom" {
			return protected("模块菜单不能删除，可以隐藏")
		}
		children, err := exists(tx, "SELECT 1 FROM sys_menus WHERE parent_id = ?", id)
		if err != nil {
			return err
		}
		if children {
			return inUse("菜单下存在子菜单，不能删除")
		}
		return tx.Exec("DELETE FROM sys_menus WHERE id = ?", id).Error
	})
}

type permItem struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Requires []string `json:"requires"`
}

type permNode struct {
	MenuCode    string      `json:"menuCode"`
	Title       string      `json:"title"`
	Type        string      `json:"type"`
	Permissions []permItem  `json:"permissions"`
	Children    []*permNode `json:"children"`
}

// permissionTree groups active codes under the menus that declare them, for
// the role grant UI. Codes without a menu are listed under "其他".
func (m *Module) permissionTree(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	rows, err := loadMenus(ctx, m.db)
	if err != nil {
		return nil, err
	}
	var perms []struct {
		Code     string
		Name     string
		MenuCode *string
		Requires database.TextArray
	}
	if err := m.db.WithContext(ctx).Raw(`SELECT code, name, menu_code, requires FROM sys_permissions
		WHERE status = 'active' ORDER BY module, sort, code`).Scan(&perms).Error; err != nil {
		return nil, err
	}
	byID := map[int64]*permNode{}
	byCode := map[string]*permNode{}
	for _, r := range rows {
		n := &permNode{MenuCode: r.Code, Title: r.Title, Type: r.Type, Permissions: []permItem{}, Children: []*permNode{}}
		byID[r.ID], byCode[r.Code] = n, n
	}
	other := &permNode{MenuCode: "", Title: "其他", Type: "dir", Permissions: []permItem{}, Children: []*permNode{}}
	for _, p := range perms {
		item := permItem{Code: p.Code, Name: p.Name, Requires: []string(p.Requires)}
		if p.MenuCode != nil && byCode[*p.MenuCode] != nil {
			byCode[*p.MenuCode].Permissions = append(byCode[*p.MenuCode].Permissions, item)
		} else {
			other.Permissions = append(other.Permissions, item)
		}
	}
	roots := []*permNode{}
	for _, r := range rows {
		if r.ParentID != nil && byID[*r.ParentID] != nil {
			byID[*r.ParentID].Children = append(byID[*r.ParentID].Children, byID[r.ID])
		} else {
			roots = append(roots, byID[r.ID])
		}
	}
	if len(other.Permissions) > 0 {
		roots = append(roots, other)
	}
	return pruneEmpty(roots), nil
}

func pruneEmpty(nodes []*permNode) []*permNode {
	out := []*permNode{}
	for _, n := range nodes {
		n.Children = pruneEmpty(n.Children)
		if len(n.Permissions) > 0 || len(n.Children) > 0 {
			out = append(out, n)
		}
	}
	return out
}
