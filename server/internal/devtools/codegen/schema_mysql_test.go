package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMySQLDefault(t *testing.T) {
	ptr := func(s string) *string { return &s }
	for _, tc := range []struct {
		udt, def, extra, want string
		null                  bool
	}{
		{udt: "varchar", def: "it's", want: "'it''s'"},
		{udt: "varchar", def: `a\b`, want: `'a\\b'`},
		{udt: "int4", def: "-1", want: "-1"},
		{udt: "numeric", def: "1.50", want: "1.50"},
		{udt: "bool", def: "0", want: "false"},
		{udt: "bool", def: "1", want: "true"},
		{udt: "date", def: "2026-01-02", want: "'2026-01-02'"},
		{udt: "varchar", null: true, want: ""},
		{udt: "timestamptz", def: "CURRENT_TIMESTAMP(6)", extra: "default_generated", want: "CURRENT_TIMESTAMP(6)"},
		{udt: "json", def: "json_array()", extra: "default_generated", want: "'[]'"},
		{udt: "text", def: `_utf8mb4\'x\\\'y\'`, extra: "default_generated", want: "'x''y'"},
		// 无 as information_schema shows it (latin1 decoded twice).
		{udt: "text", def: "_utf8mb4\\'æ\u0097 \\'", extra: "default_generated", want: "'无'"},
		{udt: "text", def: `_utf8mb4\'café\'`, extra: "default_generated", want: "'café'"},
	} {
		def := ptr(tc.def)
		if tc.null {
			def = nil
		}
		assert.Equal(t, tc.want, mysqlDefault(tc.udt, def, tc.extra), "%s %q", tc.udt, tc.def)
	}
}

func TestMySQLUDT(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"bigint", "bigint unsigned"}: "int8", {"int", "int"}: "int4", {"tinyint", "tinyint(1)"}: "bool",
		{"tinyint", "tinyint"}: "int2", {"decimal", "decimal(12,2)"}: "numeric", {"char", "char(36)"}: "bpchar",
		{"mediumtext", "mediumtext"}: "text", {"datetime", "datetime(6)"}: "timestamptz", {"json", "json"}: "json",
		{"blob", "blob"}: "blob",
	} {
		assert.Equal(t, want, mysqlUDT(in[0], in[1]), in)
	}
}

func TestMySQLVisibleUnique(t *testing.T) {
	m := mysqlVisibleUnique.FindStringSubmatch("(case when (`deleted_at` is null) then `isbn` end)")
	assert.Equal(t, []string{"(case when (`deleted_at` is null) then `isbn` end)", "isbn"}, m)
	assert.Nil(t, mysqlVisibleUnique.FindStringSubmatch("(lower(`name`))"))
}
