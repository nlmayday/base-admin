//go:build integration

package codegen

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/testinfra/migrated"
)

const fixtureDDL = `
CREATE TABLE biz_gadgets (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        varchar(64)  NOT NULL,
    code        varchar(32)  NOT NULL DEFAULT '',
    price       numeric(12, 2),
    released_on date,
    meta        jsonb,
    tags        text[],
    dept_id     bigint,
    status      smallint     NOT NULL DEFAULT 1,
    version     integer      NOT NULL DEFAULT 0,
    created_at  timestamptz  NOT NULL DEFAULT now(),
    created_by  bigint,
    deleted_at  timestamptz
);
COMMENT ON TABLE biz_gadgets IS '小工具';
COMMENT ON COLUMN biz_gadgets.name IS '名称，必填';
CREATE UNIQUE INDEX biz_gadgets_code_uq ON biz_gadgets (code);
CREATE UNIQUE INDEX biz_gadgets_name_dept_uq ON biz_gadgets (name, dept_id);
CREATE TABLE biz_legacy (id serial PRIMARY KEY, title text);
CREATE TABLE biz_pairs (a bigint, b bigint, PRIMARY KEY (a, b));
CREATE TABLE biz_tags (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, code varchar(16), title text, secret_token text);
CREATE TABLE biz_items (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tag_id   bigint REFERENCES biz_tags (id),
    owner_id bigint REFERENCES sys_users (id),
    plain_id bigint
);
`

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(context.Background(), cfg, "codegen-test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := database.NewGorm(sqlDB, cfg, zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, db.Exec(fixtureDDL).Error)
	return db
}

func TestIntrospect(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	tb, err := Introspect(ctx, db, "biz_gadgets")
	require.NoError(t, err)
	assert.Equal(t, "小工具", tb.Comment)
	require.Len(t, tb.Columns, 13)

	byName := map[string]Column{}
	for _, c := range tb.Columns {
		byName[c.Name] = c
	}
	assert.Equal(t, Column{Name: "id", UDT: "int8", Identity: true, Primary: true}, byName["id"])
	assert.Equal(t, Column{Name: "name", UDT: "varchar", MaxLength: 64, Comment: "名称，必填"}, byName["name"])
	assert.Equal(t, "''::character varying", byName["code"].Default)
	assert.True(t, byName["code"].Unique, "single-column unique index")
	assert.False(t, byName["name"].Unique, "multi-column unique indexes are not per column")
	assert.Equal(t, "numeric", byName["price"].UDT)
	assert.True(t, byName["price"].Nullable)
	assert.Equal(t, "_text", byName["tags"].UDT)
	assert.Equal(t, "now()", byName["created_at"].Default)
	assert.Equal(t, "id", tb.Columns[0].Name, "columns keep table order")
	require.NoError(t, CheckTable(tb))

	res, err := Generate(tb, Spec{Module: "gadget", Resource: "gadget", Name: "小工具", NewModule: true})
	require.NoError(t, err)
	assert.Contains(t, res.Notes[0], "tags")

	_, err = Introspect(ctx, db, "no_such_table")
	assert.True(t, errors.Is(err, ErrTableNotFound))
}

func TestListTables(t *testing.T) {
	db := openDB(t)
	tables, err := ListTables(context.Background(), db)
	require.NoError(t, err)
	byName := map[string]TableInfo{}
	for _, tb := range tables {
		byName[tb.Name] = tb
		assert.NotContains(t, tb.Name, "sys_", "core tables are excluded")
		assert.NotEqual(t, "schema_migrations", tb.Name)
	}
	assert.Equal(t, TableInfo{Name: "biz_gadgets", Comment: "小工具", Supported: true, Columns: 13}, byName["biz_gadgets"])
	assert.True(t, byName["biz_legacy"].Supported == false && byName["biz_legacy"].Reason != "", "serial (int4) ids are rejected")
	assert.False(t, byName["biz_pairs"].Supported)
}

func TestIntrospectForeignKeys(t *testing.T) {
	db := openDB(t)
	tb, err := Introspect(context.Background(), db, "biz_items")
	require.NoError(t, err)
	byName := map[string]Column{}
	for _, c := range tb.Columns {
		byName[c.Name] = c
	}
	assert.Equal(t, &Ref{Table: "biz_tags", Labels: []string{"title", "code"}}, byName["tag_id"].Ref,
		"preferred label columns first; sensitive columns are not offered")
	owner := byName["owner_id"].Ref
	require.NotNil(t, owner)
	assert.Equal(t, "sys_users", owner.Table)
	assert.Equal(t, []string{"nickname", "username"}, owner.Labels[:2])
	assert.NotContains(t, owner.Labels, "password_hash")
	assert.True(t, owner.SoftDelete)
	assert.Nil(t, byName["plain_id"].Ref, "no foreign key")
}

// TestTableSpecRoundTrip applies the SQL of a TableSpec and generates a
// module from the resulting table: the conventions it writes are the ones
// the generator reads (relations, uploads, options, standard columns).
func TestTableSpecRoundTrip(t *testing.T) {
	db := openDB(t)
	customers := TableSpec{Name: "biz_customers", Label: "客户", Columns: []ColumnSpec{
		{Name: "name", Type: ColString, Label: "客户名称", Required: true, Unique: true},
	}}
	for _, spec := range []TableSpec{customers, orderSpec()} {
		require.Empty(t, spec.Problems())
		up, _ := spec.MigrationSQL()
		require.NoError(t, db.Exec(up).Error, up)
	}
	tb, err := Introspect(context.Background(), db, "biz_orders")
	require.NoError(t, err)
	require.NoError(t, CheckTable(tb))
	infos, err := Describe(tb)
	require.NoError(t, err)
	byColumn := map[string]FieldInfo{}
	for _, f := range infos {
		byColumn[f.Column] = f
	}
	assert.Equal(t, "name", byColumn["customer_id"].Relation)
	assert.Equal(t, UploadImage, byColumn["cover_url"].Upload)
	assert.Equal(t, UploadFiles, byColumn["attachments"].Upload)
	assert.Equal(t, OptionsFromComment, byColumn["kind"].Dict)
	assert.Equal(t, OptionsFromComment, byColumn["channel"].Dict)
	assert.Equal(t, "订单号", byColumn["order_no"].Label)

	res, err := Generate(tb, Spec{Module: "sales", Resource: "order", Name: "订单", NewModule: true})
	require.NoError(t, err)
	assert.NotContains(t, strings.Join(res.Notes, "\n"), "表缺少标准字段")

	// An omitted optional column gets the database default on create.
	defaults := map[string]string{}
	for _, f := range buildData(tb, Spec{Module: "sales", Resource: "order", Name: "订单"}).Writable {
		if f.InsertVal == "?" {
			continue
		}
		var got string
		require.NoError(t, db.Raw("SELECT ("+f.InsertVal+")::text", nil).Scan(&got).Error, f.InsertVal)
		defaults[f.Column] = got
	}
	assert.NotEmpty(t, defaults["ordered_at"], "DEFAULT now()")
	delete(defaults, "ordered_at")
	assert.Equal(t, map[string]string{"kind": "1", "channel": "web"}, defaults)
}
