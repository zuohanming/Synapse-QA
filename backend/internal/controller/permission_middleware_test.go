package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
)

func TestRequirePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		claims     model.Claims
		permission string
		want       int
	}{
		{name: "超级管理员始终允许", claims: model.Claims{RoleCode: "admin"}, permission: "system.user.manage", want: http.StatusNoContent},
		{name: "具有权限允许", claims: model.Claims{Permissions: []string{"system.user.read"}}, permission: "system.user.read", want: http.StatusNoContent},
		{name: "缺少权限拒绝", claims: model.Claims{Permissions: []string{"system.user.read"}}, permission: "system.user.manage", want: http.StatusForbidden},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			engine := gin.New()
			engine.GET("/", func(c *gin.Context) {
				c.Set("claims", item.claims)
			}, RequirePermission(item.permission), func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			if response.Code != item.want {
				t.Fatalf("状态码=%d，期望=%d", response.Code, item.want)
			}
		})
	}
}
