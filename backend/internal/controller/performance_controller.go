package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	run, reused, err := ctl.performanceService.RunPlanWithEnvironment(c.Request.Context(), claims.Username, req.IdempotencyKey, id, req.EnvironmentID)
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

func (ctl *PerformanceController) ListEnvironments(c *gin.Context) {
	productID, err := strconv.ParseInt(c.Query("productId"), 10, 64)
	if err != nil || productID <= 0 {
		fail(c, http.StatusBadRequest, "产品 ID 无效")
		return
	}
	items, err := ctl.performanceService.ListPerfEnvironments(c.Request.Context(), productID)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]any{"items": items, "total": len(items)})
}

func (ctl *PerformanceController) GetBaseline(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := ctl.performanceService.GetPerfBaseline(c.Request.Context(), id)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *PerformanceController) SetBaseline(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := ctl.performanceService.SetPerfBaseline(c.Request.Context(), claims.Username, id)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *PerformanceController) DeleteBaseline(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := ctl.performanceService.DeletePerfBaseline(c.Request.Context(), id)
	if err != nil {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *PerformanceController) CompareRun(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := ctl.performanceService.ComparePerfRun(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrPerfComparisonNotFinal) {
			fail(c, http.StatusConflict, err.Error())
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *PerformanceController) TrendRun(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	limit := int64(20)
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			fail(c, http.StatusBadRequest, "trend limit 无效")
			return
		}
		limit = parsed
	}
	items, err := ctl.performanceService.TrendPerfRun(c.Request.Context(), id, limit)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, items)
}

func (ctl *PerformanceController) ExportRun(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	data, contentType, filename, err := ctl.performanceService.ExportPerfRun(c.Request.Context(), id, c.Query("format"))
	if err != nil {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, contentType, data)
}

func (ctl *PerformanceController) ListSchedules(c *gin.Context) {
	page, pageSize := pageParams(c)
	var enabled *bool
	if value := strings.TrimSpace(c.Query("enabled")); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			fail(c, http.StatusBadRequest, "enabled 无效")
			return
		}
		enabled = &parsed
	}
	result, err := ctl.performanceService.ListPerfSchedules(c.Request.Context(), model.PerfScheduleFilter{PlanID: c.Query("planId"), Enabled: enabled}, page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *PerformanceController) CreateSchedule(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.PerfScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	item, err := ctl.performanceService.CreatePerfSchedule(c.Request.Context(), claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, item)
}

func (ctl *PerformanceController) UpdateSchedule(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var raw map[string]json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if _, exists := raw["enabled"]; exists {
		fail(c, http.StatusBadRequest, "不能通过部分更新修改 enabled")
		return
	}
	var patch model.PerfSchedulePatch
	copyRaw := map[string]json.RawMessage{}
	for key, value := range raw {
		if key != "environmentId" {
			copyRaw[key] = value
		}
	}
	encoded, _ := json.Marshal(copyRaw)
	if err := json.Unmarshal(encoded, &patch); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if value, exists := raw["environmentId"]; exists {
		var environmentID *int64
		if string(value) != "null" {
			if err := json.Unmarshal(value, &environmentID); err != nil {
				fail(c, http.StatusBadRequest, "environmentId 无效")
				return
			}
		}
		patch.EnvironmentID = &environmentID
	}
	item, err := ctl.performanceService.UpdatePerfSchedule(c.Request.Context(), claims.Username, id, patch)
	if err != nil {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *PerformanceController) EnableSchedule(c *gin.Context)  { ctl.setScheduleEnabled(c, true) }
func (ctl *PerformanceController) DisableSchedule(c *gin.Context) { ctl.setScheduleEnabled(c, false) }
func (ctl *PerformanceController) setScheduleEnabled(c *gin.Context, enabled bool) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := ctl.performanceService.EnablePerfSchedule(c.Request.Context(), claims.Username, id, enabled)
	if err != nil {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *PerformanceController) DeleteSchedule(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.performanceService.DeletePerfSchedule(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	ok(c, map[string]string{"message": "定时任务已删除"})
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
	if runs, ok := result.Items.([]model.PerfTestRun); ok {
		items := make([]map[string]any, 0, len(runs))
		for _, run := range runs {
			encoded, _ := json.Marshal(run)
			var item map[string]any
			_ = json.Unmarshal(encoded, &item)
			delete(item, "summary")
			delete(item, "series")
			delete(item, "planSnapshot")
			delete(item, "diagnosticOutput")
			items = append(items, item)
		}
		result.Items = items
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
	if !limitPerfJSONBody(c, 4<<20) {
		return
	}
	var req model.PerfCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if isPerfBodyTooLarge(err) {
			fail(c, http.StatusRequestEntityTooLarge, "回调请求体过大")
			return
		}
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

func (ctl *PerformanceController) EventCallback(c *gin.Context) {
	taskID := c.Param("taskId")
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !limitPerfJSONBody(c, 64<<10) {
		return
	}
	var event model.PerfSampleEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		if isPerfBodyTooLarge(err) {
			fail(c, http.StatusRequestEntityTooLarge, "采样事件过大")
			return
		}
		fail(c, http.StatusBadRequest, "采样事件格式错误")
		return
	}
	if err := ctl.performanceService.HandleSampleEvent(c.Request.Context(), taskID, token, event); err != nil {
		switch err.Error() {
		case "回调凭据无效":
			fail(c, http.StatusUnauthorized, err.Error())
		case "执行记录不存在":
			fail(c, http.StatusNotFound, err.Error())
		case "执行已终结":
			fail(c, http.StatusGone, err.Error())
		default:
			fail(c, http.StatusBadRequest, err.Error())
		}
		return
	}
	ok(c, map[string]string{"message": "采样事件已处理"})
}

func limitPerfJSONBody(c *gin.Context, limit int64) bool {
	if c.Request.ContentLength > limit {
		fail(c, http.StatusRequestEntityTooLarge, "回调请求体过大")
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	return true
}

func isPerfBodyTooLarge(err error) bool {
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError)
}

func (ctl *PerformanceController) StreamEvents(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	after := parseEventSequence(c.Query("after"))
	if headerAfter := parseEventSequence(c.GetHeader("Last-Event-ID")); headerAfter > after {
		after = headerAfter
	}
	stream, cancel, err := ctl.performanceService.SubscribeRunEvents(c.Request.Context(), id, after)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	defer cancel()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case message, ok := <-stream:
			if !ok {
				return
			}
			data, _ := json.Marshal(message.Data)
			_, _ = fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", message.ID, message.Event, data)
			c.Writer.Flush()
		}
	}
}

func parseEventSequence(value string) int64 {
	sequence, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || sequence < 0 {
		return 0
	}
	return sequence
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
