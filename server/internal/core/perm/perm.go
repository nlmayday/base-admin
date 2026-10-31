// Package perm defines permission codes and their dependency rules
// (design doc 5.1–5.3).
package perm

import (
	"regexp"
	"slices"
	"strings"
)

// ActionView is the page-access action every page menu must reference.
const ActionView = "view"

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(:[a-z][a-z0-9-]*){2}$`)

// Def declares one permission code.
type Def struct {
	Code string
	Name string
	// Menu groups the code under a menu in the grant UI (menu code).
	Menu string
	// Requires lists codes that must be granted together with this one.
	// nil means "the view code of the same resource" for non-view actions;
	// a non-nil empty slice opts out of any dependency.
	Requires []string
}

// Parts splits a code into module, resource and action.
func Parts(code string) (module, resource, action string, ok bool) {
	if !codePattern.MatchString(code) {
		return "", "", "", false
	}
	p := strings.Split(code, ":")
	return p[0], p[1], p[2], true
}

// ValidCode reports whether code has the module:resource:action form.
func ValidCode(code string) bool {
	_, _, _, ok := Parts(code)
	return ok
}

// ViewCode returns the view code of code's resource.
func ViewCode(code string) string {
	m, r, _, ok := Parts(code)
	if !ok {
		return ""
	}
	return m + ":" + r + ":" + ActionView
}

// IsView reports whether code is a page-access code.
func IsView(code string) bool {
	_, _, a, ok := Parts(code)
	return ok && a == ActionView
}

// EffectiveRequires applies the default dependency rule.
func (d Def) EffectiveRequires() []string {
	if d.Requires != nil {
		return slices.Clone(d.Requires)
	}
	if IsView(d.Code) {
		return []string{}
	}
	return []string{ViewCode(d.Code)}
}

// Catalog is an immutable, validated set of definitions.
type Catalog struct {
	defs     map[string]Def
	requires map[string][]string
}

// NewCatalog indexes defs. It assumes defs passed module validation.
func NewCatalog(defs []Def) Catalog {
	c := Catalog{defs: make(map[string]Def, len(defs)), requires: make(map[string][]string, len(defs))}
	for _, d := range defs {
		c.defs[d.Code] = d
		c.requires[d.Code] = d.EffectiveRequires()
	}
	return c
}

// Has reports whether code is declared.
func (c Catalog) Has(code string) bool {
	_, ok := c.defs[code]
	return ok
}

// Get returns the definition of code.
func (c Catalog) Get(code string) (Def, bool) {
	d, ok := c.defs[code]
	return d, ok
}

// Codes returns all declared codes, sorted.
func (c Catalog) Codes() []string {
	out := make([]string, 0, len(c.defs))
	for code := range c.defs {
		out = append(out, code)
	}
	slices.Sort(out)
	return out
}

// Requires returns the effective dependencies of code.
func (c Catalog) Requires(code string) []string { return slices.Clone(c.requires[code]) }

// MissingDependencies returns, for a candidate grant set, every code that is
// required by a member but absent from the set (design doc 5.3). Unknown
// codes are reported separately so callers can reject them.
func (c Catalog) MissingDependencies(granted []string) (missing, unknown []string) {
	set := make(map[string]struct{}, len(granted))
	for _, g := range granted {
		set[g] = struct{}{}
	}
	seenMissing := map[string]struct{}{}
	for _, g := range granted {
		if !c.Has(g) {
			unknown = append(unknown, g)
			continue
		}
		for _, r := range c.requires[g] {
			if _, ok := set[r]; ok {
				continue
			}
			if _, dup := seenMissing[r]; !dup {
				seenMissing[r] = struct{}{}
				missing = append(missing, r)
			}
		}
	}
	slices.Sort(missing)
	slices.Sort(unknown)
	return missing, unknown
}

// FindCycle returns a dependency cycle as a path, or nil.
func FindCycle(requires map[string][]string) []string {
	const (
		unvisited = iota
		visiting
		done
	)
	state := make(map[string]int, len(requires))
	var stack []string
	var visit func(string) []string
	visit = func(code string) []string {
		switch state[code] {
		case visiting:
			start := slices.Index(stack, code)
			return append(slices.Clone(stack[start:]), code)
		case done:
			return nil
		}
		state[code] = visiting
		stack = append(stack, code)
		for _, next := range requires[code] {
			if cycle := visit(next); cycle != nil {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		state[code] = done
		return nil
	}
	keys := make([]string, 0, len(requires))
	for k := range requires {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if cycle := visit(k); cycle != nil {
			return cycle
		}
	}
	return nil
}

// FormatCycle renders a cycle for error messages.
func FormatCycle(cycle []string) string { return strings.Join(cycle, " -> ") }
