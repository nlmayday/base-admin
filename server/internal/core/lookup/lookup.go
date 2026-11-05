// Package lookup serves relation fields of generated modules: the options
// of a select that refers to another table by key, and the check that a
// chosen key exists. Table and column names come from generated code; they
// are still checked so a mistake cannot reach SQL.
package lookup

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
)

// MaxOptions bounds one options request; users narrow the list by typing.
const MaxOptions = 50

var identPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// Target is a referenced table: Key is its key column ("" for id; a bigint
// or a text code), Label the column shown to users, SoftDelete hides rows
// whose deleted_at is set. NumberLabel: Label is an integer column (a
// player's short id), searched by exact value so its index is used.
type Target struct {
	Table       string
	Label       string
	SoftDelete  bool
	Key         string
	NumberLabel bool
	// Extra is a second column shown after the label ("100001（玩家甲）")
	// and searched with LIKE.
	Extra string
}

func (t Target) key() string {
	if t.Key == "" {
		return "id"
	}
	return t.Key
}

// Option is one selectable row; ID is the key as text (bigints as strings,
// like ids.ID).
type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Deleted: a soft-deleted row, listed only for Query.Deleted.
	Deleted bool `json:"deleted,omitempty"`
	// Extra is Target.Extra's value, already in Label.
	Extra string `json:"-"`
}

// Query narrows Find.
type Query struct {
	// Keyword: rows whose label contains it (a text label) or equals it (an
	// integer label); empty lists the first rows.
	Keyword string
	// Deleted also lists soft-deleted rows, marked 「（已删除）」: a search
	// form finds the records of closed accounts, a form never picks them.
	Deleted bool
}

func (t Target) query(ctx context.Context, db *gorm.DB, deleted bool) (*gorm.DB, error) {
	if !identPattern.MatchString(t.Table) || !identPattern.MatchString(t.Label) || !identPattern.MatchString(t.key()) ||
		(t.Extra != "" && !identPattern.MatchString(t.Extra)) {
		return nil, fmt.Errorf("lookup: invalid target %s.%s", t.Table, t.Label)
	}
	q := db.WithContext(ctx).Table(`"` + t.Table + `" AS x`)
	if t.SoftDelete && !deleted {
		q = q.Where("x.deleted_at IS NULL")
	}
	return q, nil
}

// Options lists the rows a form may pick whose label contains keyword
// (all rows when empty), ordered by label, at most MaxOptions.
func (t Target) Options(ctx context.Context, db *gorm.DB, keyword string) ([]Option, error) {
	return t.Find(ctx, db, Query{Keyword: keyword})
}

// Find lists the rows matching q, ordered by label, at most MaxOptions.
func (t Target) Find(ctx context.Context, db *gorm.DB, q Query) ([]Option, error) {
	tx, err := t.query(ctx, db, q.Deleted)
	if err != nil {
		return nil, err
	}
	col := `x."` + t.Label + `"`
	label := `COALESCE(` + database.CastText(db, col) + `, '')`
	extra := `COALESCE(` + database.CastText(db, `x."`+t.Extra+`"`) + `, '')`
	out := []Option{}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		n, numErr := strconv.ParseInt(kw, 10, 64)
		switch {
		case t.NumberLabel && numErr == nil:
			tx = tx.Where(col+" = ?", n) // the label's index
		case t.NumberLabel && t.Extra != "":
			tx = tx.Where(`LOWER(`+extra+`) LIKE LOWER(?)`, like)
		case t.NumberLabel:
			return out, nil // not a number: nothing matches
		case t.Extra != "":
			tx = tx.Where(`(LOWER(`+label+`) LIKE LOWER(?) OR LOWER(`+extra+`) LIKE LOWER(?))`, like, like)
		default:
			tx = tx.Where(`LOWER(`+label+`) LIKE LOWER(?)`, like)
		}
	}
	sel := database.CastText(db, "x."+t.key()) + " AS id, " + label + " AS label"
	if t.Extra != "" {
		sel += ", " + extra + " AS extra"
	}
	if t.SoftDelete && q.Deleted {
		sel += ", x.deleted_at IS NOT NULL AS deleted"
	}
	// The raw column orders by its index where there is one.
	err = tx.Select(sel).Order(col + ", x." + t.key()).Limit(MaxOptions).Scan(&out).Error
	for i := range out {
		if out[i].Extra != "" {
			out[i].Label += "（" + out[i].Extra + "）"
		}
		if out[i].Deleted {
			out[i].Label += "（已删除）"
		}
	}
	return out, err
}

// Exists reports whether key is a row a form may pick (not soft-deleted).
func (t Target) Exists(ctx context.Context, db *gorm.DB, key any) (bool, error) {
	q, err := t.query(ctx, db, false)
	if err != nil {
		return false, err
	}
	var n int64
	err = q.Where("x."+t.key()+" = ?", key).Limit(1).Count(&n).Error
	return n > 0, err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
