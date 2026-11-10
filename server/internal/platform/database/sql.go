package database

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Shared SQL conventions. Hand-written SQL runs unchanged on both dialects
// when it sticks to what they share; MySQL sessions run with ANSI_QUOTES and
// PIPES_AS_CONCAT (see MySQLSQLMode), so:
//
//   - identifiers that are reserved words in either database are written in
//     double quotes ("key", "trigger"); string literals use single quotes;
//   - || concatenates strings;
//   - CURRENT_TIMESTAMP(6) is the current time (UTC on MySQL);
//   - case-insensitive matching is LOWER(col) LIKE LOWER(?), not ILIKE.
//
// The helpers below cover what has no shared spelling.

// CountAtMost counts the rows of q, stopping at limit: SELECT COUNT(*) FROM
// (SELECT 1 … LIMIT limit) reads at most limit rows, where COUNT(*) on a
// table of millions reads them all. A result of limit means "limit or
// more". q keeps its conditions and joins; its select list and order are
// replaced.
func CountAtMost(q *gorm.DB, limit int) (int64, error) {
	if limit <= 0 {
		return 0, errors.New("database: CountAtMost needs a positive limit")
	}
	sub := q.Session(&gorm.Session{}).Select("1").Order(nil).Limit(limit)
	var n int64
	err := q.Session(&gorm.Session{NewDB: true}).Table("(?) AS counted", sub).Count(&n).Error
	return n, err
}

// PageFirst pages q by its key before anything is joined to it:
//
//	SELECT … FROM from JOIN (SELECT key AS page_key FROM … WHERE …
//	  ORDER BY order LIMIT limit OFFSET offset) AS page ON page.page_key = key
//	ORDER BY order
//
// from is q's table with its alias ("game_rounds AS t"), key its primary
// key ("t.id") and order names columns of from only. Join the tables names
// are shown from onto the result and select the columns.
//
// Joined in the same query as the ORDER BY … LIMIT, small lookup tables
// (players, games) lead MySQL to a hash join that reads and sorts the whole
// large table instead of walking its time index: seconds on a table of ten
// million rows. Paged first, the index is walked for one page and only
// that page is joined. Paging the key rather than whole rows lets a filter
// on an indexed column be checked in the index (with InnoDB every
// secondary index carries the primary key), so a filter that matches few
// rows reads index entries, not a full row for every entry it passes.
func PageFirst(q *gorm.DB, from, key, order string, offset, limit int) *gorm.DB {
	page := q.Session(&gorm.Session{}).Select(key + " AS page_key").Order(order).Offset(offset).Limit(limit)
	return q.Session(&gorm.Session{NewDB: true}).Table(from).
		Joins("JOIN (?) AS page ON page.page_key = "+key, page).Order(order)
}

// InsertID runs an INSERT and returns the id the database generated for
// the new row: RETURNING id on PostgreSQL, the insert id of the statement
// on MySQL. query is a plain INSERT without a RETURNING clause.
func InsertID(tx *gorm.DB, query string, args ...any) (int64, error) {
	if !IsMySQL(tx) {
		var id int64
		if err := tx.Raw(query+" RETURNING id", args...).Scan(&id).Error; err != nil {
			return 0, err
		}
		return id, nil
	}
	// Let GORM expand the arguments without running the statement, then run
	// it on the same connection (pool or transaction) to read its insert id;
	// a separate SELECT LAST_INSERT_ID() could land on another connection.
	// The statement bypasses GORM's logger, so it is not in slow-query logs.
	stmt := tx.Session(&gorm.Session{DryRun: true, NewDB: true}).Exec(query, args...).Statement
	if stmt.Error != nil {
		return 0, stmt.Error
	}
	res, err := tx.Statement.ConnPool.ExecContext(stmt.Context, stmt.SQL.String(), stmt.Vars...)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read insert id: %w", err)
	}
	if id == 0 {
		return 0, errors.New("insert generated no id")
	}
	return id, nil
}

// OnConflictUpdate is the clause that follows the VALUES list of an upsert
// keyed on the unique columns cols, up to and including the keyword before
// the assignments. The proposed row is called excluded in both dialects:
//
//	INSERT INTO t (code, name) VALUES (?, ?) <OnConflictUpdate("code")> name = excluded.name
//
// MySQL resolves a conflict on any unique key of the table, so the table
// must have no unique key other than cols that the insert could hit. Row
// counts differ: MySQL counts an updated row as 2 and an unchanged one as 1.
func OnConflictUpdate(db *gorm.DB, cols ...string) string {
	if IsMySQL(db) {
		return "AS excluded ON DUPLICATE KEY UPDATE"
	}
	return "ON CONFLICT (" + strings.Join(cols, ", ") + ") DO UPDATE SET"
}

// OnConflictDoNothing ends an INSERT that skips rows conflicting with a
// unique key. MySQL has no DO NOTHING: assigning the table's column col to
// itself changes nothing, and unlike INSERT IGNORE it keeps every other
// error, such as a foreign key violation. A skipped row counts as affected
// on MySQL (0 on PostgreSQL): to count inserted rows, leave existing ones
// out with NOT EXISTS instead of relying on RowsAffected.
func OnConflictDoNothing(db *gorm.DB, table, col string) string {
	if IsMySQL(db) {
		return "ON DUPLICATE KEY UPDATE " + table + "." + col + " = " + table + "." + col
	}
	return "ON CONFLICT DO NOTHING"
}

// CastText converts expr to text, for example to read a JSON column or to
// compare a number with a string.
func CastText(db *gorm.DB, expr string) string {
	if IsMySQL(db) {
		return "CAST(" + expr + " AS CHAR)"
	}
	return "CAST(" + expr + " AS text)"
}

// FormatDate renders a date or timestamp expression as YYYY-MM-DD.
func FormatDate(db *gorm.DB, expr string) string {
	if IsMySQL(db) {
		return "DATE_FORMAT(" + expr + ", '%Y-%m-%d')"
	}
	return "to_char(" + expr + ", 'YYYY-MM-DD')"
}
