package audit

import "github.com/gin-gonic/gin"

// moduleName is a module identifier with its display name.
type moduleName struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

// listModules names the modules recorded in operation logs. A module's
// name is the title of its top-level directory menu, so generated modules
// are named without a code change and renamed menus follow.
func (m *Module) listModules(c *gin.Context) (any, error) {
	rows := []moduleName{}
	// The first top-level directory of each module, by sort then id.
	err := m.db.WithContext(c.Request.Context()).Raw(`SELECT name, title FROM (
			SELECT module AS name, title, sort, id,
				ROW_NUMBER() OVER (PARTITION BY module ORDER BY sort, id) AS rn
			FROM sys_menus
			WHERE source = 'module' AND type = 'dir' AND parent_id IS NULL AND module <> ''
		) d WHERE rn = 1 ORDER BY sort, id`).Scan(&rows).Error
	return rows, err
}
