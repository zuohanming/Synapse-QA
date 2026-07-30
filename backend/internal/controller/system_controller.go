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
	result, err := ctl.systemService.Overview(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询系统概览失败")
		return
	}
	ok(c, result)
}

func (ctl *SystemController) ListOperationLogs(c *gin.Context) {
	result, err := ctl.systemService.ListOperationLogs(c.Request.Context(), operationLogFilter(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询操作日志失败")
		return
	}
	ok(c, result)
}

func (ctl *SystemController) ExportOperationLogs(c *gin.Context) {
	content, err := ctl.systemService.ExportOperationLogs(c.Request.Context(), operationLogFilter(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "导出操作日志失败")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="operation-logs.csv"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", content)
}

func operationLogFilter(c *gin.Context) model.OperationLogFilter {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	return model.OperationLogFilter{
		Actor: c.Query("actor"), Action: c.Query("action"), Keyword: c.Query("keyword"),
		DateFrom: c.Query("dateFrom"), DateTo: c.Query("dateTo"), Page: page, PageSize: pageSize,
	}
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

func (ctl *SystemController) CreateUser(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.UserCreateRequest
	if c.ShouldBindJSON(&req) != nil {
		fail(c, http.StatusBadRequest, "用户参数无效")
		return
	}
	result, err := ctl.systemService.CreateManagedUser(c.Request.Context(), claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, result)
}

func (ctl *SystemController) ResetUserPassword(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	password, err := ctl.systemService.ResetUserPassword(c.Request.Context(), claims.Username, id)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"temporaryPassword": password})
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

func (ctl *SystemController) UnlockUser(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.systemService.UnlockUser(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "用户已解锁"})
}

func (ctl *SystemController) ListSettings(c *gin.Context) {
	items, err := ctl.systemService.ListSystemSettings(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询系统参数失败")
		return
	}
	ok(c, items)
}

func (ctl *SystemController) UpdateSettings(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.SystemSettingUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "配置参数无效")
		return
	}
	revision, err := ctl.systemService.UpdateSystemSettings(c.Request.Context(), claims.Username, c.Param("groupKey"), req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"revision": revision})
}

func (ctl *SystemController) ListSettingHistory(c *gin.Context) {
	items, err := ctl.systemService.ListSystemSettingHistory(c.Request.Context(), c.Param("groupKey"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, items)
}

func (ctl *SystemController) RollbackSettings(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.SystemSettingRollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "回滚参数无效")
		return
	}
	revision, err := ctl.systemService.RollbackSystemSettings(c.Request.Context(), claims.Username, c.Param("groupKey"), req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"revision": revision})
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

func (ctl *SystemController) ListPermissions(c *gin.Context) {
	items, err := ctl.systemService.ListPermissions(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询权限失败")
		return
	}
	ok(c, items)
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
