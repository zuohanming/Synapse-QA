package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

// AuthController 负责认证相关 HTTP 入口。
type AuthController struct {
	BaseController
}

func NewAuthController(systemService *service.SystemService) *AuthController {
	return &AuthController{BaseController: NewBaseController(systemService)}
}

func (ctl *AuthController) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体无效")
		return
	}
	result, err := ctl.systemService.Login(c.Request.Context(), req, clientIP(c))
	if err != nil {
		fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *AuthController) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体无效")
		return
	}
	if err := ctl.systemService.Register(c.Request.Context(), req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "注册成功"})
}

func (ctl *AuthController) Me(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	user, err := ctl.systemService.CurrentUser(c.Request.Context(), claims.Username)
	if err != nil {
		fail(c, http.StatusUnauthorized, "用户不存在")
		return
	}
	ok(c, user)
}

func (ctl *AuthController) ChangePassword(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.ChangePasswordRequest
	if c.ShouldBindJSON(&req) != nil {
		fail(c, http.StatusBadRequest, "密码参数无效")
		return
	}
	result, err := ctl.systemService.ChangePassword(c.Request.Context(), claims, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}
