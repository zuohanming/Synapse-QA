package controller

import (
	"errors"
	"net/http"
	"strings"

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
	var req model.PerfRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	run, reused, err := ctl.performanceService.RunPlan(c.Request.Context(), claims.Username, req.IdempotencyKey, id)
	if err != nil {
		if errors.Is(err, service.ErrPerfConfigConflict) {
			fail(c, http.StatusConflict, err.Error())
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if reused {
		ok(c, run)
		return
	}
	created(c, run)
}

func (ctl *PerformanceController) CancelRun(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.performanceService.CancelRun(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "性能测试执行已取消"})
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

// Callback 接收执行器回调，凭据为 Bearer callback token，task_id 仅定位。
func (ctl *PerformanceController) Callback(c *gin.Context) {
	taskID := c.Param("taskId")
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	var req model.PerfCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	err := ctl.performanceService.HandleCallback(c.Request.Context(), taskID, token, req)
	if err != nil {
		switch err.Error() {
		case "回调凭据无效":
			fail(c, http.StatusUnauthorized, err.Error())
		case "执行记录不存在":
			fail(c, http.StatusNotFound, err.Error())
		case "执行已终结":
			fail(c, http.StatusGone, err.Error())
		case "回调状态冲突":
			fail(c, http.StatusConflict, err.Error())
		default:
			fail(c, http.StatusBadRequest, err.Error())
		}
		return
	}
	ok(c, map[string]string{"message": "回调已处理"})
}

// StartSmoke 下发一次冒烟测试请求（SPEC §8.5），不创建执行记录。
func (ctl *PerformanceController) StartSmoke(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.PerfSmokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	taskID, err := ctl.performanceService.StartSmoke(c.Request.Context(), claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"taskId": taskID})
}

// GetSmoke 轮询冒烟测试任务结果。
func (ctl *PerformanceController) GetSmoke(c *gin.Context) {
	taskID := c.Param("taskId")
	result, err := ctl.performanceService.GetSmoke(c.Request.Context(), taskID)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

func planFilter(c *gin.Context) model.PerfTestPlanFilter {
	return model.PerfTestPlanFilter{
		ID:           c.Query("id"),
		Name:         c.Query("name"),
		ProductID:    c.Query("productId"),
		ScenarioType: c.Query("scenarioType"),
		Environment:  c.Query("environment"),
		Priority:     c.Query("priority"),
		Status:       c.Query("status"),
		Owner:        c.Query("owner"),
	}
}

func runFilter(c *gin.Context) model.PerfTestRunFilter {
	return model.PerfTestRunFilter{
		ID:           c.Query("id"),
		PlanID:       c.Query("planId"),
		ScenarioType: c.Query("scenarioType"),
		Status:       c.Query("status"),
		Environment:  c.Query("environment"),
	}
}
