package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGINCORSAllowsInterfaceRevisionHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ginCORS())
	engine.PATCH("/api/api-automation/interfaces/:id", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodOptions, "/api/api-automation/interfaces/8", nil)
	request.Header.Set("Origin", "http://127.0.0.1:4173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	request.Header.Set("Access-Control-Request-Headers", "authorization,content-type,if-match")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("预检状态码 = %d，期望 %d", response.Code, http.StatusNoContent)
	}
	allowed := strings.ToLower(response.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(allowed, "if-match") {
		t.Fatalf("允许请求头中缺少 If-Match：%s", allowed)
	}
}
