package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

const validCaptureSessionBody = `{"pageId":8,"executorId":"exec-1","browserChannel":"chrome","url":"https://example.test"}`

type captureControllerServiceStub struct {
	createErr    error
	listErr      error
	heartbeatErr error
	rollbackErr  error
	created      model.CaptureSessionCreated
	candidates   []model.ElementCaptureCandidate
	rollbackID   int64
	rollbackVer  int
	ackID        int64
}

func (s *captureControllerServiceStub) CreateSession(context.Context, string, model.CaptureSessionCreateRequest) (model.CaptureSessionCreated, error) {
	return s.created, s.createErr
}
func (s *captureControllerServiceStub) GetSession(context.Context, int64, string) (model.ElementCaptureSessionDetail, error) {
	return model.ElementCaptureSessionDetail{}, nil
}
func (s *captureControllerServiceStub) SetMode(context.Context, string, string, string) error {
	return nil
}
func (s *captureControllerServiceStub) StopSession(context.Context, string, string) error { return nil }
func (s *captureControllerServiceStub) ListCandidates(context.Context, int64, string, int64, int) ([]model.ElementCaptureCandidate, error) {
	return s.candidates, s.listErr
}
func (s *captureControllerServiceStub) UpdateCandidate(context.Context, string, string, int64, model.CaptureCandidateUpdateRequest) error {
	return nil
}
func (s *captureControllerServiceStub) DeleteCandidate(context.Context, string, string, int64) error { return nil }
func (s *captureControllerServiceStub) BatchSave(context.Context, string, model.CandidateBatchSaveRequest) (model.BatchSaveResult, error) {
	return model.BatchSaveResult{}, nil
}
func (s *captureControllerServiceStub) Heartbeat(context.Context, string, string, string, string, string, string) error {
	return s.heartbeatErr
}
func (s *captureControllerServiceStub) AddCandidate(context.Context, string, string, model.CaptureCandidateCreateRequest) (model.ElementCaptureCandidate, error) {
	return model.ElementCaptureCandidate{}, nil
}
func (s *captureControllerServiceStub) AuthorizeExecutor(_ context.Context, _, _, token string) error {
	if token == "" {
		return model.NewDomainError(model.ErrUnauthorized, "执行器或会话令牌无效")
	}
	return nil
}
func (s *captureControllerServiceStub) FailSession(context.Context, string, string, string, string) error {
	return nil
}
func (s *captureControllerServiceStub) ListCommands(context.Context, string, string, string) ([]model.ElementCaptureCommand, error) {
	return nil, nil
}
func (s *captureControllerServiceStub) AckCommand(_ context.Context, _, _ string, id int64, _ string) error {
	s.ackID = id
	return nil
}
func (s *captureControllerServiceStub) ListVersions(context.Context, int64, int64) ([]model.PageElementVersion, error) {
	return nil, nil
}
func (s *captureControllerServiceStub) RollbackVersion(_ context.Context, _ string, elementID int64, version int) (model.PageElementVersion, error) {
	s.rollbackID, s.rollbackVer = elementID, version
	return model.PageElementVersion{}, s.rollbackErr
}

func captureTestRouter(claims model.Claims, stub *captureControllerServiceStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("claims", claims) })
	controller := NewElementCaptureController(stub)
	platform := engine.Group("/api/ui/page-elements")
	platform.POST("/capture-sessions", RequirePermission("ui.element.capture"), controller.CreateSession)
	platform.GET("/capture-sessions/:id/candidates", RequirePermission("ui.element.read"), controller.ListCandidates)
	platform.POST("/:id/versions/:version/rollback", RequirePermission("ui.element.rollback"), controller.RollbackVersion)
	executor := engine.Group("/api/executor/element-capture")
	executor.POST("/:id/heartbeat", controller.Heartbeat)
	executor.POST("/commands/:id/ack", controller.AckCommand)
	return engine
}

func performCaptureJSON(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func TestElementCaptureSessionRequiresCapturePermission(t *testing.T) {
	router := captureTestRouter(model.Claims{Permissions: []string{"ui.element.read"}}, &captureControllerServiceStub{})
	response := performCaptureJSON(router, http.MethodPost, "/api/ui/page-elements/capture-sessions", validCaptureSessionBody)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestElementCaptureCandidatesRequireReadPermissionAndDisableCaching(t *testing.T) {
	stub := &captureControllerServiceStub{candidates: []model.ElementCaptureCandidate{}}
	denied := captureTestRouter(model.Claims{Permissions: []string{"ui.element.capture"}}, stub)
	if response := performCaptureJSON(denied, http.MethodGet, "/api/ui/page-elements/capture-sessions/session-1/candidates", ""); response.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d", response.Code)
	}
	allowed := captureTestRouter(model.Claims{Permissions: []string{"ui.element.read"}}, stub)
	response := performCaptureJSON(allowed, http.MethodGet, "/api/ui/page-elements/capture-sessions/session-1/candidates", "")
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d cache=%q", response.Code, response.Header().Get("Cache-Control"))
	}
}

