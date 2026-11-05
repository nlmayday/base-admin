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
	if f.ref == nil || (f.Kind != kindID && f.Kind != kindString) || f.Role != "" || !f.canWrite {
		return []string{}
	}
	return slices.Concat(f.ref.Labels, f.ref.NumberLabels)
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
		if len(relationLabels(f)) > 0 && len(f.ref.Labels) > 0 {
			f.Relation = f.ref.Labels[0] // integer columns only when chosen
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
		f.RelNumber = slices.Contains(f.ref.NumberLabels, f.Relation)
		// 客户名称 reads well; 玩家 ID名称 (short_id) or 玩法家族名称 (the
		// family code itself) do not: name the shown column instead.
		f.LabelDoc = f.Label + "名称"
		if f.RelNumber || f.Relation == f.ref.key() {
			f.LabelDoc = f.Label + "（" + f.ref.Table + "." + f.Relation + "）"
		}
		target := []string{fmt.Sprintf("Table: %q", f.ref.Table)}
		if k := f.ref.key(); k != "id" {
			target = append(target, fmt.Sprintf("Key: %q", k))
		}
		target = append(target, fmt.Sprintf("Label: %q", f.Relation))
		if f.RelNumber {
			target = append(target, "NumberLabel: true")
		}
		target = append(target, fmt.Sprintf("SoftDelete: %v", f.ref.SoftDelete))
		f.RelTarget = "lookup.Target{" + strings.Join(target, ", ") + "}"
		f.RelValue, f.RelValuePtr = "req."+f.GoName, "*req."+f.GoName
		if f.Kind == kindID {
			f.RelValue, f.RelValuePtr = "int64(req."+f.GoName+")", "int64(*req."+f.GoName+")"
		}
		if f.Kind == kindString && (f.Search == SearchLike || f.Search == SearchRange) {
			f.Search = SearchEq // picked from the select
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
		if f.mustWrite && f.Writable {
			d.RequiredRelations = append(d.RequiredRelations, f)
			d.skipped = append(d.skipped, fmt.Sprintf("关联字段 %s 必填：生成的集成测试从 %s 取一条已有记录，表为空时跳过", f.Column, f.ref.Table))
		}
	}
}

// relationSearch turns the ID (or code) search of relation fields into
// the select.
func (d *tmplData) relationSearch() {
	for _, f := range d.Relations {
		switch {
		case f.SearchWidget == "id":
		case f.Kind == kindString && f.SearchWidget == "text":
			// The select clears to null.
			f.FilterTS, f.FilterInit = []string{f.JSONName + "?: string | null;"}, []string{f.JSONName + ": null"}
		default:
			continue
		}
		f.SearchWidget = "relation"
		d.RelationSearch = append(d.RelationSearch, f)
	}
}
