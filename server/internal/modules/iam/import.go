package iam

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"golang.org/x/text/encoding/simplifiedchinese"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
)

// User import: a CSV file (UTF-8, or GBK as saved by Excel on Chinese
// Windows) with one user per row. Every row is checked first; any error
// rejects the whole file with per-row messages, otherwise all users are
// created in one transaction with the same initial password, which each
// user must change at first login.

const (
	maxImportRows   = 1000
	maxImportBytes  = 2 << 20
	maxImportErrors = 100
)

// importColumns are the template columns; a header cell matches by the
// text before any "（" hint.
var importColumns = []string{
	"用户名（必填）", "昵称", "手机号", "邮箱",
	"部门（名称；重名时写路径，如 总公司/研发部）",
	"角色（编码或名称，多个用 | 分隔）",
	"岗位（编码或名称，多个用 | 分隔）",
}

const (
	colUsername = iota
	colNickname
	colPhone
	colEmail
	colDept
	colRoles
	colPositions
)

var importValidate = validator.New()

// importRowError is one rejected row; Row is the line number in the file.
type importRowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type importErrorDetails struct {
	Rows []importRowError `json:"rows"`
}

type importResult struct {
	Created int `json:"created"`
}

type importUser struct {
	line        int
	username    string
	nickname    string
	phone       string
	email       string
	deptID      *int64
	roleIDs     []int64
	positionIDs []int64
}

func (m *Module) importTemplate(c *gin.Context) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="users-import-template.csv"`)
	_, _ = c.Writer.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(c.Writer)
	_ = w.Write(importColumns)
	w.Flush()
}

func (m *Module) importUsers(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	password := c.PostForm("password")
	if err := m.core.ValidatePassword(ctx, "password", password); err != nil {
		return nil, err
	}
	records, err := readImportFile(c)
	if err != nil {
		return nil, err
	}
	hash, err := m.core.HashPassword(ctx, password)
	if err != nil {
		return nil, err
	}
	var created int
	err = database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		users, rowErrs, err := resolveImport(ctx, tx, records)
		if err != nil {
			return err
		}
		if len(rowErrs) > 0 {
			return importRejected(rowErrs)
		}
		for _, u := range users {
			if err := insertImported(ctx, tx, u, hash); err != nil {
				return err
			}
		}
		created = len(users)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return importResult{Created: created}, nil
}

func importRejected(rows []importRowError) error {
	total := len(rows)
	if total > maxImportErrors {
		rows = rows[:maxImportErrors]
	}
	return httpx.NewError(http.StatusUnprocessableEntity, httpx.CodeValidation,
		fmt.Sprintf("导入数据有误（%d 处），未导入任何用户", total)).
		WithDetails(importErrorDetails{Rows: rows})
}

// readImportFile returns the data rows of the uploaded file, without the
// header and blank rows, each with its line number.
func readImportFile(c *gin.Context) ([]importRecord, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, fieldError("file", "请选择要导入的 CSV 文件")
	}
	if fh.Size > maxImportBytes {
		return nil, fieldError("file", "文件不能超过 2 MB")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxImportBytes+1))
	if err != nil {
		return nil, err
	}
	return parseImportCSV(raw)
}

type importRecord struct {
	line   int
	fields []string
}

func parseImportCSV(raw []byte) ([]importRecord, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(raw) {
		decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw)
		if err != nil {
			return nil, fieldError("file", "无法识别文件编码，请另存为 UTF-8 CSV")
		}
		raw = decoded
	}
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return nil, fieldError("file", "文件为空或不是 CSV 格式")
	}
	index, err := importHeaderIndex(header)
	if err != nil {
		return nil, err
	}
	var out []importRecord
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fieldError("file", "CSV 格式错误："+err.Error())
		}
		line, _ := r.FieldPos(0)
		fields := pickColumns(rec, index)
		if strings.Join(fields, "") == "" {
			continue
		}
		if len(out) == maxImportRows {
			return nil, fieldError("file", fmt.Sprintf("一次最多导入 %d 个用户", maxImportRows))
		}
		out = append(out, importRecord{line: line, fields: fields})
	}
	if len(out) == 0 {
		return nil, fieldError("file", "文件中没有用户数据")
	}
	return out, nil
}

// importHeaderIndex maps each template column to its position in header.
func importHeaderIndex(header []string) ([]int, error) {
	index := make([]int, len(importColumns))
	for i, col := range importColumns {
		index[i] = -1
		name, _, _ := strings.Cut(col, "（")
		for j, h := range header {
			if strings.HasPrefix(strings.TrimSpace(h), name) {
				index[i] = j
				break
			}
		}
	}
	if index[colUsername] < 0 {
		return nil, fieldError("file", "缺少「用户名」列，请使用导入模板")
	}
	return index, nil
}

func pickColumns(rec []string, index []int) []string {
	out := make([]string, len(index))
	for i, j := range index {
		if j >= 0 && j < len(rec) {
			out[i] = strings.TrimSpace(rec[j])
		}
	}
	return out
}

// insertImported creates one validated user like createUser does.
func insertImported(ctx context.Context, tx *gorm.DB, u importUser, hash string) error {
	uid := actor.From(ctx).UserID()
	id, err := database.InsertID(tx, `INSERT INTO sys_users (username, password_hash, nickname, email, phone, dept_id, status,
			must_change_password, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, true, ?, ?)`,
		u.username, hash, u.nickname, u.email, u.phone, u.deptID, actor.StatusEnabled, uid, uid)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return importRejected([]importRowError{{Row: u.line, Message: "用户名已存在"}})
		}
		return err
	}
	if err := replaceUserLinks(tx, id, u.roleIDs, u.positionIDs); err != nil {
		return err
	}
	if err := ensureOutranks(ctx, tx, id); err != nil {
		if errors.Is(err, errEscalation) {
			return importRejected([]importRowError{{Row: u.line, Message: "无权分配这些角色"}})
		}
		return err
	}
	return nil
}
