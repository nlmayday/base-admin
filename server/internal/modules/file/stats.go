package file

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/config"
)

const (
	topExtensions = 8
	trendDays     = 14
)

// storageLocation describes where objects live. It never contains
// credentials.
type storageLocation struct {
	Dir          string `json:"dir,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	Region       string `json:"region,omitempty"`
	UseSSL       bool   `json:"useSSL"`
	UsePathStyle bool   `json:"usePathStyle"`
}

type usageStat struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
	Size  int64  `json:"size"`
}

type storageView struct {
	Driver   string          `json:"driver"`
	Location storageLocation `json:"location"`
	// MaxUploadBytes is the configured hard ceiling.
	MaxUploadBytes   int64 `json:"maxUploadBytes"`
	SignedURLTTLSecs int64 `json:"signedUrlTtlSeconds"`
	// Policy is the effective upload policy; null when it is unavailable.
	Policy        *policyView `json:"policy"`
	FileCount     int64       `json:"fileCount"`
	TotalSize     int64       `json:"totalSize"`
	CategoryCount int64       `json:"categoryCount"`
	// ByKind always lists every kind; Key is the kind.
	ByKind []usageStat `json:"byKind"`
	// TopExtensions lists the largest extensions by count; Key is the ext.
	TopExtensions []usageStat `json:"topExtensions"`
	// Daily lists uploads per day for the last 14 days; Key is YYYY-MM-DD.
	Daily []usageStat `json:"daily"`
}

func (m *Module) location() storageLocation {
	if m.cfg.Driver == config.FileDriverS3 {
		s := m.cfg.S3
		return storageLocation{Endpoint: s.Endpoint, Bucket: s.Bucket, Region: s.Region, UseSSL: s.UseSSL, UsePathStyle: s.UsePathStyle}
	}
	dir := m.cfg.LocalDir
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return storageLocation{Dir: dir}
}

func (m *Module) storageStats(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	db := m.db.WithContext(ctx)
	view := storageView{
		Driver: m.storage.Name(), Location: m.location(), MaxUploadBytes: m.cfg.MaxUploadBytes,
		SignedURLTTLSecs: int64(m.cfg.SignedURLTTL.Seconds()),
	}
	if p, err := m.effectivePolicy(ctx); err == nil {
		view.Policy = &policyView{Extensions: p.exts, MaxBytes: p.maxBytes}
	} else {
		m.log.Warn("upload policy unavailable", zap.Error(err))
	}
	if err := db.Raw("SELECT count(*) FROM sys_file_categories").Scan(&view.CategoryCount).Error; err != nil {
		return nil, err
	}
	byKind, err := usage(db, `SELECT kind AS "key", count(*) AS count, COALESCE(sum(size), 0) AS size
		FROM sys_files WHERE deleted_at IS NULL GROUP BY kind`)
	if err != nil {
		return nil, err
	}
	view.ByKind = make([]usageStat, len(allKinds))
	for i, k := range allKinds {
		view.ByKind[i] = usageStat{Key: k}
		for _, s := range byKind {
			if s.Key == k {
				view.ByKind[i] = s
			}
		}
		view.FileCount += view.ByKind[i].Count
		view.TotalSize += view.ByKind[i].Size
	}
	if view.TopExtensions, err = usage(db, `SELECT ext AS "key", count(*) AS count, COALESCE(sum(size), 0) AS size
		FROM sys_files WHERE deleted_at IS NULL GROUP BY ext ORDER BY count(*) DESC, ext LIMIT ?`, topExtensions); err != nil {
		return nil, err
	}
	if view.Daily, err = m.dailyUsage(db); err != nil {
		return nil, err
	}
	return view, nil
}

// dailyUsage counts the uploads of each of the last trendDays calendar days
// in the server's time zone, oldest first, including days without uploads.
func (m *Module) dailyUsage(db *gorm.DB) ([]usageStat, error) {
	now := m.now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	bounds := make([]time.Time, trendDays+1)
	for i := range bounds {
		bounds[i] = today.AddDate(0, 0, i-trendDays+1)
	}
	var b strings.Builder
	args := make([]any, 0, trendDays+2)
	b.WriteString("CASE")
	for i := range trendDays {
		fmt.Fprintf(&b, " WHEN created_at < ? THEN %d", i)
		args = append(args, bounds[i+1])
	}
	b.WriteString(" END")
	args = append(args, bounds[0], bounds[trendDays])
	var rows []struct{ Day, Count, Size int64 }
	if err := db.Raw(`SELECT `+b.String()+` AS day, count(*) AS count, COALESCE(sum(size), 0) AS size
		FROM sys_files WHERE deleted_at IS NULL AND created_at >= ? AND created_at < ?
		GROUP BY 1`, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]usageStat, trendDays)
	for i := range out {
		out[i].Key = bounds[i].Format(time.DateOnly)
	}
	for _, r := range rows {
		if r.Day >= 0 && r.Day < trendDays {
			out[r.Day].Count, out[r.Day].Size = r.Count, r.Size
		}
	}
	return out, nil
}

func usage(db *gorm.DB, query string, args ...any) ([]usageStat, error) {
	out := []usageStat{}
	if err := db.Raw(query, args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
