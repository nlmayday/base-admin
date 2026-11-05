package audit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestPathTarget(t *testing.T) {
	cases := []struct{ route, id, want string }{
		{"/api/v1/users/:id", "5", "user:5"},
		{"/api/v1/users/:id/status", "5", "user:5"},
		{"/api/v1/dict-types/:id/items", "3", "dict-type:3"},
		{"/api/v1/categories/:id", "9", "category:9"},
		{"/api/v1/boxes/:id", "2", "box:2"},
		{"/api/v1/addresses/:id", "2", "address:2"},
		{"/api/v1/job-runs/:id", "8", "job-run:8"},
		{"/api/v1/me/notices/:id/read", "4", "notice:4"},
		{"/api/v1/users", "", ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, pathTarget(tc.route, tc.id), tc.route)
	}
}

func TestTargetOfPrefersSetTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	got := map[string]string{}
	r := gin.New()
	r.PUT("/users/:id", func(c *gin.Context) { got["path"] = targetOf(c) })
	r.PUT("/players/:id", func(c *gin.Context) {
		SetTarget(c, "player:A1")
		got["set"] = targetOf(c)
	})
	r.PUT("/silent/:id", func(c *gin.Context) {
		SetTarget(c)
		got["empty"] = targetOf(c)
	})
	for _, p := range []string{"/users/5", "/players/5", "/silent/5"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, p, nil))
	}
	assert.Equal(t, map[string]string{"path": "user:5", "set": "player:A1", "empty": ""}, got)
}
