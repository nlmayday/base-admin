// Package lookup serves relation fields of generated modules: the options
// of a select that refers to another table by id, and the check that a
// chosen id exists. Table and column names come from generated code; they
// are still checked so a mistake cannot reach SQL.
package lookup

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/ids"
)

// MaxOptions bounds one options request; users narrow the list by typing.
const MaxOptions = 50

var identPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// Target is a referenced table: Key is its bigint key column ("" for id),
// Label the column shown to users, SoftDelete hides rows whose deleted_at
// is set.
type Target struct {
	Table      string
	Label      string
	SoftDelete bool
	Key        string
}

func (t Target) key() string {
	if t.Key == "" {
		return "id"
	}
	return t.Key
}

// Option is one selectable row.
type Option struct {
	ID    ids.ID `json:"id"`
	Label string `json:"label"`
}

func (t Target) query(ctx context.Context, db *gorm.DB) (*gorm.DB, error) {
	if !identPattern.MatchString(t.Table) || !identPattern.MatchString(t.Label) || !identPattern.MatchString(t.key()) {
		return nil, fmt.Errorf("lookup: invalid target %s.%s", t.Table, t.Label)
	}
	q := db.WithContext(ctx).Table(`"` + t.Table + `" AS x`)
	if t.SoftDelete {
		q = q.Where("x.deleted_at IS NULL")
	}
	return q, nil
}

// Options lists rows whose label contains keyword (all rows when empty),
// ordered by label, at most MaxOptions.
func (t Target) Options(ctx context.Context, db *gorm.DB, keyword string) ([]Option, error) {
	q, err := t.query(ctx, db)
	if err != nil {
		return nil, err
	}
	label := `COALESCE(` + database.CastText(db, `x."`+t.Label+`"`) + `, '')`
	if kw := strings.TrimSpace(keyword); kw != "" {
		q = q.Where(`LOWER(`+label+`) LIKE LOWER(?)`, "%"+escapeLike(kw)+"%")
	}
	out := []Option{}
	err = q.Select("x." + t.key() + " AS id, " + label + " AS label").Order(label + ", x." + t.key()).Limit(MaxOptions).Scan(&out).Error
	return out, err
}

// Exists reports whether id is a visible row of the target.
func (t Target) Exists(ctx context.Context, db *gorm.DB, id int64) (bool, error) {
	q, err := t.query(ctx, db)
	if err != nil {
		return false, err
	}
	var n int64
	err = q.Where("x."+t.key()+" = ?", id).Limit(1).Count(&n).Error
	return n > 0, err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
