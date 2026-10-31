// Package menu defines module-declared menus (design doc 4.4 and 5.1).
// Menus are UI structure only; they never carry grants.
package menu

// Type is the menu node type.
type Type string

// Menu node types.
const (
	TypeDir  Type = "dir"
	TypeMenu Type = "menu"
	TypeLink Type = "link"
)

// Valid reports whether t is a known type.
func (t Type) Valid() bool {
	return t == TypeDir || t == TypeMenu || t == TypeLink
}

// Def declares one menu node. Path, Component, Permission and Parent are
// owned by code and overwritten on every start; Title, Icon, Sort and Hidden
// are only used when the node is first created, so administrators can
// change them afterwards.
type Def struct {
	Code        string // stable key, "<module>.<name>"
	Parent      string // parent menu code, "" for top level
	Type        Type
	Title       string
	Path        string
	Component   string
	Icon        string
	Sort        int
	Hidden      bool
	KeepAlive   bool
	Permission  string // required for TypeMenu: a view code
	ExternalURL string
}
