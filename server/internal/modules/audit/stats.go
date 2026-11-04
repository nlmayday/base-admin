package audit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// trendDays is the length of the trend series, today included.
const trendDays = 7

type dayStats struct {
	LoginSuccess      int64 `json:"loginSuccess"`
	LoginFailure      int64 `json:"loginFailure"`
	Operations        int64 `json:"operations"`
	OperationFailures int64 `json:"operationFailures"`
}

type trendPoint struct {
	// Date is the server-local day, formatted 2006-01-02.
	Date string `json:"date"`
	dayStats
}

type statsView struct {
	Today dayStats     `json:"today"`
	Trend []trendPoint `json:"trend"`
}

type bucketRow struct {
	Day     int
	Success bool
	N       int64
}

// stats returns today's counts and a 7-day trend, bucketed by server-local
// days. Logout events are not counted as logins.
func (m *Module) stats(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	now := m.now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := today.AddDate(0, 0, -(trendDays - 1))

	trend := make([]trendPoint, trendDays)
	for i := range trend {
		trend[i].Date = start.AddDate(0, 0, i).Format(time.DateOnly)
	}
	logins, err := m.buckets(ctx, tableLoginLogs, "kind = 'login'", start)
	if err != nil {
		return nil, err
	}
	for _, b := range logins {
		if b.Success {
			trend[b.Day].LoginSuccess += b.N
		} else {
			trend[b.Day].LoginFailure += b.N
		}
	}
	ops, err := m.buckets(ctx, tableOperationLogs, "TRUE", start)
	if err != nil {
		return nil, err
	}
	for _, b := range ops {
		trend[b.Day].Operations += b.N
		if !b.Success {
			trend[b.Day].OperationFailures += b.N
		}
	}
	return statsView{Today: trend[trendDays-1].dayStats, Trend: trend}, nil
}

// buckets counts rows per day offset from start and success flag. table
// and cond are constants. Day offsets come from local calendar days in Go,
// so the bucket boundaries follow the server time zone.
func (m *Module) buckets(ctx context.Context, table, cond string, start time.Time) ([]bucketRow, error) {
	bounds := make([]time.Time, trendDays+1)
	for i := range bounds {
		bounds[i] = start.AddDate(0, 0, i)
	}
	// The CASE maps rows inside [bounds[0], bounds[n]) to their day 0..n-1.
	var b strings.Builder
	args := make([]any, 0, trendDays+2)
	b.WriteString("CASE")
	for i := range trendDays {
		fmt.Fprintf(&b, " WHEN created_at < ? THEN %d", i)
		args = append(args, bounds[i+1])
	}
	b.WriteString(" END")
	args = append(args, bounds[0], bounds[trendDays])
	var rows []bucketRow
	err := m.db.WithContext(ctx).Raw(`SELECT `+b.String()+` AS day, success, count(*) AS n
		FROM `+table+` WHERE `+cond+` AND created_at >= ? AND created_at < ?
		GROUP BY 1, 2`, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := rows[:0:0]
	for _, r := range rows {
		if r.Day >= 0 && r.Day < trendDays {
			out = append(out, r)
		}
	}
	return out, nil
}
