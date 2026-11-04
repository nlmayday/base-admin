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
// Requests that are not logged (GET without Doc.Sensitive) ignore it.
func SetTarget(c *gin.Context, targets ...string) {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		if t = strings.TrimSpace(t); t != "" {
			parts = append(parts, t)
		}
	}
	c.Set(targetKey, strings.Join(parts, ","))
}

// targetOf returns the target set by SetTarget, or "".
func targetOf(c *gin.Context) string {
	return c.GetString(targetKey)
}
