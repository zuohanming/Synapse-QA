package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
)

// ExecutionService 是 ExecutionController 依赖的最小接口。
type ExecutionService interface {
	ListRunsScoped(ctx context.Context, claims model.Claims, filter model.ExecutionRunFilter, page, pageSize int) (model.PageResult, error)
	StatisticsScoped(ctx context.Context, claims model.Claims) (model.ExecutionStatistics, error)
	GetRunScoped(ctx context.Context, claims model.Claims, id int64) (model.ExecutionRunDetail, error)
	CreateRunScoped(ctx context.Context, claims model.Claims, req model.ExecutionRunRequest) (model.ExecutionRunDetail, error)
	StartDebug(ctx context.Context, actor string, req model.ExecutionDebugRequest) (model.ExecutionDebugStart, error)
	GetDebugTask(ctx context.Context, executorID, taskID string) (map[string]any, error)
	CancelRunScoped(ctx context.Context, claims model.Claims, id int64) error
	HandleCallback(ctx context.Context, req model.ExecutionCallbackRequest) error
	ListLogsScoped(ctx context.Context, claims model.Claims, taskID int64) ([]model.ExecutionLog, error)
}

// StartDebug 创建页面步骤即时调试任务。
func (ctl *ExecutionController) StartDebug(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.ExecutionDebugRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	result, err := ctl.executionService.StartDebug(c.Request.Context(), claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, result)
}

// GetDebug 查询页面步骤即时调试状态。
func (ctl *ExecutionController) GetDebug(c *gin.Context) {
	if _, exists := claimsFromContext(c); !exists {
		return
	}
	result, err := ctl.executionService.GetDebugTask(c.Request.Context(), c.Query("executorId"), c.Param("taskId"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

// ExecutionController 处理执行批次相关 HTTP 请求。
type ExecutionController struct {
	executionService ExecutionService
}

func NewExecutionController(executionService ExecutionService) *ExecutionController {
	return &ExecutionController{executionService: executionService}
}

// List 查询执行批次列表。
func (ctl *ExecutionController) List(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	page, pageSize := pageParams(c)
	filter := model.ExecutionRunFilter{
		ID:      c.Query("id"),
		RunType: c.Query("runType"),
		Status:  c.Query("status"),
	}
	result, err := ctl.executionService.ListRunsScoped(c.Request.Context(), claims, filter, page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *ExecutionController) Statistics(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	statistics, err := ctl.executionService.StatisticsScoped(c.Request.Context(), claims)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, statistics)
}

func buildExecutionStatistics(runs []model.ExecutionRun, now time.Time) model.ExecutionStatistics {
	result := model.ExecutionStatistics{Trend: make([]model.ExecutionTrendPoint, 14)}
	start := dayStart(now).AddDate(0, 0, -13)
	currentStart := dayStart(now).AddDate(0, 0, -6)
	previousStart := currentStart.AddDate(0, 0, -7)
	var currentRuns, previousRuns, currentCases, previousCases, currentPassed, previousPassed int64
	for index := range result.Trend {
		result.Trend[index].Date = start.AddDate(0, 0, index).Format("01-02")
	}
	for _, run := range runs {
		var summary model.ExecutionSummary
		_ = json.Unmarshal(run.Summary, &summary)
		result.TotalRuns++
		result.TotalCases += summary.Total
		result.PassedCases += summary.Passed
		result.FailedCases += summary.Failed
		if run.Status == "failed" {
			result.FailedRuns++
		}
		if run.Status == "pending" || run.Status == "queued" || run.Status == "running" {
			result.RunningRuns++
		}
		created := run.CreatedAt
		if !created.Before(currentStart) {
			currentRuns++
			currentCases += summary.Total
			currentPassed += summary.Passed
		} else if !created.Before(previousStart) {
			previousRuns++
			previousCases += summary.Total
			previousPassed += summary.Passed
		}
		dayIndex := int(dayStart(created).Sub(start).Hours() / 24)
		if dayIndex >= 0 && dayIndex < len(result.Trend) {
			point := &result.Trend[dayIndex]
			point.Runs++
			point.Cases += summary.Total
			point.PassRate += float64(summary.Passed)
		}
	}
	result.PassRate = percent(result.PassedCases, result.TotalCases)
	result.RunChange = changeRate(currentRuns, previousRuns)
	result.CaseChange = changeRate(currentCases, previousCases)
	result.PassRateChange = percent(currentPassed, currentCases) - percent(previousPassed, previousCases)
	for index := range result.Trend {
		result.Trend[index].PassRate = percent(int64(result.Trend[index].PassRate), result.Trend[index].Cases)
	}
	return result
}

func dayStart(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func percent(value, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}

func changeRate(current, previous int64) float64 {
	if previous == 0 {
		if current > 0 {
			return 100
		}
		return 0
	}
	return float64(current-previous) * 100 / float64(previous)
}

// Get 查询执行批次详情。
func (ctl *ExecutionController) Get(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	detail, err := ctl.executionService.GetRunScoped(c.Request.Context(), claims, id)
	if err != nil {
		if err.Error() == "执行批次不存在" {
			fail(c, http.StatusNotFound, err.Error())
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, detail)
}

// Create 创建执行批次。
func (ctl *ExecutionController) Create(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.ExecutionRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	detail, err := ctl.executionService.CreateRunScoped(c.Request.Context(), claims, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, detail)
}

// Cancel 取消执行批次。
func (ctl *ExecutionController) Cancel(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.executionService.CancelRunScoped(c.Request.Context(), claims, id); err != nil {
		if err.Error() == "执行批次不存在" {
			fail(c, http.StatusNotFound, err.Error())
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "执行批次已取消"})
}

// Callback 接收执行器任务回调。
func (ctl *ExecutionController) Callback(c *gin.Context) {
	var req model.ExecutionCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.executionService.HandleCallback(c.Request.Context(), req); err != nil {
		if err.Error() == "任务不存在" {
			fail(c, http.StatusNotFound, err.Error())
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "回调已处理"})
}

// ListTaskLogs 查询任务日志。
func (ctl *ExecutionController) ListTaskLogs(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	taskID, err := strconv.ParseInt(c.Param("taskId"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "任务 ID 无效")
		return
	}
	logs, err := ctl.executionService.ListLogsScoped(c.Request.Context(), claims, taskID)
	if err != nil {
		if err.Error() == "执行批次不存在" {
			fail(c, http.StatusNotFound, err.Error())
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, logs)
}
