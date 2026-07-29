package controller

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type elementCaptureService interface {
	CreateSession(context.Context, string, model.CaptureSessionCreateRequest) (model.CaptureSessionCreated, error)
	GetSession(context.Context, int64, string) (model.ElementCaptureSessionDetail, error)
	SetMode(context.Context, string, string, string) error
	StopSession(context.Context, string, string) error
	ListCandidates(context.Context, int64, string, int64, int) ([]model.ElementCaptureCandidate, error)
	UpdateCandidate(context.Context, string, string, int64, model.CaptureCandidateUpdateRequest) error
	BatchSave(context.Context, string, model.CandidateBatchSaveRequest) (model.BatchSaveResult, error)
	Heartbeat(context.Context, string, string, string, string, string) error
	AddCandidate(context.Context, string, string, model.CaptureCandidateCreateRequest) (model.ElementCaptureCandidate, error)
	AuthorizeExecutor(context.Context, string, string, string) error
	FailSession(context.Context, string, string, string, string) error
	ListCommands(context.Context, string, string, string) ([]model.ElementCaptureCommand, error)
	ListVersions(context.Context, int64, int64) ([]model.PageElementVersion, error)
	RollbackVersion(context.Context, string, int64, int) (model.PageElementVersion, error)
}

type ElementCaptureController struct{ service elementCaptureService }

func NewElementCaptureController(service elementCaptureService) *ElementCaptureController {
	return &ElementCaptureController{service: service}
}

func (ctl *ElementCaptureController) CreateSession(c *gin.Context) {
	claims, authorized := claimsFromContext(c)
	if !authorized {
		return
	}
	var req model.CaptureSessionCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	item, err := ctl.service.CreateSession(c.Request.Context(), claims.Username, req)
	if err != nil {
		failCaptureError(c, err)
		return
	}
	created(c, item)
}

func (ctl *ElementCaptureController) GetSession(c *gin.Context) {
	claims, authorized := claimsFromContext(c)
	if !authorized {
		return
	}
	item, err := ctl.service.GetSession(c.Request.Context(), claims.UserID, c.Param("id"))
	if err != nil {
		failCaptureError(c, err)
		return
	}
	ok(c, item)
}

func (ctl *ElementCaptureController) SetMode(c *gin.Context) {
	claims, authorized := claimsFromContext(c)
	if !authorized {
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	if err := ctl.service.SetMode(c.Request.Context(), claims.Username, c.Param("id"), req.Mode); err != nil {
		failCaptureError(c, err)
		return
	}
	ok(c, gin.H{"message": "采集模式已更新"})
}

func (ctl *ElementCaptureController) StopSession(c *gin.Context) {
	claims, authorized := claimsFromContext(c)
	if !authorized {
		return
	}
	if err := ctl.service.StopSession(c.Request.Context(), claims.Username, c.Param("id")); err != nil {
		failCaptureError(c, err)
		return
	}
	ok(c, gin.H{"message": "采集会话已停止"})
}

func (ctl *ElementCaptureController) ListCandidates(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	claims, authorized := claimsFromContext(c)
	if !authorized {
		return
	}
	afterID, err := queryInt64(c, "afterId", 0)
	if err != nil {
		fail(c, http.StatusBadRequest, "afterId 无效")
		return
	}
	limit, present, err := queryInt(c, "limit", 100)
	if err != nil || (present && limit == 0) {
		fail(c, http.StatusBadRequest, "limit 无效")
		return
	}
	items, err := ctl.service.ListCandidates(c.Request.Context(), claims.UserID, c.Param("id"), afterID, limit)
	if err != nil {
		failCaptureError(c, err)
		return
	}
	ok(c, items)
}

func (ctl *ElementCaptureController) UpdateCandidate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	claims, authorized := claimsFromContext(c)
	if !authorized {
		return
	}
	candidateID, err := strconv.ParseInt(c.Param("candidateId"), 10, 64)
	if err != nil || candidateID <= 0 {
		fail(c, http.StatusBadRequest, "候选项 ID 无效")
		return
	}
	var req model.CaptureCandidateUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	if err := ctl.service.UpdateCandidate(c.Request.Context(), claims.Username, c.Param("id"), candidateID, req); err != nil {
		failCaptureError(c, err)
		return
	}
	ok(c, gin.H{"message": "候选项已更新"})
}

func (ctl *ElementCaptureController) SaveCandidates(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	claims, authorized := claimsFromContext(c)
	if !authorized {
		return
	}
	var req model.CandidateBatchSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	req.SessionID = c.Param("id")
	result, err := ctl.service.BatchSave(c.Request.Context(), claims.Username, req)
	if err != nil {
		failCaptureError(c, err)
		return
	}
	ok(c, result)
}

func (ctl *ElementCaptureController) ListVersions(c *gin.Context) {
	claims, authorized := claimsFromContext(c)
	if !authorized {
		return
	}
	elementID, valid := idParam(c)
	if !valid {
		return
	}
	items, err := ctl.service.ListVersions(c.Request.Context(), claims.UserID, elementID)
	if err != nil {
		failCaptureError(c, err)
		return
	}
	ok(c, items)
}

