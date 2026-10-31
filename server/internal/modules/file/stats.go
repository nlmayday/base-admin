package file

import (
	"path/filepath"

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
	byKind, err := usage(db, `SELECT kind AS key, count(*) AS count, COALESCE(sum(size), 0) AS size
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
	if view.TopExtensions, err = usage(db, `SELECT ext AS key, count(*) AS count, COALESCE(sum(size), 0) AS size
		FROM sys_files WHERE deleted_at IS NULL GROUP BY ext ORDER BY count(*) DESC, ext LIMIT ?`, topExtensions); err != nil {
		return nil, err
	}
	if view.Daily, err = usage(db, `SELECT to_char(d, 'YYYY-MM-DD') AS key, count(f.id) AS count, COALESCE(sum(f.size), 0) AS size
		FROM generate_series(current_date - (? - 1), current_date, interval '1 day') AS d
		LEFT JOIN sys_files f ON f.deleted_at IS NULL AND f.created_at >= d AND f.created_at < d + interval '1 day'
		GROUP BY d ORDER BY d`, trendDays); err != nil {
		return nil, err
	}
	return view, nil
}

func usage(db *gorm.DB, query string, args ...any) ([]usageStat, error) {
	out := []usageStat{}
	if err := db.Raw(query, args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
