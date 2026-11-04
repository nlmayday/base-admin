// Package export holds the rules every CSV export follows: a row limit set
// in 基础设置 (larger exports are refused, never cut short), and times in
// UTC, said so in the header.
package export

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"

	"base-admin/server/internal/platform/httpx"
)

// SettingKey is the system setting with the row limit of one export.
const SettingKey = "export.max_rows"

// Limits of the setting; DefaultMaxRows applies while it is unset or
// unreadable.
const (
	DefaultMaxRows = 100000
	MinMaxRows     = 1000
	MaxMaxRows     = 1000000
)

// MaxRows reads the row limit from sys_configs.
func MaxRows(ctx context.Context, db *gorm.DB) int {
	var values []string
	err := db.WithContext(ctx).Raw(`SELECT value FROM sys_configs WHERE "key" = ?`, SettingKey).Scan(&values).Error
	if err != nil || len(values) == 0 {
		return DefaultMaxRows
	}
	n, err := strconv.Atoi(values[0])
	if err != nil || n < MinMaxRows || n > MaxMaxRows {
		return DefaultMaxRows
	}
	return n
}

// Check counts the rows of q and refuses an export larger than the limit;
// it returns the limit, to bound the read that follows.
func Check(ctx context.Context, db, q *gorm.DB) (int, error) {
	maxRows := MaxRows(ctx, db)
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return 0, err
	}
	if total > int64(maxRows) {
		return 0, httpx.NewError(http.StatusUnprocessableEntity, httpx.CodeValidation,
			fmt.Sprintf("数据量过大（%d 条，导出上限 %d 条），请缩小查询范围", total, maxRows))
	}
	return maxRows, nil
}

// TimeHeader labels the header of a time column.
func TimeHeader(label string) string { return label + "(UTC)" }

// Time formats a time cell in UTC.
func Time(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.DateTime)
}