func TestElementCaptureCandidatesExposeConflictTargetIDsAndNames(t *testing.T) {
	stub := &captureControllerServiceStub{candidates: []model.ElementCaptureCandidate{{
		CursorID: 1,
		ConflictTargets: []model.ElementCaptureTarget{
			{ID: 42, Name: "提交按钮"},
			{ID: 43, Name: "提交副本"},
		},
	}}}
	router := captureTestRouter(model.Claims{Permissions: []string{"ui.element.read"}}, stub)
	response := performCaptureJSON(router, http.MethodGet, "/api/ui/page-elements/capture-sessions/session-1/candidates", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data []model.ElementCaptureCandidate `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("响应 JSON 无效：%v", err)
	}
	if len(payload.Data) != 1 || len(payload.Data[0].ConflictTargets) != 2 || payload.Data[0].ConflictTargets[1].Name != "提交副本" {
		t.Fatalf("控制器响应丢失冲突目标：%s", response.Body.String())
	}
}

func TestElementCaptureRollbackRequiresRollbackPermission(t *testing.T) {
	stub := &captureControllerServiceStub{}
	denied := captureTestRouter(model.Claims{Permissions: []string{"ui.element.manage"}}, stub)
	if response := performCaptureJSON(denied, http.MethodPost, "/api/ui/page-elements/9/versions/2/rollback", "{}"); response.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d", response.Code)
	}
	allowed := captureTestRouter(model.Claims{Permissions: []string{"ui.element.rollback"}}, stub)
	if response := performCaptureJSON(allowed, http.MethodPost, "/api/ui/page-elements/9/versions/2/rollback", "{}"); response.Code != http.StatusOK {
		t.Fatalf("allowed status=%d", response.Code)
	}
	if stub.rollbackID != 9 || stub.rollbackVer != 2 {
		t.Fatalf("rollback=%d/%d", stub.rollbackID, stub.rollbackVer)
	}
}

func TestElementCaptureExecutorHeartbeatRejectsInvalidToken(t *testing.T) {
	stub := &captureControllerServiceStub{heartbeatErr: errors.New("采集会话不存在、执行器或令牌无效，或恢复窗口已过期")}
	router := captureTestRouter(model.Claims{}, stub)
	response := performCaptureJSON(router, http.MethodPost, "/api/executor/element-capture/session-1/heartbeat", `{"executorId":"exec-1","token":"bad","browserContextId":"ctx-1","currentUrl":"https://example.test"}`)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestElementCaptureCandidateIssuesUseUnifiedBadRequestResponse(t *testing.T) {
	stub := &captureControllerServiceStub{listErr: &service.CandidateIssuesError{Issues: []model.CandidateIssue{{CandidateID: 1, Field: "name", Message: "候选项名称不可靠"}}}}
	router := captureTestRouter(model.Claims{Permissions: []string{"ui.element.read"}}, stub)
	response := performCaptureJSON(router, http.MethodGet, "/api/ui/page-elements/capture-sessions/session-1/candidates", "")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "候选项名称不可靠") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestElementCaptureCandidatesRejectExplicitZeroLimit(t *testing.T) {
	router := captureTestRouter(model.Claims{Permissions: []string{"ui.element.read"}}, &captureControllerServiceStub{})
	response := performCaptureJSON(router, http.MethodGet, "/api/ui/page-elements/capture-sessions/session-1/candidates?limit=0", "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestElementCaptureExecutorRejectsInvalidTokenBeforeParsingPayload(t *testing.T) {
	router := captureTestRouter(model.Claims{}, &captureControllerServiceStub{})
	response := performCaptureJSON(router, http.MethodPost, "/api/executor/element-capture/session-1/heartbeat", "{")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestElementCaptureCommandAckRequiresLongTokenAndAcknowledgesOwnCommand(t *testing.T) {
	stub := &captureControllerServiceStub{}
	router := captureTestRouter(model.Claims{}, stub)
	request := httptest.NewRequest(http.MethodPost, "/api/executor/element-capture/commands/7/ack?executorId=exec-1", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/executor/element-capture/commands/7/ack?executorId=exec-1", strings.NewReader(`{"receipt":"receipt"}`))
	request.Header.Set("X-Executor-Token", "long-token")
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || stub.ackID != 7 {
		t.Fatalf("status=%d ack=%d", response.Code, stub.ackID)
	}
}

func TestElementCaptureDoesNotExposeUnknownServiceErrors(t *testing.T) {
	router := captureTestRouter(model.Claims{Permissions: []string{"ui.element.read"}}, &captureControllerServiceStub{listErr: errors.New("database password leaked")})
	response := performCaptureJSON(router, http.MethodGet, "/api/ui/page-elements/capture-sessions/session-1/candidates", "")
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "password") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