func (ctl *ElementCaptureController) RollbackVersion(c *gin.Context) {
	claims, authorized := claimsFromContext(c)
	if !authorized {
		return
	}
	elementID, valid := idParam(c)
	if !valid {
		return
	}
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil || version <= 0 {
		fail(c, http.StatusBadRequest, "版本号无效")
		return
	}
	item, err := ctl.service.RollbackVersion(c.Request.Context(), claims.Username, elementID, version)
	if err != nil {
		failCaptureError(c, err)
		return
	}
	ok(c, item)
}

func (ctl *ElementCaptureController) Heartbeat(c *gin.Context) {
	executorID, token, authorized := executorCredentials(c)
	if !authorized {
		return
	}
	if err := ctl.service.AuthorizeExecutor(c.Request.Context(), c.Param("id"), executorID, token); err != nil {
		failExecutorCaptureError(c, err)
		return
	}
	var req model.CaptureHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	if req.ExecutorID != "" && req.ExecutorID != executorID {
		failExecutorCaptureError(c, unauthorizedExecutorError())
		return
	}
	if err := ctl.service.Heartbeat(c.Request.Context(), c.Param("id"), executorID, token, req.BrowserContextID, req.CurrentURL); err != nil {
		failExecutorCaptureError(c, err)
		return
	}
	ok(c, gin.H{"message": "心跳已接收"})
}

func (ctl *ElementCaptureController) AddCandidate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	executorID, token, authorized := executorCredentials(c)
	if !authorized {
		return
	}
	if err := ctl.service.AuthorizeExecutor(c.Request.Context(), c.Param("id"), executorID, token); err != nil {
		failExecutorCaptureError(c, err)
		return
	}
	var req model.CaptureCandidateCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	req.SessionID = c.Param("id")
	if req.ExecutorID != "" && req.ExecutorID != executorID {
		failExecutorCaptureError(c, unauthorizedExecutorError())
		return
	}
	if req.SessionID == "" {
		fail(c, http.StatusBadRequest, "执行器候选项参数无效")
		return
	}
	item, err := ctl.service.AddCandidate(c.Request.Context(), executorID, token, req)
	if err != nil {
		failExecutorCaptureError(c, err)
		return
	}
	created(c, item)
}

func (ctl *ElementCaptureController) FailSession(c *gin.Context) {
	executorID, token, authorized := executorCredentials(c)
	if !authorized {
		return
	}
	if err := ctl.service.AuthorizeExecutor(c.Request.Context(), c.Param("id"), executorID, token); err != nil {
		failExecutorCaptureError(c, err)
		return
	}
	var req model.CaptureFailureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	if req.ExecutorID != "" && req.ExecutorID != executorID {
		failExecutorCaptureError(c, unauthorizedExecutorError())
		return
	}
	if err := ctl.service.FailSession(c.Request.Context(), c.Param("id"), executorID, token, req.Reason); err != nil {
		failExecutorCaptureError(c, err)
		return
	}
	ok(c, gin.H{"message": "采集失败状态已记录"})
}

func (ctl *ElementCaptureController) ListCommands(c *gin.Context) {
	executorID, token, authorized := executorCredentials(c)
	if !authorized {
		return
	}
	items, err := ctl.service.ListCommands(c.Request.Context(), executorID, c.Query("sessionId"), token)
	if err != nil {
		failExecutorCaptureError(c, err)
		return
	}
	ok(c, items)
}

func queryInt64(c *gin.Context, key string, fallback int64) (int64, error) {
	if c.Query(key) == "" {
		return fallback, nil
	}
	return strconv.ParseInt(c.Query(key), 10, 64)
}
func queryInt(c *gin.Context, key string, fallback int) (int, bool, error) {
	if c.Query(key) == "" {
		return fallback, false, nil
	}
	value, err := strconv.Atoi(c.Query(key))
	return value, true, err
}
func failExecutorCaptureError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrUnauthorized) {
		fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	failCaptureError(c, err)
}
func failCaptureError(c *gin.Context, err error) {
	var issues *service.CandidateIssuesError
	if errors.As(err, &issues) {
		c.JSON(http.StatusConflict, model.APIResponse{Error: issues.Error(), Data: gin.H{"issues": issues.Issues}})
		return
	}
	if errors.Is(err, model.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusNotFound, "资源不存在")
		return
	}
	if errors.Is(err, model.ErrConflict) {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, model.ErrValidation) {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	fail(c, http.StatusInternalServerError, "服务器内部错误")
}

func executorCredentials(c *gin.Context) (string, string, bool) {
	executorID := strings.TrimSpace(c.GetHeader("X-Executor-ID"))
	if executorID == "" {
		executorID = strings.TrimSpace(c.Query("executorId"))
	}
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if executorID == "" || !strings.HasPrefix(authorization, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")) == "" {
		fail(c, http.StatusUnauthorized, "执行器认证失败")
		return "", "", false
	}
	return executorID, strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), true
}
func unauthorizedExecutorError() error {
	return model.NewDomainError(model.ErrUnauthorized, "执行器或会话令牌无效")
}
