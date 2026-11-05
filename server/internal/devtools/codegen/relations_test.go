package codegen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelationDefaults(t *testing.T) {
	byColumn := map[string]FieldInfo{}
	for _, f := range mustDescribe(t, dealTable()) {
		byColumn[f.Column] = f
	}
	assert.Equal(t, "name", byColumn["customer_id"].Relation)
	assert.Equal(t, "biz_customers", byColumn["customer_id"].RelationTable)
	assert.Equal(t, []string{"nickname", "username"}, byColumn["owner_id"].RelationLabels)
	assert.Empty(t, byColumn["batch_id"].Relation, "no label column: a plain ID input")
	assert.Equal(t, []string{}, byColumn["title"].RelationLabels)
}

func TestRelationGenerated(t *testing.T) {
	spec := Spec{Module: "crm", Resource: "deal", Name: "商机", NewModule: true,
		Fields: []FieldOption{{Column: "customer_id", Search: SearchEq}, {Column: "owner_id", Relation: ptr("username")}}}
	res, err := Generate(dealTable(), spec)
	require.NoError(t, err)
	src := fileByPath(t, res, "/deal.go").Content
	for _, want := range []string{
		`var dealCustomerIDLookup = lookup.Target{Table: "biz_customers", Label: "name", SoftDelete: false}`,
		`var dealOwnerIDLookup = lookup.Target{Table: "sys_users", Label: "username", SoftDelete: true}`,
		`q = q.Joins(` + "`" + `LEFT JOIN biz_customers r1 ON r1.id = t.customer_id` + "`" + `)`,
		"COALESCE(` + database.CastText(db, \"r1.name\") + `, '') AS customer_name",
		`json:"customerName" doc:"客户名称"`,
		`g.GET("/options/customerId", access.Perm("crm:deal:view"), handle(m.dealCustomerIDOptions))`,
		`checkDealRelation(ctx, m.db, dealCustomerIDLookup, "customerId", "客户", int64(req.CustomerID))`,
		"if req.OwnerID != nil {",
		"csvValue(r.CustomerName)",
	} {
		assert.Contains(t, src, want)
	}
	assert.NotContains(t, src, "dealBatchIDLookup")

	api := fileByPath(t, res, "/deal.ts").Content
	assert.Contains(t, api, "customerName: string;")
	assert.Contains(t, api, `export const listDealCustomerIdOptions = (keyword: string, deleted = false) =>`)
	assert.Contains(t, api, `deleted: deleted ? 1 : undefined`)
	form := fileByPath(t, res, "Dialog.vue").Content
	assert.Contains(t, form, `:fetch="listDealCustomerIdOptions"`)
	assert.Contains(t, form, `:label="props.row?.customerName"`)
	assert.Contains(t, form, `customerId: form.customerId ?? ""`)
	assert.Contains(t, form, `ownerId: form.ownerId || null`)
	index := fileByPath(t, res, "/deal/index.vue").Content
	assert.Contains(t, index, `{{ row.customerName || "-" }}`)
	assert.Contains(t, index, `import ReRemoteSelect from "@/components/ReRemoteSelect/index.vue";`)
	assert.Contains(t, strings.Join(res.Notes, "\n"), "关联字段 customer_id 必填")

	// Turned off: an ID input, no lookup code.
	spec.Fields = []FieldOption{{Column: "customer_id", Relation: ptr("")}, {Column: "owner_id", Relation: ptr("")}}
	res, err = Generate(dealTable(), spec)
	require.NoError(t, err)
	assert.NotContains(t, fileByPath(t, res, "/deal.go").Content, "lookup")

	spec.Fields = []FieldOption{{Column: "owner_id", Relation: ptr("password_hash")}}
	require.Len(t, spec.FieldProblems(dealTable()), 1)
}
