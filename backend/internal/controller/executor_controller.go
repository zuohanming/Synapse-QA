package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

// ExecutorController 处理执行器注册、心跳和平台查询。
type ExecutorController struct {
	executorService *service.ExecutorService
}

func NewExecutorController(executorService *service.ExecutorService) *ExecutorController {
	return &ExecutorController{executorService: executorService}
}

func (ctl *ExecutorController) Register(c *gin.Context) {
	var req model.ExecutorRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	item, err := ctl.executorService.Register(c.Request.Context(), c.GetHeader("X-Executor-Token"), req)
	if err != nil {
		failExecutorError(c, err)
		return
	}
	ok(c, item)
}

func (ctl *ExecutorController) Heartbeat(c *gin.Context) {
	var req model.ExecutorHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	item, err := ctl.executorService.Heartbeat(c.Request.Context(), c.GetHeader("X-Executor-Token"), req)
	if err != nil {
		failExecutorError(c, err)
		return
	}
	ok(c, item)
}

func (ctl *ExecutorController) List(c *gin.Context) {
	items, err := ctl.executorService.List(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询执行器失败")
		return
	}
	ok(c, items)
}

func failExecutorError(c *gin.Context, err error) {
	if err.Error() == "执行器令牌无效" {
		fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	fail(c, http.StatusBadRequest, err.Error())
}
