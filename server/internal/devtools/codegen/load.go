package codegen

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"gorm.io/gorm"
)

// ErrSpec marks a spec that does not fit the database (a declared relation
// or child table that does not exist or does not match); the message is
// Chinese for the UI.
var ErrSpec = errors.New("spec")

func specError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrSpec, fmt.Sprintf(format, args...))
}

// SpecMessage is the Chinese message of an ErrSpec error.
func SpecMessage(err error) string {
	msg := err.Error()
	if p := ErrSpec.Error() + ": "; len(msg) > len(p) && msg[:len(p)] == p {
		return msg[len(p):]
	}
	return msg
}

// Load reads table and the tables spec refers to: the tables of declared
// relations (FieldOption.RefTable), which need no foreign key, and the
// child tables (spec.Children).
func Load(ctx context.Context, db *gorm.DB, table string, spec Spec) (Table, error) {
	t, err := Introspect(ctx, db, table)
	if err != nil {
		return t, err
	}
	for _, o := range spec.Fields {
		if o.RefTable == "" {
			continue
		}
		if t, err = declareRef(ctx, db, t, o); err != nil {
			return t, err
		}
	}
	for _, cs := range spec.Children {
		child, err := Introspect(ctx, db, cs.Table)
		if errors.Is(err, ErrTableNotFound) {
			return t, specError("子表 %s 不存在", cs.Table)
		}
		if err != nil {
			return t, err
		}
		t.Children = append(t.Children, ChildTable{Spec: cs, Table: child})
	}
	return t, nil
}

// declareRef makes column o.Column a relation to o.RefTable, as a foreign
// key would: the key column (o.RefKey, default id) must be a bigint, and so
// must the column.
func declareRef(ctx context.Context, db *gorm.DB, t Table, o FieldOption) (Table, error) {
	i := slices.IndexFunc(t.Columns, func(c Column) bool { return c.Name == o.Column })
	switch {
	case i < 0:
		return t, specError("字段 %s 不存在", o.Column)
	case t.Columns[i].UDT != "int8":
		return t, specError("字段 %s 声明关联时须为 bigint", o.Column)
	case !identifierPattern.MatchString(o.RefTable) || (o.RefKey != "" && !identifierPattern.MatchString(o.RefKey)):
		return t, specError("字段 %s 的关联表或关联列名无效", o.Column)
	}
	ref, err := Introspect(ctx, db, o.RefTable)
	if errors.Is(err, ErrTableNotFound) {
		return t, specError("字段 %s 的关联表 %s 不存在", o.Column, o.RefTable)
	}
	if err != nil {
		return t, err
	}
	key := o.RefKey
	if key == "" {
		key = "id"
	}
	kc, ok := ref.Column(key)
	if !ok || kc.UDT != "int8" {
		return t, specError("关联表 %s 没有 bigint 列 %s", o.RefTable, key)
	}
	cols := make([]refColumn, len(ref.Columns))
	for j, c := range ref.Columns {
		cols[j] = refColumn{Name: c.Name, UDT: c.UDT}
	}
	r := refOf(o.RefTable, cols)
	if key != "id" {
		r.Key = key
	}
	r.Labels = slices.DeleteFunc(r.Labels, func(l string) bool { return l == key })
	if len(r.Labels) == 0 {
		return t, specError("关联表 %s 没有可显示的文本列", o.RefTable)
	}
	if o.Relation != nil && *o.Relation != "" && !slices.Contains(r.Labels, *o.Relation) {
		return t, specError("关联表 %s 没有文本列 %s", o.RefTable, *o.Relation)
	}
	cols2 := slices.Clone(t.Columns)
	cols2[i].Ref = r
	t.Columns = cols2
	return t, nil
}
