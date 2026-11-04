package codegen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/database"
)

func orderSpec() TableSpec {
	return TableSpec{
		Name: "biz_orders", Label: "订单", Status: true, Dept: true, SoftDelete: true,
		Columns: []ColumnSpec{
			{Name: "order_no", Type: ColString, Label: "订单号", Length: 32, Required: true, Unique: true},
			{Name: "customer_id", Type: ColRef, Ref: "biz_customers", Label: "客户", Required: true},
			{Name: "kind", Type: ColInt, Label: "类型", Required: true, Default: "1",
				Options: []EnumOption{{Value: "1", Label: "普通"}, {Value: "2", Label: "加急"}}},
			{Name: "amount", Type: ColDecimal, Label: "金额", Required: true, Default: "0"},
			{Name: "paid", Type: ColBool, Label: "已付款"},
			{Name: "ordered_at", Type: ColDatetime, Label: "下单时间", Default: "now"},
			{Name: "cover_url", Type: ColImage, Label: "封面"},
			{Name: "attachments", Type: ColAttachments, Label: "附件"},
			{Name: "remark", Type: ColText, Label: "备注"},
			{Name: "channel", Type: ColString, Label: "渠道", Length: 8, Default: "web",
				Options: []EnumOption{{Value: "web", Label: "网页"}, {Value: "app", Label: "应用"}}},
		},
	}
}

func TestTableSpecSQL(t *testing.T) {
	spec := orderSpec()
	require.Empty(t, spec.Problems())
	up, down := spec.MigrationSQL(database.Postgres)
	for _, want := range []string{
		"CREATE TABLE biz_orders (",
		"id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY",
		"order_no varchar(32) NOT NULL",
		"customer_id bigint NOT NULL REFERENCES biz_customers (id)",
		"kind integer NOT NULL DEFAULT 1 CHECK (kind IN (1, 2))",
		"amount numeric(12, 2) NOT NULL DEFAULT 0",
		"paid boolean NOT NULL DEFAULT false",
		"ordered_at timestamptz DEFAULT now()",
		"cover_url varchar(512) NOT NULL DEFAULT ''",
		"attachments jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(attachments) = 'array')",
		"remark text,",
		"channel varchar(8) DEFAULT 'web' CHECK (channel IN ('web', 'app'))",
		"status smallint NOT NULL DEFAULT 1 CHECK (status IN (1, 2))",
		"dept_id bigint REFERENCES sys_departments (id)",
		"version integer NOT NULL DEFAULT 0",
		"created_by bigint",
		"deleted_at timestamptz\n);",
		"COMMENT ON TABLE biz_orders IS '订单';",
		"COMMENT ON COLUMN biz_orders.kind IS '类型：1=普通,2=加急';",
		"CREATE UNIQUE INDEX biz_orders_order_no_uq ON biz_orders (order_no) WHERE deleted_at IS NULL;",
		"CREATE INDEX biz_orders_customer_id_idx ON biz_orders (customer_id);",
		"CREATE INDEX biz_orders_dept_id_idx ON biz_orders (dept_id);",
	} {
		assert.Contains(t, up, want)
	}
	assert.Equal(t, "DROP TABLE IF EXISTS biz_orders;\n", down)
	// The option comment is read back by the generator.
	assert.Equal(t, []EnumOption{{"1", "普通"}, {"2", "加急"}}, parseCommentOptions("类型：1=普通,2=加急"))
}

