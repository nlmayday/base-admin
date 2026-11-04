package codegen

import (
	"fmt"
	"slices"
	"strings"
)

// Relation fields: a column that is a foreign key to another table's id
// (customer_id → biz_customers) becomes a searchable select of that table's
// rows; lists show the chosen row's label column. Lookups go through
// internal/core/lookup.

// relationLabels lists the label columns f can show, or nothing when f
// is not a relation candidate.
func relationLabels(f *Field) []string {
	if f.ref == nil || f.Kind != kindID || f.Role != "" || !f.canWrite {
		return []string{}
	}
	return slices.Clone(f.ref.Labels)
}

// relationTable is the table f refers to, or "".
func relationTable(f *Field) string {
	if len(relationLabels(f)) == 0 {
		return ""
	}
	return f.ref.Table
}

// defaultRelations shows each foreign key by the referenced table's best
// label column.
func (d *tmplData) defaultRelations() {
	for _, f := range d.Fields {
		if labels := relationLabels(f); len(labels) > 0 {
			f.Relation = labels[0]
		}
	}
}

// deriveRelations fills the join, the label field and the form widget of
// every relation field.
func (d *tmplData) deriveRelations() {
	for _, f := range d.Fields {
		if f.Relation == "" {
			continue
		}
		label := strings.TrimSuffix(f.Column, "_id") + "_name"
		if d.hasColumn(label) || label == f.Column {
			label = f.Column + "_label"
		}
		f.RelAlias = fmt.Sprintf("r%d", len(d.Relations)+1)
		f.RelTable, f.RelLabelSQL = quoteIdent(f.ref.Table), quoteIdent(f.Relation)
		f.RelKey = quoteIdent(f.ref.key())
		f.LabelColumn, f.LabelGo, f.LabelJSON = label, goExported(label), lowerCamel(label)
		f.RelVar = d.Lower + f.GoName + "Lookup"
		f.RelTarget = fmt.Sprintf("lookup.Target{Table: %q, Label: %q, SoftDelete: %v}", f.ref.Table, f.Relation, f.ref.SoftDelete)
		if k := f.ref.key(); k != "id" {
			f.RelTarget = fmt.Sprintf("lookup.Target{Table: %q, Key: %q, Label: %q, SoftDelete: %v}", f.ref.Table, k, f.Relation, f.ref.SoftDelete)
		}
		f.RelOptionsTS = "list" + d.TSUpper + upperCamel(strings.ReplaceAll(f.Column, "_", "-")) + "Options"
		f.Widget, f.RuleTrig = "relation", "change"
		f.TSForm, f.TSDefault = "string | null", "null"
		if f.Nullable {
			f.TSSubmit = "form." + f.JSONName + " || null"
		} else {
			f.TSSubmit = "form." + f.JSONName + ` ?? ""`
		}
		d.Relations = append(d.Relations, f)
		if f.Writable {
			d.RelationFields = append(d.RelationFields, f)
		}
		if f.mustWrite {
			d.skipped = append(d.skipped, fmt.Sprintf("关联字段 %s 必填：生成的集成测试需把样例 ID 改成 %s 中已存在的记录", f.Column, f.ref.Table))
		}
	}
}

// relationSearch turns the ID search of relation fields into the select.
func (d *tmplData) relationSearch() {
	for _, f := range d.Relations {
		if f.SearchWidget == "id" {
			f.SearchWidget = "relation"
			d.RelationSearch = append(d.RelationSearch, f)
		}
	}
}
