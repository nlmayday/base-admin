package module

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Report is the outcome of startup validation. Any error must stop startup;
// warnings are logged.
type Report struct {
	Errors   []string
	Warnings []string
}

// Err returns the errors joined, or nil.
func (r Report) Err() error {
	if len(r.Errors) == 0 {
		return nil
	}
	errs := make([]error, len(r.Errors))
	for i, e := range r.Errors {
		errs[i] = errors.New(e)
	}
	return fmt.Errorf("module validation failed:\n%w", errors.Join(errs...))
}

func (r *Report) errorf(format string, args ...any) {
	r.Errors = append(r.Errors, fmt.Sprintf(format, args...))
}

func (r *Report) warnf(format string, args ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
}

// Validate checks modules, the route registry and the Gin engine against the
// rules in design doc 4.3. apiPrefix is the path under which every route
// must have been registered through a Router.
func Validate(mods []Module, reg *Registry, engineRoutes gin.RoutesInfo, apiPrefix string) Report {
	var rep Report
	validateModuleNames(&rep, mods)
	defs := collectPermissions(&rep, mods)
	menus := collectMenus(&rep, mods, defs)
	validateRequires(&rep, defs)
	validatePermissionMenus(&rep, defs, menus)
	used := validateRoutes(&rep, reg.Routes(), defs)
	validateEngineRoutes(&rep, reg.Routes(), engineRoutes, apiPrefix)

	for _, code := range sortedKeys(defs) {
		if _, ok := used[code]; !ok {
			rep.warnf("permission %s is not used by any route (frontend-only button?)", code)
		}
	}
	return rep
}

func validateModuleNames(rep *Report, mods []Module) {
	seen := map[string]bool{}
	for _, m := range mods {
		name := m.Name()
		if !namePattern.MatchString(name) {
			rep.errorf("module name %q is invalid", name)
		}
		if seen[name] {
			rep.errorf("module %q is registered twice", name)
		}
		seen[name] = true
	}
}

func collectPermissions(rep *Report, mods []Module) map[string]perm.Def {
	defs := map[string]perm.Def{}
	owner := map[string]string{}
	for _, m := range mods {
		for _, d := range m.Permissions() {
			mod, _, _, ok := perm.Parts(d.Code)
			switch {
			case !ok:
				rep.errorf("module %s: permission code %q is not module:resource:action", m.Name(), d.Code)
				continue
			case mod != m.Name():
				rep.errorf("module %s: permission code %s must start with %q", m.Name(), d.Code, m.Name()+":")
			}
			if prev, dup := owner[d.Code]; dup {
				rep.errorf("permission %s is declared by both %s and %s", d.Code, prev, m.Name())
				continue
			}
			owner[d.Code] = m.Name()
			defs[d.Code] = d
		}
	}
	return defs
}

func collectMenus(rep *Report, mods []Module, defs map[string]perm.Def) map[string]menu.Def {
	menus := map[string]menu.Def{}
	for _, m := range mods {
		for _, d := range m.Menus() {
			if !strings.HasPrefix(d.Code, m.Name()+".") {
				rep.errorf("module %s: menu code %q must start with %q", m.Name(), d.Code, m.Name()+".")
			}
			if _, dup := menus[d.Code]; dup {
				rep.errorf("menu %s is declared twice", d.Code)
				continue
			}
			validateMenuDef(rep, d, defs)
			menus[d.Code] = d
		}
	}
	for _, code := range sortedKeys(menus) {
		if p := menus[code].Parent; p != "" {
			if _, ok := menus[p]; !ok {
				rep.errorf("menu %s: parent %s does not exist", code, p)
			}
		}
	}
	validateMenuCycles(rep, menus)
	return menus
}

func validateMenuDef(rep *Report, d menu.Def, defs map[string]perm.Def) {
	if !d.Type.Valid() {
		rep.errorf("menu %s: invalid type %q", d.Code, d.Type)
	}
	if d.Title == "" {
		rep.errorf("menu %s: title is required", d.Code)
	}
	switch d.Type {
	case menu.TypeMenu:
		if d.Path == "" || d.Component == "" {
			rep.errorf("menu %s: path and component are required", d.Code)
		}
		if !perm.IsView(d.Permission) {
			rep.errorf("menu %s: a page menu must reference a view permission, got %q", d.Code, d.Permission)
		}
	case menu.TypeDir:
		if d.Permission != "" {
			rep.errorf("menu %s: directories must not reference a permission", d.Code)
		}
	case menu.TypeLink:
		if d.ExternalURL == "" {
			rep.errorf("menu %s: links require an external URL", d.Code)
		}
	}
	if d.Permission != "" {
		if _, ok := defs[d.Permission]; !ok {
			rep.errorf("menu %s: permission %s is not declared", d.Code, d.Permission)
		}
	}
}

func validateMenuCycles(rep *Report, menus map[string]menu.Def) {
	for _, code := range sortedKeys(menus) {
		seen := map[string]bool{code: true}
		for p := menus[code].Parent; p != ""; p = menus[p].Parent {
			if seen[p] {
				rep.errorf("menu %s: parent chain contains a cycle", code)
				break
			}
			seen[p] = true
		}
	}
}

func validateRequires(rep *Report, defs map[string]perm.Def) {
	graph := map[string][]string{}
	for _, code := range sortedKeys(defs) {
		reqs := defs[code].EffectiveRequires()
		for _, r := range reqs {
			if _, ok := defs[r]; !ok {
				rep.errorf("permission %s requires undeclared permission %s", code, r)
			}
		}
		graph[code] = reqs
	}
	if cycle := perm.FindCycle(graph); cycle != nil {
		rep.errorf("permission dependencies contain a cycle: %s", perm.FormatCycle(cycle))
	}
}

func validatePermissionMenus(rep *Report, defs map[string]perm.Def, menus map[string]menu.Def) {
	for _, code := range sortedKeys(defs) {
		if m := defs[code].Menu; m != "" {
			if _, ok := menus[m]; !ok {
				rep.errorf("permission %s: menu %s does not exist", code, m)
			}
		}
	}
}

func validateRoutes(rep *Report, routes []Route, defs map[string]perm.Def) map[string]struct{} {
	used := map[string]struct{}{}
	seen := map[string]bool{}
	var public []string
	for _, r := range routes {
		key := r.Method + " " + r.Path
		if seen[key] {
			rep.errorf("route %s is registered twice", key)
		}
		seen[key] = true
		if !r.Access.Valid() {
			rep.errorf("route %s has no valid access level", key)
			continue
		}
		if r.Access.Kind() == access.KindPublic {
			public = append(public, key)
			if r.MaxBodyBytes > 0 {
				rep.errorf("public route %s cannot raise the body limit (MaxBody)", key)
			}
		}
		for _, code := range r.Access.Codes() {
			if _, ok := defs[code]; !ok {
				rep.errorf("route %s references undeclared permission %s", key, code)
			}
			used[code] = struct{}{}
		}
	}
	slices.Sort(public)
	for _, p := range public {
		rep.warnf("public route: %s", p)
	}
	return used
}

func validateEngineRoutes(rep *Report, routes []Route, engineRoutes gin.RoutesInfo, apiPrefix string) {
	registered := map[string]bool{}
	for _, r := range routes {
		registered[r.Method+" "+r.Path] = true
	}
	for _, er := range engineRoutes {
		if !strings.HasPrefix(er.Path, apiPrefix) {
			continue
		}
		if key := er.Method + " " + er.Path; !registered[key] {
			rep.errorf("route %s bypasses the module router", key)
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
