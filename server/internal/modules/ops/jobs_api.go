package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

const maxParamsBytes = 8 * 1024

type jobView struct {
	ID             ids.ID          `json:"id"`
	Name           string          `json:"name"`
	HandlerKey     string          `json:"handlerKey"`
	HandlerName    string          `json:"handlerName"`
	HandlerExists  bool            `json:"handlerExists"`
	CronExpr       string          `json:"cronExpr"`
	Params         json.RawMessage `json:"params"`
	Enabled        bool            `json:"enabled"`
	TimeoutSeconds int             `json:"timeoutSeconds"`
	MaxRetries     int             `json:"maxRetries"`
	Remark         string          `json:"remark"`
	Version        int             `json:"version"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	LastRunAt      *time.Time      `json:"lastRunAt"`
	// LastStatus is "" (never run), running, success, failed, timeout or skipped.
	LastStatus string `json:"lastStatus"`
	// NextRunAt is null for disabled jobs.
	NextRunAt *time.Time `json:"nextRunAt"`
}

func (m *Module) view(r jobRow, now time.Time) jobView {
	h, ok := m.handlers.get(r.HandlerKey)
	v := jobView{
		ID: ids.ID(r.ID), Name: r.Name, HandlerKey: r.HandlerKey, HandlerName: h.Name, HandlerExists: ok,
		CronExpr: r.CronExpr, Params: json.RawMessage(r.Params), Enabled: r.Enabled,
		TimeoutSeconds: r.TimeoutSeconds, MaxRetries: r.MaxRetries, Remark: r.Remark, Version: r.Version,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LastRunAt: r.LastRunAt, LastStatus: r.LastStatus,
	}
	if r.Enabled {
		v.NextRunAt = nextRun(r.CronExpr, now)
	}
	return v
}

type jobFilter struct {
	Keyword    string `form:"keyword"`
	HandlerKey string `form:"handlerKey"`
	Enabled    string `form:"enabled"`
}

func (m *Module) listJobs(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	q := m.db.WithContext(c.Request.Context()).Table("sys_jobs")
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		q = q.Where("LOWER(name) LIKE LOWER(?) OR LOWER(remark) LIKE LOWER(?)", like, like)
	}
	if hk := c.Query("handlerKey"); hk != "" {
		q = q.Where("handler_key = ?", hk)
	}
	switch c.Query("enabled") {
	case "":
	case "true":
		q = q.Where("enabled")
	case "false":
		q = q.Where("NOT enabled")
	default:
		return nil, fieldError("enabled", "取值必须是 true 或 false")
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []jobRow
	if err := q.Select(jobColumns).Order("id").Offset(pq.Offset()).Limit(pq.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	views := make([]jobView, len(rows))
	for i, r := range rows {
		views[i] = m.view(r, now)
	}
	return httpx.Page[jobView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func (m *Module) loadJobView(c *gin.Context, id int64) (jobView, error) {
	row, found, err := m.sched.loadJob(c.Request.Context(), id)
	if err != nil {
		return jobView{}, err
	}
	if !found {
		return jobView{}, httpx.NotFound("定时任务不存在")
	}
	return m.view(row, time.Now()), nil
}

func (m *Module) getJob(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	return m.loadJobView(c, id)
}

type jobRequest struct {
	Name           string          `json:"name" binding:"required,max=64"`
	HandlerKey     string          `json:"handlerKey" binding:"required,max=64"`
	CronExpr       string          `json:"cronExpr" binding:"required,max=128"`
	Params         json.RawMessage `json:"params"`
	Enabled        *bool           `json:"enabled"`
	TimeoutSeconds int             `json:"timeoutSeconds" binding:"min=0,max=86400"`
	MaxRetries     int             `json:"maxRetries" binding:"min=0,max=10"`
	Remark         string          `json:"remark" binding:"max=255"`
	// Version is required on update.
	Version *int `json:"version"`
}

// validate checks what binding tags cannot and returns normalized params.
func (m *Module) validate(req *jobRequest) (string, error) {
	var fields []httpx.FieldError
	add := func(field, msg string) { fields = append(fields, httpx.FieldError{Field: field, Message: msg}) }
	req.Name = strings.TrimSpace(req.Name)
	req.CronExpr = strings.TrimSpace(req.CronExpr)
	if req.Name == "" {
		add("name", "不能为空")
	}
	if _, ok := m.handlers.get(req.HandlerKey); !ok {
		add("handlerKey", "任务处理器未注册")
	}
	if sched, err := parseCron(req.CronExpr); err != nil {
		add("cronExpr", err.Error())
	} else if e, ok := sched.(alignedEvery); ok && e.every < m.sched.tuning.MinEvery {
		add("cronExpr", "@every 间隔不能小于 "+m.sched.tuning.MinEvery.String())
	}
	params, msg := normalizeParams(req.Params)
	if msg != "" {
		add("params", msg)
	}
	if len(fields) > 0 {
		return "", httpx.Validation(fields)
	}
	return params, nil
}

// normalizeParams requires a JSON object; empty or null becomes {}.
func normalizeParams(raw json.RawMessage) (string, string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "{}", ""
	}
	if len(trimmed) > maxParamsBytes {
		return "", "参数不能超过 8 KB"
	}
	var obj map[string]any
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return "", "参数必须是 JSON 对象"
	}
	return string(trimmed), ""
}

func (m *Module) createJob(c *gin.Context) (any, error) {
	var req jobRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	params, err := m.validate(&req)
	if err != nil {
		return nil, err
	}
	enabled := req.Enabled == nil || *req.Enabled
	ctx := c.Request.Context()
	uid := actor.From(ctx).UserID()
	id, err := database.InsertID(m.db.WithContext(ctx), `INSERT INTO sys_jobs (name, handler_key, cron_expr, params, enabled, timeout_seconds,
		max_retries, remark, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Name, req.HandlerKey, req.CronExpr, params, enabled, req.TimeoutSeconds, req.MaxRetries, req.Remark, uid, uid)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return nil, fieldError("name", "任务名称已存在")
		}
		return nil, err
	}
	m.sched.notifyChanged(ctx)
	return m.loadJobView(c, id)
}

