//go:build integration

package database

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/testinfra"
)

// dialectDB opens a GORM handle on a fresh database holding the tables the
// helpers are tested against, in SQL both dialects accept.
func dialectDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()
	cfg := testinfra.Database(t)
	sqlDB, err := OpenSQL(ctx, cfg, "dialect-test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := NewGorm(sqlDB, cfg, zap.NewNop())
	require.NoError(t, err)

	id := "id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY"
	list := "text[] NOT NULL DEFAULT '{}'"
	if IsMySQL(db) {
		id, list = "id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY", "json NOT NULL"
	}
	for _, stmt := range []string{
		`CREATE TABLE t_parent (` + id + `, code varchar(32) NOT NULL, name varchar(32) NOT NULL DEFAULT '',
			tags ` + list + `, CONSTRAINT t_parent_code_uq UNIQUE (code))`,
		`CREATE TABLE t_child (` + id + `, parent_id bigint NOT NULL,
			CONSTRAINT t_child_parent_fk FOREIGN KEY (parent_id) REFERENCES t_parent (id))`,
		`CREATE TABLE sys_tx_locks (name varchar(128) PRIMARY KEY)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	return db
}

func TestDialectMatchesURL(t *testing.T) {
	db := dialectDB(t)
	want := Postgres
	if testinfra.IsMySQL(t) {
		want = MySQL
	}
	assert.Equal(t, want, DialectOf(db))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	assert.Equal(t, want, SQLDialect(sqlDB))
}

func TestInsertIDInAndOutsideTransactions(t *testing.T) {
	db := dialectDB(t)
	first, err := InsertID(db, `INSERT INTO t_parent (code, tags) VALUES (?, ?)`, "a", TextArray{"x"})
	require.NoError(t, err)
	var second int64
	require.NoError(t, RunTx(context.Background(), db, func(tx *gorm.DB) error {
		second, err = InsertID(tx, `INSERT INTO t_parent (code, tags) VALUES (?, ?)`, "b", TextArray{})
		return err
	}))
	assert.Greater(t, second, first)

	var codes []string
	require.NoError(t, db.Raw(`SELECT code FROM t_parent WHERE id IN ? ORDER BY id`, []int64{first, second}).Scan(&codes).Error)
	assert.Equal(t, []string{"a", "b"}, codes)

	_, err = InsertID(db, `INSERT INTO t_parent (code, tags) VALUES (?, ?)`, "a", TextArray{})
	assert.True(t, IsUniqueViolation(err), "%v", err)
}

func TestTextArrayStoredRoundTrip(t *testing.T) {
	db := dialectDB(t)
	in := TextArray{"plain", `with "quotes"`, `back\slash`, "逗号,分隔", ""}
	id, err := InsertID(db, `INSERT INTO t_parent (code, tags) VALUES (?, ?)`, "c", in)
	require.NoError(t, err)
	var row struct{ Tags TextArray }
	require.NoError(t, db.Raw(`SELECT tags FROM t_parent WHERE id = ?`, id).Scan(&row).Error)
	assert.Equal(t, in, row.Tags)
}

func TestErrorClassification(t *testing.T) {
	db := dialectDB(t)
	require.NoError(t, db.Exec(`INSERT INTO t_parent (code, tags) VALUES ('dup', ?)`, TextArray{}).Error)

	err := db.Exec(`INSERT INTO t_parent (code, tags) VALUES ('dup', ?)`, TextArray{}).Error
	name, ok := UniqueConstraint(err)
	assert.True(t, ok)
	assert.Equal(t, "t_parent_code_uq", name)
	assert.False(t, IsForeignKeyViolation(err))

	err = db.Exec(`INSERT INTO t_child (parent_id) VALUES (999)`).Error
	assert.True(t, IsForeignKeyViolation(err), "%v", err)
	assert.False(t, IsUniqueViolation(err))
	assert.False(t, IsValueTooLong(err))

	err = db.Exec(`INSERT INTO t_parent (code, tags) VALUES (?, ?)`, strings.Repeat("长", 33), TextArray{}).Error
	assert.True(t, IsValueTooLong(err), "%v", err)
	assert.False(t, IsUniqueViolation(err))
}

func TestUpsertClauses(t *testing.T) {
	db := dialectDB(t)
	upsert := func(name string) {
		require.NoError(t, db.Exec(`INSERT INTO t_parent (code, name, tags) VALUES ('u', ?, ?) `+
			OnConflictUpdate(db, "code")+` name = excluded.name`, name, TextArray{}).Error)
	}
	upsert("first")
	upsert("second")
	var names []string
	require.NoError(t, db.Raw(`SELECT name FROM t_parent WHERE code = 'u'`).Scan(&names).Error)
	assert.Equal(t, []string{"second"}, names)

	res := db.Exec(`INSERT INTO t_parent (code, name, tags) VALUES ('u', 'third', ?) `+
		OnConflictDoNothing(db, "t_parent", "code"), TextArray{})
	require.NoError(t, res.Error)
	require.NoError(t, db.Raw(`SELECT name FROM t_parent WHERE code = 'u'`).Scan(&names).Error)
	assert.Equal(t, []string{"second"}, names, "do nothing keeps the row")

	err := db.Exec(`INSERT INTO t_child (parent_id) VALUES (999) ` + OnConflictDoNothing(db, "t_child", "id")).Error
	assert.True(t, IsForeignKeyViolation(err), "other errors are kept: %v", err)
}

func TestCastTextAndFormatDate(t *testing.T) {
	db := dialectDB(t)
	var got struct{ N, D string }
	require.NoError(t, db.Raw(`SELECT `+CastText(db, "42")+` AS n, `+FormatDate(db, "CAST(? AS date)")+` AS d`,
		"2026-03-07").Scan(&got).Error)
	assert.Equal(t, "42", got.N)
	assert.Equal(t, "2026-03-07", got.D)
}

func TestSharedSQLConventions(t *testing.T) {
	db := dialectDB(t)
	var got struct {
		Joined string
		Key    string
		Lower  bool
	}
	require.NoError(t, db.Raw(`SELECT 'a' || 'b' AS joined, 'k' AS "key",
		LOWER('ABC') LIKE LOWER(?) AS lower`, "%b%").Scan(&got).Error)
	assert.Equal(t, "ab", got.Joined)
	assert.Equal(t, "k", got.Key)
	assert.True(t, got.Lower)
	var now time.Time
	require.NoError(t, db.Raw(`SELECT CURRENT_TIMESTAMP(6)`).Scan(&now).Error)
	assert.WithinDuration(t, time.Now(), now, time.Minute, "the database clock is read as UTC")
	assert.Equal(t, time.UTC, now.Location(), "times come back in UTC on both databases, so the API sends UTC")
}

func TestLockTxSerializesTransactions(t *testing.T) {
	db := dialectDB(t)
	ctx := context.Background()
	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = RunTx(ctx, db, func(tx *gorm.DB) error {
			if err := LockTx(tx, "k"); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	var entered atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- RunTx(ctx, db, func(tx *gorm.DB) error {
			if err := LockTx(tx, "k"); err != nil {
				return err
			}
			entered.Store(true)
			return nil
		})
	}()
	time.Sleep(200 * time.Millisecond)
	assert.False(t, entered.Load(), "the second transaction must wait for the lock")
	close(release)
	require.NoError(t, <-done)
	assert.True(t, entered.Load())
	require.NoError(t, RunTx(ctx, db, func(tx *gorm.DB) error { return LockTx(tx, "other") }),
		"other keys do not wait")
}

func TestLockTimeoutIsReportedAndRestored(t *testing.T) {
	db := dialectDB(t)
	ctx := context.Background()
	id, err := InsertID(db, `INSERT INTO t_parent (code, tags) VALUES ('l', ?)`, TextArray{})
	require.NoError(t, err)

	holder := db.WithContext(ctx).Begin()
	require.NoError(t, holder.Exec(`SELECT id FROM t_parent WHERE id = ? FOR UPDATE`, id).Error)
	defer holder.Rollback()

	start := time.Now()
	err = RunTx(ctx, db, func(tx *gorm.DB) error {
		if err := SetLockTimeout(tx, 500*time.Millisecond); err != nil {
			return err
		}
		return tx.Exec(`SELECT id FROM t_parent WHERE id = ? FOR UPDATE`, id).Error
	})
	assert.True(t, IsLockTimeout(err), "%v", err)
	assert.Less(t, time.Since(start), 5*time.Second)

	if IsMySQL(db) {
		// Every pooled connection is back at the server default.
		sqlDB, err := db.DB()
		require.NoError(t, err)
		for range sqlDB.Stats().OpenConnections {
			var secs int
			require.NoError(t, db.Raw(`SELECT @@session.innodb_lock_wait_timeout`).Scan(&secs).Error)
			assert.Greater(t, secs, 1)
		}
	}
	if IsMySQL(db) {
		// Outside RunTx the setting could not be restored, so it is refused.
		err := db.Transaction(func(tx *gorm.DB) error { return SetLockTimeout(tx, time.Second) })
		assert.ErrorContains(t, err, "inside database.RunTx")
	}
}

func TestLockTimeoutRestoredAfterPanicAndNesting(t *testing.T) {
	db := dialectDB(t)
	if !IsMySQL(db) {
		t.Skip("PostgreSQL scopes lock_timeout to the transaction itself")
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1) // every statement below shares one connection
	ctx := context.Background()
	wait := func() int {
		var secs int
		require.NoError(t, db.Raw(`SELECT @@session.innodb_lock_wait_timeout`).Scan(&secs).Error)
		return secs
	}
	def := wait()

	assert.Panics(t, func() {
		_ = RunTx(ctx, db, func(tx *gorm.DB) error {
			require.NoError(t, SetLockTimeout(tx, time.Second))
			panic("boom")
		})
	})
	assert.Equal(t, def, wait(), "a panic must not leave the short wait on the connection")

	require.NoError(t, RunTx(ctx, db, func(tx *gorm.DB) error {
		if err := SetLockTimeout(tx, 2*time.Second); err != nil {
			return err
		}
		if err := RunTx(ctx, tx, func(*gorm.DB) error { return nil }); err != nil {
			return err
		}
		var secs int
		require.NoError(t, tx.Raw(`SELECT @@session.innodb_lock_wait_timeout`).Scan(&secs).Error)
		assert.Equal(t, 2, secs, "a nested RunTx keeps the outer wait")
		return nil
	}))
	assert.Equal(t, def, wait())
}

// CountAtMost stops at the limit, keeps the conditions and joins of the
// query, and ignores its order and select list.
func TestCountAtMost(t *testing.T) {
	db := dialectDB(t)
	for i := range 30 {
		_, err := InsertID(db, `INSERT INTO t_parent (code, tags) VALUES (?, ?)`, fmt.Sprintf("c%02d", i), TextArray{})
		require.NoError(t, err)
	}
	// A query a list handler would build: a session, reused after counting.
	q := db.Table("t_parent").Select("id, code").Order("id DESC").Session(&gorm.Session{})

	n, err := CountAtMost(q, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 10, n, "stops at the limit")

	n, err = CountAtMost(q, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 30, n, "fewer rows than the limit: the exact count")

	n, err = CountAtMost(q.Where("code LIKE ?", "c1%"), 100)
	require.NoError(t, err)
	assert.EqualValues(t, 10, n, "conditions are kept")

	joined := db.Table("t_parent AS p").Joins("JOIN t_parent AS q2 ON q2.id = p.id").Where("p.code < ?", "c05")
	n, err = CountAtMost(joined, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 5, n, "joins are kept")

	var rows []struct{ Code string }
	require.NoError(t, q.Limit(3).Scan(&rows).Error)
	assert.Equal(t, []struct{ Code string }{{"c29"}, {"c28"}, {"c27"}}, rows, "the query keeps its order and select after counting")

	_, err = CountAtMost(q, 0)
	assert.Error(t, err)
}

func TestPageFirst(t *testing.T) {
	db := dialectDB(t)
	parents := make([]int64, 3)
	for i := range parents {
		id, err := InsertID(db, `INSERT INTO t_parent (code, name, tags) VALUES (?, ?, ?)`,
			fmt.Sprintf("p%d", i), fmt.Sprintf("name%d", i), TextArray{})
		require.NoError(t, err)
		parents[i] = id
	}
	children := make([]int64, 10)
	for i := range children {
		id, err := InsertID(db, `INSERT INTO t_child (parent_id) VALUES (?)`, parents[i%3])
		require.NoError(t, err)
		children[i] = id
	}
	type row struct {
		ID   int64
		Name string
	}
	// The list's query: conditions on the large table only, counted alone,
	// then paged and joined.
	q := db.Table("t_child AS c").Where("c.parent_id <> ?", parents[2]).Session(&gorm.Session{})
	var rows []row
	err := PageFirst(q, "t_child AS c", "c.id", "c.id DESC", 2, 3).
		Joins("LEFT JOIN t_parent p ON p.id = c.parent_id").
		Select("c.id, p.name").Scan(&rows).Error
	require.NoError(t, err)
	// Children not of parents[2], newest first: 9 7 6 4 3 1 0 (index); skip 2.
	assert.Equal(t, []row{
		{children[6], "name0"}, {children[4], "name1"}, {children[3], "name0"},
	}, rows, "conditions, order, offset and limit apply before the join")

	n, err := CountAtMost(q, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 7, n, "the query is reusable after paging")
}
