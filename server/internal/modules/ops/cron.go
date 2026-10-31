package ops

import (
	"errors"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	maxCronLength = 128
	minEvery      = time.Second
	previewCount  = 5
)

// cronParser accepts the standard 5-field syntax (minute hour dom month dow)
// plus descriptors such as @daily, @hourly and @every 5m.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// alignedEvery replaces robfig's @every schedule, which counts from the
// moment it was added. Fires are aligned to multiples of the interval since
// the zero time instead, so every instance computes the same fire times and
// the per-fire Redis key deduplicates them.
type alignedEvery struct{ every time.Duration }

func (a alignedEvery) Next(t time.Time) time.Time { return t.Truncate(a.every).Add(a.every) }

// parseCron validates expr and returns its schedule. Messages are shown to
// users.
func parseCron(expr string) (cron.Schedule, error) {
	expr = strings.TrimSpace(expr)
	switch {
	case expr == "":
		return nil, errors.New("不能为空")
	case len(expr) > maxCronLength:
		return nil, errors.New("不能多于 128 个字符")
	case strings.HasPrefix(expr, "TZ=") || strings.HasPrefix(expr, "CRON_TZ="):
		return nil, errors.New("不支持指定时区，按服务器时区执行")
	}
	if rest, ok := strings.CutPrefix(expr, "@every "); ok {
		// robfig silently rounds sub-second intervals; reject them instead.
		d, err := time.ParseDuration(strings.TrimSpace(rest))
		if err != nil {
			return nil, errors.New("表达式无效：@every 后应为时长，如 30s、5m、1h")
		}
		if d < minEvery || d%time.Second != 0 {
			return nil, errors.New("@every 间隔必须是整秒且不能小于 1 秒")
		}
		return alignedEvery{every: d}, nil
	}
	sched, err := cronParser.Parse(expr)
	if err != nil {
		return nil, errors.New("表达式无效：" + err.Error())
	}
	// robfig gives up after five years and returns the zero time, e.g. for
	// "0 0 30 2 *" (February 30th).
	if sched.Next(time.Now()).IsZero() {
		return nil, errors.New("表达式永远不会触发")
	}
	return sched, nil
}

// nextTimes returns the next n fire times after from.
func nextTimes(sched cron.Schedule, from time.Time, n int) []time.Time {
	out := make([]time.Time, 0, n)
	t := from
	for range n {
		t = sched.Next(t)
		if t.IsZero() {
			break
		}
		out = append(out, t)
	}
	return out
}

// nextRun returns the next fire time of expr, or nil when it is invalid.
func nextRun(expr string, from time.Time) *time.Time {
	sched, err := parseCron(expr)
	if err != nil {
		return nil
	}
	t := sched.Next(from)
	if t.IsZero() {
		return nil
	}
	return &t
}
