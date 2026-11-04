package ops

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type handlerView struct {
	Key                   string `json:"key"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	DefaultTimeoutSeconds int    `json:"defaultTimeoutSeconds"`
}

func (m *Module) listHandlers(*gin.Context) (any, error) {
	hs := m.handlers.list()
	out := make([]handlerView, len(hs))
	for i, h := range hs {
		out[i] = handlerView{Key: h.Key, Name: h.Name, Description: h.Description,
			DefaultTimeoutSeconds: int(effectiveTimeout(0, h) / time.Second)}
	}
	return out, nil
}

type cronPreviewRequest struct {
	Expr string `json:"expr" binding:"required,max=128"`
}

type cronPreviewView struct {
	Expr      string      `json:"expr"`
	NextTimes []time.Time `json:"nextTimes"`
}

func (m *Module) previewCron(c *gin.Context) (any, error) {
	var req cronPreviewRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	sched, err := parseCron(req.Expr)
	if err != nil {
		return nil, fieldError("expr", err.Error())
	}
	return cronPreviewView{Expr: req.Expr, NextTimes: nextTimes(sched, time.Now(), previewCount)}, nil
}

type runView struct {
	ID          ids.ID     `json:"id"`
	JobID       *ids.ID    `json:"jobId"`
	JobName     string     `json:"jobName"`
	HandlerKey  string     `json:"handlerKey"`
	Trigger     string     `json:"trigger"`
	Status      string     `json:"status"`
	Attempt     int        `json:"attempt"`
	ScheduledAt *time.Time `json:"scheduledAt"`
	StartedAt   time.Time  `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
	DurationMs  *int64     `json:"durationMs"`
	Error       string     `json:"error"`
	Instance    string     `json:"instance"`
	TriggeredBy *ids.ID    `json:"triggeredBy"`
}

type runRow struct {
	ID          int64
	JobID       *int64
	JobName     string
	HandlerKey  string
	Trigger     string
	Status      string
	Attempt     int
	ScheduledAt *time.Time
	StartedAt   time.Time
	FinishedAt  *time.Time
	DurationMs  *int64
	Error       string
	Instance    string
	TriggeredBy *int64
}

func optID(p *int64) *ids.ID {
	if p == nil {
		return nil
	}
	id := ids.ID(*p)
	return &id
}

func (r runRow) view() runView {
	return runView{ID: ids.ID(r.ID), JobID: optID(r.JobID), JobName: r.JobName, HandlerKey: r.HandlerKey,
		Trigger: r.Trigger, Status: r.Status, Attempt: r.Attempt, ScheduledAt: r.ScheduledAt,
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, DurationMs: r.DurationMs, Error: r.Error,
		Instance: r.Instance, TriggeredBy: optID(r.TriggeredBy)}
}

const runColumns = `id, job_id, job_name, handler_key, "trigger", status, attempt, scheduled_at, started_at,
	finished_at, duration_ms, error, instance, triggered_by`

type runFilter struct {
	JobID     string `form:"jobId"`
	Status    string `form:"status"`
	Trigger   string `form:"trigger"`
	StartFrom string `form:"startFrom"` // RFC 3339, inclusive
	StartTo   string `form:"startTo"`   // RFC 3339, exclusive
}

var (
	runStatuses = map[string]bool{statusRunning: true, statusSuccess: true, statusFailed: true, statusTimeout: true, statusSkipped: true}
	runTriggers = map[string]bool{triggerCron: true, triggerManual: true}
)

// applyRunFilter validates the filter and adds its conditions to q.
func applyRunFilter(q *gorm.DB, f runFilter) (*gorm.DB, error) {
	var fields []httpx.FieldError
	add := func(field, msg string) { fields = append(fields, httpx.FieldError{Field: field, Message: msg}) }
	if f.JobID != "" {
		if id, err := ids.Parse(f.JobID); err != nil {
			add("jobId", "任务 ID 无效")
		} else {
			q = q.Where("job_id = ?", int64(id))
		}
	}
	if f.Status != "" {
		if !runStatuses[f.Status] {
			add("status", "状态无效")
		}
		q = q.Where("status = ?", f.Status)
	}
	if f.Trigger != "" {
		if !runTriggers[f.Trigger] {
			add("trigger", "触发方式无效")
		}
		q = q.Where(`"trigger" = ?`, f.Trigger)
	}
	for _, t := range []struct{ field, value, cond string }{
		{"startFrom", f.StartFrom, "started_at >= ?"}, {"startTo", f.StartTo, "started_at < ?"},
	} {
		if t.value == "" {
			continue
		}
		v, err := time.Parse(time.RFC3339, t.value)
		if err != nil {
			add(t.field, "时间格式无效，应为 RFC 3339")
			continue
		}
		q = q.Where(t.cond, v)
	}
	if len(fields) > 0 {
		return nil, httpx.Validation(fields)
	}
	return q, nil
}

func (m *Module) listRuns(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	var f runFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		return nil, httpx.BadRequest("查询参数格式错误").WithCause(err)
	}
	q, err := applyRunFilter(m.db.WithContext(c.Request.Context()).Table("sys_job_runs"), f)
	if err != nil {
		return nil, err
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []runRow
	if err := q.Select(runColumns).Order("started_at DESC, id DESC").Offset(pq.Offset()).Limit(pq.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]runView, len(rows))
	for i, r := range rows {
		views[i] = r.view()
	}
	return httpx.Page[runView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func (m *Module) getRun(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var rows []runRow
	if err := m.db.WithContext(c.Request.Context()).Raw("SELECT "+runColumns+" FROM sys_job_runs WHERE id = ?", id).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, httpx.NotFound("执行记录不存在")
	}
	return rows[0].view(), nil
}
