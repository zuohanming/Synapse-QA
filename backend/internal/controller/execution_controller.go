package controller

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
)

// ExecutionService 是 ExecutionController 依赖的最小接口。
type ExecutionService interface {
	ListRuns(ctx context.Context, filter model.ExecutionRunFilter, page, pageSize int) (model.PageResult, error)
	GetRun(ctx context.Context, id int64) (model.ExecutionRunDetail, error)
	CreateRun(ctx context.Context, actor string, req model.ExecutionRunRequest) (model.ExecutionRunDetail, error)
	StartDebug(ctx context.Context, actor string, req model.ExecutionDebugRequest) (model.ExecutionDebugStart, error)
	GetDebugTask(ctx context.Context, executorID, taskID string) (map[string]any, error)
	CancelRun(ctx context.Context, actor string, id int64) error
	HandleCallback(ctx context.Context, req model.ExecutionCallbackRequest) error
	ListLogs(ctx context.Context, taskID int64) ([]model.ExecutionLog, error)
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
	if _, exists := claimsFromContext(c); !exists {
		return
	}
	page, pageSize := pageParams(c)
	filter := model.ExecutionRunFilter{
		ID:      c.Query("id"),
		RunType: c.Query("runType"),
		Status:  c.Query("status"),
	}
	result, err := ctl.executionService.ListRuns(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

// Get 查询执行批次详情。
func (ctl *ExecutionController) Get(c *gin.Context) {
	if _, exists := claimsFromContext(c); !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	detail, err := ctl.executionService.GetRun(c.Request.Context(), id)
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
	detail, err := ctl.executionService.CreateRun(c.Request.Context(), claims.Username, req)
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
	if err := ctl.executionService.CancelRun(c.Request.Context(), claims.Username, id); err != nil {
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
	if _, exists := claimsFromContext(c); !exists {
		return
	}
	taskID, err := strconv.ParseInt(c.Param("taskId"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "任务 ID 无效")
		return
	}
	logs, err := ctl.executionService.ListLogs(c.Request.Context(), taskID)
	if err != nil {
		if err.Error() == "任务不存在" {
			fail(c, http.StatusNotFound, err.Error())
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, logs)
}
