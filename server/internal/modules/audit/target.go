package audit

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// targetKey holds the operation's target in the Gin context.
const targetKey = "audit.target"

// maxTargetLength bounds the stored target (sys_operation_logs.target).
const maxTargetLength = 255

// SetTarget records what the request operates on, written "<kind>:<id>"
// (player:123, order:A2026), in its operation log; the log list and export
// filter by it exactly. Several targets of a batch operation are joined
// with commas. Call it in the handler, also when the request fails, so
// failed attempts on an object are found too:
//
//	audit.SetTarget(c, "player:"+playerID)
//
// Without a call, a route with an :id parameter gets a target from its path
// (PUT /users/:id/status → user:<id>); SetTarget(c) with no targets keeps
// the target empty. Requests that are not logged (GET without
// Doc.Sensitive) ignore it.
func SetTarget(c *gin.Context, targets ...string) {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		if t = strings.TrimSpace(t); t != "" {
			parts = append(parts, t)
		}
	}
	c.Set(targetKey, strings.Join(parts, ","))
}

// targetOf returns the target set by SetTarget, else the one the route's
// :id names, else "".
func targetOf(c *gin.Context) string {
	if v, ok := c.Get(targetKey); ok {
		s, _ := v.(string)
		return s
	}
	return pathTarget(c.FullPath(), c.Param("id"))
}

// pathTarget is "<kind>:<id>" for a route like /api/v1/dict-types/:id/items:
// the kind is the singular of the segment before :id (dict-type).
func pathTarget(route, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	segments := strings.Split(strings.Trim(route, "/"), "/")
	for i, s := range segments {
		if s == ":id" && i > 0 && !strings.HasPrefix(segments[i-1], ":") {
			return singular(segments[i-1]) + ":" + id
		}
	}
	return ""
}

// singular undoes the plurals of route names (users, categories, boxes).
func singular(s string) string {
	switch {
	case strings.HasSuffix(s, "ies"):
		return strings.TrimSuffix(s, "ies") + "y"
	case strings.HasSuffix(s, "sses"), strings.HasSuffix(s, "xes"),
		strings.HasSuffix(s, "ches"), strings.HasSuffix(s, "shes"):
		return strings.TrimSuffix(s, "es")
	case strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss"):
		return strings.TrimSuffix(s, "s")
	}
	return s
}
