//go:build integration

package ops

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/httpx"
)

func fieldsOf(t *testing.T, r response) map[string]string {
	t.Helper()
	require.Equal(t, http.StatusUnprocessableEntity, r.Status, string(r.Data))
	details := decode[httpx.ValidationDetails](t, r)
	out := map[string]string{}
	for _, f := range details.Fields {
		out[f.Field] = f.Message
	}
	return out
}

func TestJobCRUD(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", nil)

	// The migration seeds the built-in cleanup job.
	r := a.do(http.MethodGet, "/jobs?handlerKey="+CleanupRunsKey, nil)
	require.Equal(t, http.StatusOK, r.Status)
	seeded := decode[httpx.Page[jobView]](t, r)
	require.Equal(t, int64(1), seeded.Total)
	assert.Equal(t, "清理执行记录", seeded.List[0].HandlerName)
	assert.True(t, seeded.List[0].HandlerExists)
	assert.NotNil(t, seeded.List[0].NextRunAt)

	hs := decode[[]handlerView](t, a.do(http.MethodGet, "/jobs/handlers", nil))
	require.NotEmpty(t, hs)
	assert.Equal(t, CleanupRunsKey, hs[0].Key)
	assert.Equal(t, 600, hs[0].DefaultTimeoutSeconds)

	job := a.createJob(jobBody("每分钟任务", "test.ok", "* * * * *", map[string]any{
		"params": map[string]any{"a": 1}, "timeoutSeconds": 30, "maxRetries": 2, "remark": "r",
	}))
	assert.True(t, job.Enabled)
	assert.JSONEq(t, `{"a":1}`, string(job.Params))
	assert.Equal(t, 0, job.Version)
	assert.Empty(t, job.LastStatus)
	require.NotNil(t, job.NextRunAt)

	// Stale version is rejected, a fresh one applies.
	update := jobBody("每分钟任务2", "test.fail", "@hourly", map[string]any{"version": 5})
	r = a.do(http.MethodPut, "/jobs/"+job.ID.String(), update)
	assert.Equal(t, http.StatusConflict, r.Status)
	update["version"] = 0
	r = a.do(http.MethodPut, "/jobs/"+job.ID.String(), update)
	require.Equal(t, http.StatusOK, r.Status, string(r.Data))
	updated := decode[jobView](t, r)
	assert.Equal(t, "test.fail", updated.HandlerKey)
	assert.Equal(t, "@hourly", updated.CronExpr)
	assert.JSONEq(t, `{}`, string(updated.Params))
	assert.Equal(t, 1, updated.Version)

	// Disable: no next run; stale version conflicts.
	r = a.do(http.MethodPut, "/jobs/"+job.ID.String()+"/status", map[string]any{"enabled": false, "version": 1})
	require.Equal(t, http.StatusOK, r.Status, string(r.Data))
	disabled := decode[jobView](t, r)
	assert.False(t, disabled.Enabled)
	assert.Nil(t, disabled.NextRunAt)
	r = a.do(http.MethodPut, "/jobs/"+job.ID.String()+"/status", map[string]any{"enabled": true, "version": 1})
	assert.Equal(t, http.StatusConflict, r.Status)

	list := decode[httpx.Page[jobView]](t, a.do(http.MethodGet, "/jobs?enabled=false&keyword=每分钟", nil))
	require.Equal(t, int64(1), list.Total)
	assert.Equal(t, job.ID, list.List[0].ID)

	r = a.do(http.MethodDelete, "/jobs/"+job.ID.String(), nil)
	require.Equal(t, http.StatusOK, r.Status)
	assert.Equal(t, http.StatusNotFound, a.do(http.MethodGet, "/jobs/"+job.ID.String(), nil).Status)
	assert.Equal(t, http.StatusNotFound, a.do(http.MethodDelete, "/jobs/"+job.ID.String(), nil).Status)
}