func (m *Module) updateJob(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req jobRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if req.Version == nil {
		return nil, fieldError("version", "不能为空")
	}
	params, err := m.validate(&req)
	if err != nil {
		return nil, err
	}
	if _, err := m.loadJobView(c, id); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	sets := map[string]any{
		"name": req.Name, "handler_key": req.HandlerKey, "cron_expr": req.CronExpr,
		"params": params, "timeout_seconds": req.TimeoutSeconds,
		"max_retries": req.MaxRetries, "remark": req.Remark, "updated_by": actor.From(ctx).UserID(),
	}
	if req.Enabled != nil {
		sets["enabled"] = *req.Enabled
	}
	if err := updateVersioned(m.db.WithContext(ctx), "sys_jobs", id, *req.Version, sets, "name", "任务名称已存在"); err != nil {
		return nil, err
	}
	m.sched.notifyChanged(ctx)
	return m.loadJobView(c, id)
}

type jobEnabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
	Version *int  `json:"version" binding:"required"`
}

func (m *Module) setJobEnabled(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req jobEnabledRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if _, err := m.loadJobView(c, id); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	err = updateVersioned(m.db.WithContext(ctx), "sys_jobs", id, *req.Version, map[string]any{
		"enabled": *req.Enabled, "updated_by": actor.From(ctx).UserID(),
	}, "", "")
	if err != nil {
		return nil, err
	}
	m.sched.notifyChanged(ctx)
	return m.loadJobView(c, id)
}

func (m *Module) deleteJob(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	res := m.db.WithContext(ctx).Exec("DELETE FROM sys_jobs WHERE id = ?", id)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, httpx.NotFound("定时任务不存在")
	}
	m.sched.notifyChanged(ctx)
	return nil, nil
}

type runNowView struct {
	RunID  ids.ID `json:"runId"`
	Status string `json:"status"`
}

// runJobNow starts a manual run asynchronously. It honours the job lock:
// when the job is already running the run is recorded as skipped.
func (m *Module) runJobNow(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	job, found, err := m.sched.loadJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, httpx.NotFound("定时任务不存在")
	}
	h, ok := m.handlers.get(job.HandlerKey)
	if !ok {
		return nil, errHandlerMissing(job.HandlerKey)
	}
	done, ok := m.sched.track()
	if !ok {
		return nil, errStopping()
	}
	uid := actor.From(ctx).UserID()
	// Detached from the request: once the run row is written, a client
	// disconnect must not leave it "running" with the lock released.
	beginCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dbTimeout)
	defer cancel()
	run, err := m.sched.begin(beginCtx, job, h, triggerManual, nil, &uid)
	if err != nil || run.exec == nil {
		done()
		if err != nil {
			return nil, httpx.Unavailable("任务启动失败，请稍后再试").WithCause(err)
		}
		return runNowView{RunID: ids.ID(run.id), Status: run.status}, nil
	}
	go func() {
		defer done()
		run.exec()
	}()
	return runNowView{RunID: ids.ID(run.id), Status: run.status}, nil
}
