package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type DashboardController struct {
	dashboardService *service.DashboardService
}

func NewDashboardController(dashboardService *service.DashboardService) *DashboardController {
	return &DashboardController{dashboardService: dashboardService}
}

func (ctl *DashboardController) Overview(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	req, err := dashboardRequest(c)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := ctl.dashboardService.Overview(c.Request.Context(), claims, req)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			fail(c, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, model.ErrValidation) {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
		fail(c, http.StatusInternalServerError, "查询首页概览失败")
		return
	}
	ok(c, result)
}

func dashboardRequest(c *gin.Context) (service.DashboardRequest, error) {
	request := service.DashboardRequest{Range: c.Query("range")}
	projectID := c.Query("projectId")
	if projectID == "" {
		return request, nil
	}
	id, err := strconv.ParseInt(projectID, 10, 64)
	if err != nil || id <= 0 {
		return service.DashboardRequest{}, errors.New("项目 ID 无效")
	}
	request.ProjectID = &id
	return request, nil
}