func TestJobValidation(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", nil)

	fields := fieldsOf(t, a.do(http.MethodPost, "/jobs", jobBody("x", "nope.missing", "61 * * * *",
		map[string]any{"params": []int{1}})))
	assert.Contains(t, fields, "handlerKey")
	assert.Contains(t, fields["cronExpr"], "表达式无效")
	assert.Contains(t, fields, "params")

	fields = fieldsOf(t, a.do(http.MethodPost, "/jobs", jobBody("x", "test.ok", "@daily",
		map[string]any{"timeoutSeconds": -1, "maxRetries": 11})))
	assert.Contains(t, fields, "timeoutSeconds")
	assert.Contains(t, fields, "maxRetries")

	fields = fieldsOf(t, a.do(http.MethodPost, "/jobs", map[string]any{"handlerKey": "test.ok"}))
	assert.Contains(t, fields, "name")
	assert.Contains(t, fields, "cronExpr")

	job := a.createJob(jobBody("唯一", "test.ok", "@daily", nil))
	fields = fieldsOf(t, a.do(http.MethodPost, "/jobs", jobBody("唯一", "test.ok", "@daily", nil)))
	assert.Equal(t, "任务名称已存在", fields["name"])
	other := a.createJob(jobBody("另一个", "test.ok", "@daily", nil))
	fields = fieldsOf(t, a.do(http.MethodPut, "/jobs/"+other.ID.String(), jobBody("唯一", "test.ok", "@daily",
		map[string]any{"version": 0})))
	assert.Equal(t, "任务名称已存在", fields["name"])
	fields = fieldsOf(t, a.do(http.MethodPut, "/jobs/"+job.ID.String(), jobBody("唯一", "test.ok", "@daily", nil)))
	assert.Contains(t, fields, "version")

	assert.Equal(t, http.StatusNotFound, a.do(http.MethodPut, "/jobs/999999", jobBody("z", "test.ok", "@daily",
		map[string]any{"version": 0})).Status)
	assert.Equal(t, http.StatusNotFound, a.do(http.MethodGet, "/jobs/abc", nil).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, a.do(http.MethodGet, "/jobs?enabled=maybe", nil).Status)

	// Cron preview.
	r := a.do(http.MethodPost, "/jobs/cron/preview", map[string]any{"expr": "*/5 * * * *"})
	require.Equal(t, http.StatusOK, r.Status)
	preview := decode[cronPreviewView](t, r)
	require.Len(t, preview.NextTimes, 5)
	for i := 1; i < 5; i++ {
		assert.Equal(t, 5*60.0, preview.NextTimes[i].Sub(preview.NextTimes[i-1]).Seconds())
	}
	assert.Contains(t, fieldsOf(t, a.do(http.MethodPost, "/jobs/cron/preview", map[string]any{"expr": "bad"})), "expr")
}

func TestJobPermissions(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", nil)
	job := a.createJob(jobBody("权限", "test.ok", "@daily", nil))
	id := job.ID.String()

	cases := []struct {
		method, path string
		body         any
		need         string
	}{
		{http.MethodGet, "/jobs", nil, PermJobView},
		{http.MethodGet, "/jobs/handlers", nil, PermJobView},
		{http.MethodPost, "/jobs/cron/preview", map[string]any{"expr": "@daily"}, PermJobView},
		{http.MethodGet, "/jobs/" + id, nil, PermJobView},
		{http.MethodPost, "/jobs", jobBody("p2", "test.ok", "@daily", nil), PermJobCreate},
		{http.MethodPut, "/jobs/" + id, jobBody("权限", "test.ok", "@daily", map[string]any{"version": 0}), PermJobUpdate},
		{http.MethodPut, "/jobs/" + id + "/status", map[string]any{"enabled": true, "version": 1}, PermJobUpdate},
		{http.MethodPost, "/jobs/" + id + "/run", nil, PermJobExecute},
		{http.MethodGet, "/job-runs", nil, PermRunView},
		{http.MethodGet, "/monitor/app", nil, PermMonitor},
		{http.MethodGet, "/monitor/server", nil, PermMonitor},
		{http.MethodDelete, "/jobs/" + id, nil, PermJobDelete},
	}
	for _, tc := range cases {
		r := a.do(tc.method, tc.path, tc.body, "none")
		assert.Equal(t, http.StatusForbidden, r.Status, "%s %s without permission", tc.method, tc.path)
		r = a.do(tc.method, tc.path, tc.body, PermRunView+","+PermJobView)
		if tc.need != PermJobView && tc.need != PermRunView {
			assert.Equal(t, http.StatusForbidden, r.Status, "%s %s with view only", tc.method, tc.path)
		}
		r = a.do(tc.method, tc.path, tc.body, tc.need)
		assert.Equal(t, http.StatusOK, r.Status, "%s %s with %s: %s", tc.method, tc.path, tc.need, r.Data)
	}
}
