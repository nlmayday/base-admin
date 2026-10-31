package iam

import (
	"context"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

// importLookup resolves department, role and position names of an import.
type importLookup struct {
	deptByName map[string][]int64
	deptByPath map[string]int64
	roles      codeNameIndex
	positions  codeNameIndex
	existing   map[string]bool
}

// codeNameIndex finds a row by code, then by a unique name.
type codeNameIndex struct {
	byCode map[string]int64
	byName map[string][]int64
}

func (x codeNameIndex) find(key string) (int64, string) {
	if id, ok := x.byCode[key]; ok {
		return id, ""
	}
	switch ids := x.byName[key]; len(ids) {
	case 0:
		return 0, "不存在"
	case 1:
		return ids[0], ""
	default:
		return 0, "名称重复，请填写编码"
	}
}

func loadCodeNames(tx *gorm.DB, table string) (codeNameIndex, error) {
	var rows []struct {
		ID   int64
		Code string
		Name string
	}
	if err := tx.Table(table).Select("id, code, name").Scan(&rows).Error; err != nil {
		return codeNameIndex{}, err
	}
	x := codeNameIndex{byCode: map[string]int64{}, byName: map[string][]int64{}}
	for _, r := range rows {
		x.byCode[r.Code] = r.ID
		x.byName[r.Name] = append(x.byName[r.Name], r.ID)
	}
	return x, nil
}

func loadImportLookup(tx *gorm.DB, records []importRecord) (*importLookup, error) {
	var depts []struct {
		ID       int64
		ParentID *int64
		Name     string
	}
	if err := tx.Table("sys_departments").Select("id, parent_id, name").Scan(&depts).Error; err != nil {
		return nil, err
	}
	l := &importLookup{deptByName: map[string][]int64{}, deptByPath: map[string]int64{}, existing: map[string]bool{}}
	parent := map[int64]*int64{}
	name := map[int64]string{}
	for _, d := range depts {
		parent[d.ID], name[d.ID] = d.ParentID, d.Name
		l.deptByName[d.Name] = append(l.deptByName[d.Name], d.ID)
	}
	for _, d := range depts {
		parts := []string{d.Name}
		for p := d.ParentID; p != nil && len(parts) < 64; p = parent[*p] {
			parts = append([]string{name[*p]}, parts...)
		}
		l.deptByPath[strings.Join(parts, "/")] = d.ID
	}
	var err error
	if l.roles, err = loadCodeNames(tx, "sys_roles"); err != nil {
		return nil, err
	}
	if l.positions, err = loadCodeNames(tx, "sys_positions"); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(records))
	for _, r := range records {
		names = append(names, strings.ToLower(r.fields[colUsername]))
	}
	var taken []string
	err = tx.Raw(`SELECT lower(username) FROM sys_users WHERE deleted_at IS NULL AND lower(username) IN ?`, names).
		Scan(&taken).Error
	for _, n := range taken {
		l.existing[n] = true
	}
	return l, err
}

func (l *importLookup) dept(v string) (int64, string) {
	if id, ok := l.deptByPath[strings.Trim(v, "/ ")]; ok {
		return id, ""
	}
	switch ids := l.deptByName[v]; len(ids) {
	case 0:
		return 0, "部门「" + v + "」不存在"
	case 1:
		return ids[0], ""
	default:
		return 0, "部门「" + v + "」重名，请写完整路径，如 总公司/" + v
	}
}

// splitList splits "a | b，c" into its non-empty items.
func splitList(v string) []string {
	f := strings.FieldsFunc(v, func(r rune) bool { return r == '|' || r == ',' || r == '，' || r == '、' })
	out := f[:0]
	for _, s := range f {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func resolveImport(ctx context.Context, tx *gorm.DB, records []importRecord) ([]importUser, []importRowError, error) {
	l, err := loadImportLookup(tx, records)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]int{}
	var users []importUser
	var errs []importRowError
	for _, r := range records {
		u, msgs := l.resolveRow(r)
		key := strings.ToLower(u.username)
		if first, dup := seen[key]; dup && key != "" {
			msgs = append(msgs, "用户名与第 "+itoa(int64(first))+" 行重复")
		} else {
			seen[key] = r.line
		}
		if len(msgs) == 0 {
			if err := guardDeptInScope(ctx, tx, u.deptID, "dept"); err != nil {
				msgs = append(msgs, deptScopeMessage(u.deptID))
			}
		}
		for _, msg := range msgs {
			errs = append(errs, importRowError{Row: r.line, Message: msg})
		}
		users = append(users, u)
	}
	return users, errs, nil
}

func deptScopeMessage(deptID *int64) string {
	if deptID == nil {
		return "请填写部门（只能导入到你数据权限范围内的部门）"
	}
	return "部门不在你的数据权限范围内"
}

// resolveRow checks one row and resolves its references.
func (l *importLookup) resolveRow(r importRecord) (importUser, []string) {
	f := r.fields
	u := importUser{line: r.line, username: f[colUsername], nickname: f[colNickname], phone: f[colPhone], email: f[colEmail]}
	var msgs []string
	switch {
	case u.username == "":
		msgs = append(msgs, "用户名不能为空")
	case !usernamePattern.MatchString(u.username):
		msgs = append(msgs, "用户名须为 3–32 位字母、数字或 _ . -")
	case l.existing[strings.ToLower(u.username)]:
		msgs = append(msgs, "用户名「"+u.username+"」已存在")
	}
	if u.nickname == "" {
		u.nickname = u.username
	}
	if utf8.RuneCountInString(u.nickname) > 64 {
		msgs = append(msgs, "昵称不能超过 64 个字符")
	}
	if len(u.phone) > 32 {
		msgs = append(msgs, "手机号不能超过 32 个字符")
	}
	if u.email != "" && (len(u.email) > 128 || importValidate.Var(u.email, "email") != nil) {
		msgs = append(msgs, "邮箱格式不正确")
	}
	if v := f[colDept]; v != "" {
		id, msg := l.dept(v)
		if msg != "" {
			msgs = append(msgs, msg)
		} else {
			u.deptID = &id
		}
	}
	u.roleIDs, msgs = resolveList(l.roles, f[colRoles], "角色", msgs)
	u.positionIDs, msgs = resolveList(l.positions, f[colPositions], "岗位", msgs)
	return u, msgs
}

func resolveList(x codeNameIndex, v, what string, msgs []string) ([]int64, []string) {
	var ids []int64
	for _, item := range splitList(v) {
		id, msg := x.find(item)
		if msg != "" {
			msgs = append(msgs, what+"「"+item+"」"+msg)
			continue
		}
		ids = append(ids, id)
	}
	return uniq(ids), msgs
}
