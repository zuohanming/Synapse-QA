package controller_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/controller"
	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/router"
	"synapseqa/backend/internal/service"
)

type routerCaptureService struct{}

func (routerCaptureService) CreateSession(context.Context, string, model.CaptureSessionCreateRequest) (model.CaptureSessionCreated, error) {
	return model.CaptureSessionCreated{}, nil
}
func (routerCaptureService) GetSession(context.Context, int64, string) (model.ElementCaptureSessionDetail, error) {
	return model.ElementCaptureSessionDetail{}, nil
}
func (routerCaptureService) SetMode(context.Context, string, string, string) error { return nil }
func (routerCaptureService) StopSession(context.Context, string, string) error     { return nil }
func (routerCaptureService) ListCandidates(context.Context, int64, string, int64, int) ([]model.ElementCaptureCandidate, error) {
	return nil, nil
}
func (routerCaptureService) UpdateCandidate(context.Context, string, string, int64, model.CaptureCandidateUpdateRequest) error {
	return nil
}
func (routerCaptureService) BatchSave(context.Context, string, model.CandidateBatchSaveRequest) (model.BatchSaveResult, error) {
	return model.BatchSaveResult{}, &service.CandidateIssuesError{Issues: []model.CandidateIssue{{CandidateID: 1, Field: "name", Message: "候选项名称不可靠"}}}
}
func (routerCaptureService) Heartbeat(context.Context, string, string, string, string, string) error {
	return nil
}
func (routerCaptureService) AddCandidate(context.Context, string, string, model.CaptureCandidateCreateRequest) (model.ElementCaptureCandidate, error) {
	return model.ElementCaptureCandidate{}, nil
}
func (routerCaptureService) AuthorizeExecutor(_ context.Context, _, _, token string) error {
	if token != "session-token" {
		return model.NewDomainError(model.ErrUnauthorized, "执行器或会话令牌无效")
	}
	return nil
}
func (routerCaptureService) FailSession(context.Context, string, string, string, string) error {
	return nil
}
func (routerCaptureService) ListCommands(context.Context, string, string, string) ([]model.ElementCaptureCommand, error) {
	return nil, nil
}
func (routerCaptureService) ListVersions(context.Context, int64, int64) ([]model.PageElementVersion, error) {
	return nil, nil
}
func (routerCaptureService) RollbackVersion(context.Context, string, int64, int) (model.PageElementVersion, error) {
	return model.PageElementVersion{}, nil
}

func elementCaptureProductionRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.RegisterRoutes(engine, router.Dependencies{
		AuthController: controller.NewAuthController(nil), SystemController: controller.NewSystemController(nil), CatalogController: controller.NewCatalogController(nil), AutomationController: controller.NewAutomationController(nil), ExecutorController: controller.NewExecutorController(nil), TestCaseController: controller.NewTestCaseController(nil), ExecutionController: controller.NewExecutionController(nil), NotificationController: controller.NewNotificationController(nil), APIAutomationController: controller.NewAPIAutomationController(nil),
		ElementCaptureController: controller.NewElementCaptureController(routerCaptureService{}),
		AuthMiddleware: func(c *gin.Context) {
			c.Set("claims", model.Claims{UserID: 7, Username: "owner", Permissions: strings.Split(c.GetHeader("X-Permissions"), ",")})
			c.Next()
		},
	})
	return engine
}

func TestElementCaptureProductionRouterRegistersAllRoutesAndEnforcesBoundaries(t *testing.T) {
	engine := elementCaptureProductionRouter()
	wanted := map[string]bool{}
	for _, route := range engine.Routes() {
		wanted[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"POST /api/ui/page-elements/capture-sessions", "GET /api/ui/page-elements/capture-sessions/:id", "PATCH /api/ui/page-elements/capture-sessions/:id/mode", "POST /api/ui/page-elements/capture-sessions/:id/stop", "GET /api/ui/page-elements/capture-sessions/:id/candidates", "PATCH /api/ui/page-elements/capture-sessions/:id/candidates/:candidateId", "POST /api/ui/page-elements/capture-sessions/:id/save", "GET /api/ui/page-elements/:id/versions", "POST /api/ui/page-elements/:id/versions/:version/rollback", "POST /api/executor/element-capture/:id/heartbeat", "POST /api/executor/element-capture/:id/candidates", "POST /api/executor/element-capture/:id/fail", "GET /api/executor/element-capture/commands",
	} {
		if !wanted[route] {
			t.Fatalf("missing production route %s", route)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/api/ui/page-elements/capture-sessions", strings.NewReader(`{}`))
	request.Header.Set("X-Permissions", "ui.element.read")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("capture permission status=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/ui/page-elements/capture-sessions/session-1/candidates", nil)
	request.Header.Set("X-Permissions", "ui.element.capture")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("read permission status=%d cache=%q", response.Code, response.Header().Get("Cache-Control"))
	}
	request.Header.Set("X-Permissions", "ui.element.read")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("read allowed status=%d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/ui/page-elements/capture-sessions/session-1/save", strings.NewReader(`{"items":[{"candidateId":1,"resolution":"create"}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Permissions", "ui.element.manage")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"issues"`) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("save status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/ui/page-elements/1/versions/1/rollback", strings.NewReader(`{}`))
	request.Header.Set("X-Permissions", "ui.element.manage")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("rollback permission status=%d", response.Code)
	}
	request.Header.Set("X-Permissions", "ui.element.rollback")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("rollback allowed status=%d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/executor/element-capture/session-1/heartbeat", strings.NewReader(`{`))
	request.Header.Set("X-Executor-ID", "exec-1")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("executor authentication status=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/executor/element-capture/commands?executorId=exec-1", nil)
	request.Header.Set("X-Executor-Token", "long-token")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("command read status=%d", response.Code)
	}
}
