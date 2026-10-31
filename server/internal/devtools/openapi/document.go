// Package openapi builds an OpenAPI 3.0.3 document from the module route
// registry (docs/p1-p3-plan.md, devtools). Request and response types are
// reflected from module.Doc; the output is deterministic so it can be diffed
// and cached.
package openapi

import (
	"bytes"
	"encoding/json"
)

// Version is the OpenAPI version produced.
const Version = "3.0.3"

// Document is the root OpenAPI object. Maps are marshalled with sorted keys
// by encoding/json, which keeps the output deterministic.
type Document struct {
	OpenAPI    string              `json:"openapi"`
	Info       Info                `json:"info"`
	Servers    []Server            `json:"servers,omitempty"`
	Tags       []Tag               `json:"tags,omitempty"`
	Paths      map[string]PathItem `json:"paths"`
	Components Components          `json:"components"`
}

// Info is the API metadata.
type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

// Server is a base URL.
type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// Tag groups operations; there is one per module.
type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PathItem holds the operations of one path, keyed by lower-case method.
type PathItem map[string]*Operation

// Operation is one route.
type Operation struct {
	Tags        []string              `json:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	OperationID string                `json:"operationId"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]Response   `json:"responses"`
	Security    []map[string][]string `json:"security"`
	// Extensions.
	Access      string   `json:"x-access"`
	Permissions []string `json:"x-permissions"`
	Module      string   `json:"x-module"`
	Sensitive   bool     `json:"x-sensitive,omitempty"`
}

// Parameter is a path or query parameter.
type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema"`
}

// RequestBody is a JSON request body.
type RequestBody struct {
	Required bool                 `json:"required"`
	Content  map[string]MediaType `json:"content"`
}

// Response is one response of an operation.
type Response struct {
	Ref         string               `json:"$ref,omitempty"`
	Description string               `json:"description,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// MediaType wraps a schema.
type MediaType struct {
	Schema *Schema `json:"schema"`
}

// Components holds reusable schemas, responses and security schemes.
type Components struct {
	Schemas         map[string]*Schema        `json:"schemas"`
	Responses       map[string]Response       `json:"responses,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
}

// SecurityScheme is an apiKey scheme (cookie or header).
type SecurityScheme struct {
	Type        string `json:"type"`
	In          string `json:"in"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Schema is the subset of the OpenAPI 3.0 schema object the generator uses.
type Schema struct {
	Ref                  string     `json:"$ref,omitempty"`
	AllOf                []*Schema  `json:"allOf,omitempty"`
	Type                 string     `json:"type,omitempty"`
	Format               string     `json:"format,omitempty"`
	Pattern              string     `json:"pattern,omitempty"`
	Description          string     `json:"description,omitempty"`
	Nullable             bool       `json:"nullable,omitempty"`
	Enum                 []any      `json:"enum,omitempty"`
	MinLength            *int       `json:"minLength,omitempty"`
	MaxLength            *int       `json:"maxLength,omitempty"`
	Minimum              *float64   `json:"minimum,omitempty"`
	Maximum              *float64   `json:"maximum,omitempty"`
	ExclusiveMinimum     bool       `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     bool       `json:"exclusiveMaximum,omitempty"`
	MinItems             *int       `json:"minItems,omitempty"`
	MaxItems             *int       `json:"maxItems,omitempty"`
	Items                *Schema    `json:"items,omitempty"`
	Properties           Properties `json:"properties,omitempty"`
	Required             []string   `json:"required,omitempty"`
	AdditionalProperties *Schema    `json:"additionalProperties,omitempty"`
}

// Property is one named property of an object schema.
type Property struct {
	Name   string
	Schema *Schema
}

// Properties keeps object properties in Go declaration order, which is how
// developers read a struct, while still being deterministic.
type Properties []Property

// MarshalJSON encodes the properties as a JSON object in order.
func (p Properties) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, prop := range p {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(prop.Name)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(prop.Schema)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Get returns the schema of the named property.
func (p Properties) Get(name string) (*Schema, bool) {
	for _, prop := range p {
		if prop.Name == name {
			return prop.Schema, true
		}
	}
	return nil, false
}

// RefPrefix is the JSON pointer prefix of component schemas.
const RefPrefix = "#/components/schemas/"

// RefName returns the component name of a $ref, or "".
func RefName(ref string) string {
	if len(ref) > len(RefPrefix) && ref[:len(RefPrefix)] == RefPrefix {
		return ref[len(RefPrefix):]
	}
	return ""
}

func intPtr(n int) *int { return &n }

func floatPtr(f float64) *float64 { return &f }
