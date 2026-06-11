package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

// SystemController 负责系统管理相关 HTTP 入口。
type SystemController struct {
	BaseController
}

func NewSystemController(systemService *service.SystemService) *SystemController {
	return &SystemController{BaseController: NewBaseController(systemService)}
}

func (ctl *SystemController) Overview(c *gin.Context) {
	ok(c, ctl.systemService.Overview(c.Request.Context()))
}

func (ctl *SystemController) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	result, err := ctl.systemService.ListUsers(
		c.Request.Context(),
		c.Query("id"),
		c.Query("nickname"),
		c.Query("account"),
		page,
		pageSize,
	)
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询用户失败")
		return
	}
	ok(c, result)
}

func (ctl *SystemController) UpdateUser(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.UserUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体无效")
		return
	}
	if err := ctl.systemService.UpdateUser(c.Request.Context(), claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "用户已更新"})
}

func (ctl *SystemController) DeleteUser(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.systemService.DeleteUser(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "用户已删除"})
}

func (ctl *SystemController) ListRoles(c *gin.Context) {
	roles, err := ctl.systemService.ListRoles(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询角色失败")
		return
	}
	ok(c, roles)
}

func (ctl *SystemController) CreateRole(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.RoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体无效")
		return
	}
	if err := ctl.systemService.CreateRole(c.Request.Context(), claims.Username, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "角色已创建"})
}

func (ctl *SystemController) UpdateRole(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.RoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体无效")
		return
	}
	if err := ctl.systemService.UpdateRole(c.Request.Context(), claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "角色已更新"})
}

func (ctl *SystemController) DeleteRole(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.systemService.DeleteRole(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "角色已删除"})
}
