package codegen

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

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
		if cs.KeyTable != "" {
			if err := checkKeyTable(ctx, db, cs); err != nil {
				return t, err
			}
		}
		t.Children = append(t.Children, ChildTable{Spec: cs, Table: child})
	}
	return t, nil
}

// declareRef makes column o.Column a relation to o.RefTable, as a foreign
// key would: the key column (o.RefKey, default id) and the column are both
// bigints or both text (a code such as a game family).
func declareRef(ctx context.Context, db *gorm.DB, t Table, o FieldOption) (Table, error) {
	i := slices.IndexFunc(t.Columns, func(c Column) bool { return c.Name == o.Column })
	switch {
	case i < 0:
		return t, specError("字段 %s 不存在", o.Column)
	case t.Columns[i].UDT != "int8" && !textUDT(t.Columns[i].UDT):
		return t, specError("字段 %s 声明关联时须为 bigint 或文本", o.Column)
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
	switch {
	case !ok:
		return t, specError("关联表 %s 没有列 %s", o.RefTable, key)
	case t.Columns[i].UDT == "int8" && kc.UDT != "int8":
		return t, specError("关联表 %s 的 %s 不是 bigint，与字段 %s 的类型不一致", o.RefTable, key, o.Column)
	case textUDT(t.Columns[i].UDT) && !textUDT(kc.UDT):
		return t, specError("关联表 %s 的 %s 不是文本，与字段 %s 的类型不一致", o.RefTable, key, o.Column)
	}
	cols := make([]refColumn, len(ref.Columns))
	for j, c := range ref.Columns {
		cols[j] = refColumn{Name: c.Name, UDT: c.UDT}
	}
	r := refOf(o.RefTable, cols)
	if key != "id" {
		r.Key = key
	}
	notKey := func(l string) bool { return l == key }
	r.Labels = slices.DeleteFunc(r.Labels, notKey)
	r.NumberLabels = slices.DeleteFunc(r.NumberLabels, notKey)
	switch {
	case len(r.Labels) == 0 && len(r.NumberLabels) == 0:
		return t, specError("关联表 %s 没有可显示的列", o.RefTable)
	case o.Relation != nil && *o.Relation != "" && !slices.Contains(r.Labels, *o.Relation) && !slices.Contains(r.NumberLabels, *o.Relation):
		return t, specError("关联表 %s 没有可显示的列 %s（文本或整数列）", o.RefTable, *o.Relation)
	case len(r.Labels) == 0 && (o.Relation == nil || *o.Relation == ""):
		return t, specError("关联表 %s 没有文本列：用 relation 指定显示的整数列（%s）", o.RefTable, strings.Join(r.NumberLabels, "、"))
	}
	cols2 := slices.Clone(t.Columns)
	cols2[i].Ref = r
	t.Columns = cols2
	return t, nil
}

func textUDT(udt string) bool { return udt == "varchar" || udt == "text" || udt == "bpchar" }

// checkKeyTable verifies that the key table of a child has the named
// columns and that the filter is a valid condition there.
func checkKeyTable(ctx context.Context, db *gorm.DB, cs ChildSpec) error {
	if msg := keyTableProblem(cs); msg != "" {
		return specError("子表 %s：%s", cs.Table, msg)
	}
	kt, err := Introspect(ctx, db, cs.KeyTable)
	if errors.Is(err, ErrTableNotFound) {
		return specError("子表 %s 的键表 %s 不存在", cs.Table, cs.KeyTable)
	}
	if err != nil {
		return err
	}
	for _, c := range []string{cs.KeyValueColumn, cs.KeyLabelColumn, cs.KeySortColumn, cs.KeyRequiredColumn} {
		if _, ok := kt.Column(c); c != "" && !ok {
			return specError("键表 %s 没有列 %s", cs.KeyTable, c)
		}
	}
	q := db.WithContext(ctx).Table(cs.KeyTable).Select(cs.KeyValueColumn).Limit(1)
	if cs.KeyFilter != "" {
		q = q.Where(cs.KeyFilter)
	}
	rows, err := q.Rows()
	if err != nil {
		return specError("键表 %s 的 keyFilter 无法执行：%s", cs.KeyTable, err.Error())
	}
	return rows.Close()
}