func TestTableSpecMySQL(t *testing.T) {
	spec := orderSpec()
	up, down := spec.MigrationSQL(database.MySQL)
	for _, want := range []string{
		"CREATE TABLE biz_orders (",
		"id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY",
		"order_no varchar(32) NOT NULL COMMENT '订单号'",
		"customer_id bigint NOT NULL COMMENT '客户'",
		"kind integer NOT NULL DEFAULT 1 CHECK (kind IN (1, 2)) COMMENT '类型：1=普通,2=加急'",
		"amount decimal(12, 2) NOT NULL DEFAULT 0",
		"paid boolean NOT NULL DEFAULT false",
		"ordered_at datetime(6) DEFAULT CURRENT_TIMESTAMP(6)",
		"cover_url varchar(512) NOT NULL DEFAULT ''",
		"attachments json NOT NULL DEFAULT (JSON_ARRAY()) CHECK (JSON_TYPE(attachments) = 'ARRAY')",
		"remark mediumtext COMMENT '备注'",
		"channel varchar(8) DEFAULT 'web' CHECK (channel IN ('web', 'app'))",
		"status smallint NOT NULL DEFAULT 1 COMMENT '状态' CHECK (status IN (1, 2))",
		"dept_id bigint COMMENT '所属部门'",
		"created_at datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)",
		"deleted_at datetime(6),",
		"UNIQUE KEY biz_orders_order_no_uq ((CASE WHEN deleted_at IS NULL THEN order_no END))",
		"KEY biz_orders_customer_id_idx (customer_id)",
		"CONSTRAINT biz_orders_customer_id_fk FOREIGN KEY (customer_id) REFERENCES biz_customers (id)",
		"CONSTRAINT biz_orders_dept_id_fk FOREIGN KEY (dept_id) REFERENCES sys_departments (id)",
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs COMMENT='订单';",
	} {
		assert.Contains(t, up, want)
	}
	assert.Equal(t, 1, strings.Count(up, ";"), "one statement: MySQL DDL is not transactional")
	assert.Equal(t, "DROP TABLE IF EXISTS biz_orders;\n", down)

	spec = TableSpec{Name: "biz_notes", Label: "笔记", Columns: []ColumnSpec{
		{Name: "title", Type: ColString, Label: "标题", Default: `it's \n`, Unique: true},
		{Name: "body", Type: ColText, Label: "正文", Default: "x"},
	}}
	require.Empty(t, spec.Problems())
	up, _ = spec.MigrationSQL(database.MySQL)
	assert.Contains(t, up, `DEFAULT 'it''s \\n'`, "backslashes are escaped")
	assert.Contains(t, up, "body mediumtext DEFAULT ('x')", "text takes an expression default")
	assert.Contains(t, up, "UNIQUE KEY biz_notes_title_uq (title)")
}

func TestTableSpecEscapesText(t *testing.T) {
	spec := TableSpec{Name: "biz_notes", Label: "笔记", Columns: []ColumnSpec{
		{Name: "title", Type: ColString, Label: "标题", Default: "it's"},
	}}
	require.Empty(t, spec.Problems())
	up, _ := spec.MigrationSQL(database.Postgres)
	assert.Contains(t, up, "DEFAULT 'it''s'")
}

func TestTableSpecProblems(t *testing.T) {
	spec := TableSpec{Name: "sys_hack", Label: "", Columns: []ColumnSpec{
		{Name: "status", Type: ColInt, Label: "状态"},
		{Name: "Bad", Type: "money", Label: "x"},
		{Name: "owner", Type: ColRef, Label: "负责人"},
		{Name: "price", Type: ColDecimal, Label: "价格", Precision: 50},
		{Name: "flag", Type: ColBool, Label: "标记", Default: "yes"},
		{Name: "level", Type: ColInt, Label: "级别", Default: "3",
			Options: []EnumOption{{Value: "1", Label: "低"}, {Value: "x", Label: "高"}}},
		{Name: "note", Type: ColText, Label: "备注');--"},
		{Name: "note", Type: ColJSON, Label: "重复", Unique: true},
		{Name: "select", Type: ColText, Label: "关键字"},
		{Name: "when_at", Type: ColDatetime, Label: "时间", Default: "2020-01-01"},
	}}
	got := map[string]string{}
	for _, p := range spec.Problems() {
		got[p.Field] = p.Message
	}
	for field, part := range map[string]string{
		"name":                        "sys_",
		"label":                       "不能为空",
		"columns[0].name":             "标准字段",
		"columns[1].name":             "小写字母",
		"columns[1].type":             "类型须为",
		"columns[2].ref":              "ref 指定",
		"columns[2].name":             "_id",
		"columns[3].precision":        "精度",
		"columns[4].default":          "true 或 false",
		"columns[5].options[1].value": "整数",
		"columns[5].default":          "选项之一",
		"columns[6].label":            "不能包含",
		"columns[7].name":             "重复",
		"columns[7].unique":           "唯一",
		"columns[8].name":             "关键字",
		"columns[9].default":          "now",
	} {
		assert.Contains(t, got[field], part, field)
	}
	assert.NotEmpty(t, TableSpec{Name: "biz_x", Label: "x"}.Problems(), "at least one column")
}

func TestMigrationName(t *testing.T) {
	assert.Equal(t, "000014_create_biz_orders", MigrationName(13, "biz_orders"))
	n, ok := ParseMigrationNumber("000013_notice_attachments.up.sql")
	assert.True(t, ok)
	assert.Equal(t, uint(13), n)
	_, ok = ParseMigrationNumber("embed.go")
	assert.False(t, ok)
	assert.True(t, strings.HasPrefix(MigrationName(99, "t"), "000100_"))
}
