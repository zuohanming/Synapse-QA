package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type PerformanceController struct {
	performanceService *service.PerformanceService
}

func NewPerformanceController(performanceService *service.PerformanceService) *PerformanceController {
	return &PerformanceController{performanceService: performanceService}
}

func (ctl *PerformanceController) ListPlans(c *gin.Context) {
	page, pageSize := pageParams(c)
	result, err := ctl.performanceService.ListPlans(c.Request.Context(), planFilter(c), page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询性能测试方案失败")
		return
	}
	ok(c, result)
}

func (ctl *PerformanceController) GetPlan(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	result, err := ctl.performanceService.GetPlan(c.Request.Context(), id)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *PerformanceController) CreatePlan(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.PerfTestPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	id, err := ctl.performanceService.CreatePlan(c.Request.Context(), claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]any{"id": id, "message": "性能测试方案已创建"})
}

func (ctl *PerformanceController) UpdatePlan(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.PerfTestPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.performanceService.UpdatePlan(c.Request.Context(), claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "性能测试方案已更新"})
}

func (ctl *PerformanceController) DeletePlan(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.performanceService.DeletePlan(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "性能测试方案已删除"})
}

func (ctl *PerformanceController) RunPlan(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	runID, err := ctl.performanceService.RunPlan(c.Request.Context(), claims.Username, id)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]any{"id": runID, "message": "已创建执行记录，等待压测执行器接入"})
}

func (ctl *PerformanceController) ListRuns(c *gin.Context) {
	page, pageSize := pageParams(c)
	result, err := ctl.performanceService.ListRuns(c.Request.Context(), runFilter(c), page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询执行记录失败")
		return
	}
	ok(c, result)
}

func (ctl *PerformanceController) GetRun(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	result, err := ctl.performanceService.GetRun(c.Request.Context(), id)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, result)
}

func planFilter(c *gin.Context) model.PerfTestPlanFilter {
	return model.PerfTestPlanFilter{
		ID:        c.Query("id"),
		Name:      c.Query("name"),
		ProductID: c.Query("productId"),
		LoadMode:  c.Query("loadMode"),
		Priority:  c.Query("priority"),
		Status:    c.Query("status"),
		Owner:     c.Query("owner"),
	}
}

func runFilter(c *gin.Context) model.PerfTestRunFilter {
	return model.PerfTestRunFilter{
		ID:     c.Query("id"),
		PlanID: c.Query("planId"),
		Status: c.Query("status"),
	}
}
