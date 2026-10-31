package audit

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/platform/database"
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
	// width_bucket returns 1..trendDays for rows inside [bounds[0], bounds[n]).
	var rows []bucketRow
	err := m.db.WithContext(ctx).Raw(`SELECT width_bucket(created_at, ?::timestamptz[]) - 1 AS day, success, count(*) AS n
		FROM `+table+` WHERE `+cond+` AND created_at >= ? AND created_at < ?
		GROUP BY 1, 2`, timestamps(bounds), bounds[0], bounds[trendDays]).Scan(&rows).Error
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

// timestamps renders bounds as a timestamptz[] parameter; GORM would expand
// a plain slice into a value list.
func timestamps(bounds []time.Time) database.TextArray {
	out := make(database.TextArray, len(bounds))
	for i, t := range bounds {
		out[i] = t.Format(time.RFC3339Nano)
	}
	return out
}
