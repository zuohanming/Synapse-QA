package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

// BaseController 放置 Controller 层共用的响应和鉴权辅助方法。
type BaseController struct {
	systemService *service.SystemService
}

func NewBaseController(systemService *service.SystemService) BaseController {
	return BaseController{systemService: systemService}
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, model.APIResponse{Data: data})
}

func created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, model.APIResponse{Data: data})
}

func fail(c *gin.Context, status int, message string) {
	c.JSON(status, model.APIResponse{Error: message})
}

func idParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "ID 无效")
		return 0, false
	}
	return id, true
}

func claimsFromContext(c *gin.Context) (model.Claims, bool) {
	value, exists := c.Get("claims")
	if !exists {
		fail(c, http.StatusUnauthorized, "未登录")
		return model.Claims{}, false
	}
	claims, ok := value.(model.Claims)
	if !ok {
		fail(c, http.StatusUnauthorized, "登录状态无效")
		return model.Claims{}, false
	}
	return claims, true
}

func AuthMiddleware(systemService *service.SystemService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token := strings.TrimPrefix(header, "Bearer ")
		if token == "" {
			fail(c, http.StatusUnauthorized, "未登录")
			c.Abort()
			return
		}
		claims, err := systemService.ParseToken(token)
		if err != nil {
			fail(c, http.StatusUnauthorized, "登录已过期")
			c.Abort()
			return
		}
		claims, err = systemService.ResolveAccess(c.Request.Context(), claims)
		if err != nil {
			fail(c, http.StatusUnauthorized, "登录状态已失效")
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}

func RequirePermission(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := claimsFromContext(c)
		if !ok {
			c.Abort()
			return
		}
		if claims.RoleCode == "admin" {
			c.Next()
			return
		}
		for _, permission := range claims.Permissions {
			if permission == code {
				c.Next()
				return
			}
		}
		fail(c, http.StatusForbidden, "无权执行此操作")
		c.Abort()
	}
}

func clientIP(c *gin.Context) string {
	if ip := c.GetHeader("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		return ip
	}
	return c.ClientIP()
}
