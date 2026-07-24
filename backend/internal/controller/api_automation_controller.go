package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type APIAutomationController struct {
	service *service.APIAutomationService
}

func NewAPIAutomationController(service *service.APIAutomationService) *APIAutomationController {
	return &APIAutomationController{service: service}
}

func (ctl *APIAutomationController) ListInterfaces(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	page, pageSize := pageParams(c)
	result, err := ctl.service.ListInterfaces(c.Request.Context(), claims.UserID, model.APIInterfaceFilter{
		ProjectID: c.Query("projectId"), ProductID: c.Query("productId"), ModuleID: c.Query("moduleId"),
		Keyword: c.Query("keyword"), Method: c.Query("method"), LifecycleStatus: c.Query("lifecycleStatus"),
	}, page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询接口失败")
		return
	}
	ok(c, result)
}

func (ctl *APIAutomationController) GetInterface(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := ctl.service.GetInterface(c.Request.Context(), claims.UserID, id)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *APIAutomationController) CreateInterface(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.APIInterfaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	id, err := ctl.service.CreateInterface(c.Request.Context(), claims.UserID, claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]any{"id": id, "message": "接口已创建"})
}

func (ctl *APIAutomationController) UpdateInterface(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.APIInterfaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if header := c.GetHeader("If-Match"); header != "" {
		req.Revision, _ = strconv.ParseInt(header, 10, 64)
	}
	if err := ctl.service.UpdateInterface(c.Request.Context(), claims.UserID, claims.Username, id, req); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "接口已被其他用户修改，请刷新后重试" {
			status = http.StatusConflict
		}
		fail(c, status, err.Error())
		return
	}
	ok(c, map[string]string{"message": "接口已更新"})
}

func (ctl *APIAutomationController) DeleteInterface(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.service.DeleteInterface(c.Request.Context(), claims.UserID, claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "接口已删除"})
}

func (ctl *APIAutomationController) ListProjectHeaders(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	projectID, err := strconv.ParseInt(c.Query("projectId"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "请选择项目")
		return
	}
	result, err := ctl.service.ListProjectHeaders(c.Request.Context(), claims.UserID, projectID, c.Query("keyword"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *APIAutomationController) CreateProjectHeader(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.APIProjectHeaderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.service.CreateProjectHeader(c.Request.Context(), claims.UserID, claims.Username, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "请求头已创建"})
}

func (ctl *APIAutomationController) UpdateProjectHeader(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.APIProjectHeaderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.service.UpdateProjectHeader(c.Request.Context(), claims.UserID, claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "请求头已更新"})
}

func (ctl *APIAutomationController) DeleteProjectHeader(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.service.DeleteProjectHeader(c.Request.Context(), claims.UserID, claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "请求头已删除"})
}
