package codegen

import (
	"strconv"
	"strings"
)

// kind is how a column is represented across Go, SQL and TypeScript.
type kind string

const (
	kindString  kind = "string"  // varchar, char, text, citext
	kindInt16   kind = "int16"   // smallint
	kindInt     kind = "int"     // integer
	kindInt64   kind = "int64"   // bigint that is not an id
	kindID      kind = "id"      // bigint id / *_id: a string in JSON
	kindFloat   kind = "float"   // real, double precision
	kindDecimal kind = "decimal" // numeric: a string to keep precision
	kindBool    kind = "bool"
	kindTime    kind = "time" // timestamptz, timestamp
	kindDate    kind = "date" // date: "2006-01-02"
	kindUUID    kind = "uuid"
	kindJSON    kind = "json" // json, jsonb: JSON text
	kindNet     kind = "net"  // inet, cidr
)

// typeInfo describes a supported column type.
type typeInfo struct {
	kind kind
	// goType is the non-pointer Go type of rows and requests.
	goType string
	// castText selects the column as text (e.g. to_char or ::text).
	castText bool
}

// mapType maps a PostgreSQL type; ok is false for unsupported types.
func mapType(c Column) (typeInfo, bool) {
	switch c.UDT {
	case "int2":
		return typeInfo{kind: kindInt16, goType: "int16"}, true
	case "int4":
		return typeInfo{kind: kindInt, goType: "int"}, true
	case "int8":
		if c.Name == "id" || strings.HasSuffix(c.Name, "_id") || c.Name == "created_by" || c.Name == "updated_by" {
			return typeInfo{kind: kindID, goType: "int64"}, true
		}
		return typeInfo{kind: kindInt64, goType: "int64"}, true
	case "float4", "float8":
		return typeInfo{kind: kindFloat, goType: "float64"}, true
	case "numeric":
		return typeInfo{kind: kindDecimal, goType: "string", castText: true}, true
	case "bool":
		return typeInfo{kind: kindBool, goType: "bool"}, true
	case "varchar", "bpchar", "text", "citext", "name":
		return typeInfo{kind: kindString, goType: "string"}, true
	case "timestamptz", "timestamp":
		return typeInfo{kind: kindTime, goType: "time.Time"}, true
	case "date":
		return typeInfo{kind: kindDate, goType: "string", castText: true}, true
	case "uuid":
		return typeInfo{kind: kindUUID, goType: "string", castText: true}, true
	case "json", "jsonb":
		return typeInfo{kind: kindJSON, goType: "string", castText: true}, true
	case "inet", "cidr":
		return typeInfo{kind: kindNet, goType: "string", castText: true}, true
	}
	return typeInfo{}, false
}

// zeroIsInvalid reports kinds whose Go zero value is not a valid database
// value, so NOT NULL columns of these kinds are either required or, when
// the database has a default, left to the database.
func (k kind) zeroIsInvalid() bool {
	switch k {
	case kindTime, kindDate, kindUUID, kindJSON, kindDecimal, kindNet:
		return true
	}
	return false
}

func (k kind) numeric() bool {
	return k == kindInt16 || k == kindInt || k == kindInt64 || k == kindFloat
}

// tsType is the TypeScript type of the JSON value.
func (k kind) tsType() string {
	switch {
	case k.numeric():
		return "number"
	case k == kindBool:
		return "boolean"
	}
	return "string"
}

// parseDefault extracts a literal from a column default such as
// "'abc'::character varying", "0" or "true". ok is false for expressions.
func parseDefault(def string) (string, bool) {
	def = strings.TrimSpace(def)
	if def == "" {
		return "", false
	}
	if strings.HasPrefix(def, "'") {
		end := strings.LastIndex(def, "'")
		if end <= 0 {
			return "", false
		}
		return strings.ReplaceAll(def[1:end], "''", "'"), true
	}
	if i := strings.Index(def, "::"); i > 0 {
		def = def[:i]
	}
	def = strings.Trim(def, "()")
	if def == "true" || def == "false" {
		return def, true
	}
	if _, err := strconv.ParseFloat(def, 64); err == nil {
		return def, true
	}
	return "", false
}
