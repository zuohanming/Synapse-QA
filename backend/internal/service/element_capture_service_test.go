package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"synapseqa/backend/internal/model"

	"github.com/jackc/pgx/v5/pgconn"
)

type fakeCaptureRepo struct {
	activeExecutor string
	pageExists     bool
	created        model.ElementCaptureSession
	setMode        string
	setModeUpdated bool
	stopped        bool
	heartbeat      bool
	heartbeatHash  string
	expiredAt      time.Time
	detail         model.ElementCaptureSessionDetail
	createErr      error
}

type fakeExecutorReader struct {
	online        bool
	uiUnsupported bool
}

func (f *fakeCaptureRepo) HasActiveCapture(_ context.Context, executorID string) (bool, error) {
	return f.activeExecutor == executorID, nil
}

func (f *fakeCaptureRepo) PageExists(_ context.Context, _ int64) (bool, error) {
	return f.pageExists, nil
}

func (f *fakeCaptureRepo) CreateSession(_ context.Context, session model.ElementCaptureSession) error {
	f.created = session
	return f.createErr
}

func (f *fakeCaptureRepo) SetMode(_ context.Context, _, _ string, mode string) (bool, error) {
	f.setMode = mode
	return f.setModeUpdated, nil
}

func (f *fakeCaptureRepo) StopSession(_ context.Context, _, _ string) (bool, error) {
	return f.stopped, nil
}

func (f *fakeCaptureRepo) Heartbeat(_ context.Context, _, _, tokenHash, _ string) (bool, error) {
	f.heartbeatHash = tokenHash
	return f.heartbeat, nil
}

func (f *fakeCaptureRepo) ExpireSessions(_ context.Context, now time.Time) error {
	f.expiredAt = now
	return nil
}

func (f *fakeCaptureRepo) GetSession(_ context.Context, _ int64, _ string) (model.ElementCaptureSessionDetail, error) {
	return f.detail, nil
}

func (f fakeExecutorReader) GetByID(_ context.Context, executorID string) (model.ExecutorView, error) {
	status := "offline"
	if f.online {
		status = "online"
	}
	supportedTypes := []string{"ui"}
	if f.uiUnsupported {
		supportedTypes = []string{"api"}
	}
	return model.ExecutorView{ExecutorID: executorID, Status: status, SupportedTypes: supportedTypes}, nil
}

func TestElementCaptureCreateSessionRejectsMissingPage(t *testing.T) {
	service := NewElementCaptureService(&fakeCaptureRepo{}, fakeExecutorReader{online: true}, []byte("secret"))

	_, err := service.CreateSession(context.Background(), "admin", model.CaptureSessionCreateRequest{
		PageID: 8, ExecutorID: "exec-1", URL: "https://example.test", BrowserChannel: "chrome",
	})
	if err == nil || err.Error() != "页面不存在" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementCaptureCreateSessionReturnsOneTimeTokenAndPersistsOnlyHash(t *testing.T) {
	repo := &fakeCaptureRepo{pageExists: true}
	service := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))
	service.now = func() time.Time { return time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC) }

	created, err := service.CreateSession(context.Background(), "admin", model.CaptureSessionCreateRequest{
		PageID: 8, ExecutorID: "exec-1", BrowserContextID: "context-1", URL: "https://example.test", BrowserChannel: "chrome",
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if len(created.Token) < 43 {
		t.Fatalf("token is too short: %q", created.Token)
	}
	hash := sha256.Sum256([]byte(created.Token))
	if repo.created.TokenHash != hex.EncodeToString(hash[:]) {
		t.Fatalf("persisted token hash does not match returned token")
	}
	if repo.created.Status != CaptureStarting || repo.created.Mode != "pick" || repo.created.ExpiresAt.Sub(service.now()) != 30*time.Minute {
		t.Fatalf("unexpected persisted session: %+v", repo.created)
	}
}

func TestElementCaptureCreateSessionMapsExecutorSessionRace(t *testing.T) {
	repo := &fakeCaptureRepo{pageExists: true, createErr: &pgconn.PgError{Code: "23505"}}
	service := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))

	_, err := service.CreateSession(context.Background(), "admin", model.CaptureSessionCreateRequest{
		PageID: 8, ExecutorID: "exec-1", URL: "https://example.test", BrowserChannel: "chrome",
	})
	if err == nil || err.Error() != "执行器正在采集页面元素" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementCaptureSetModeRejectsUnsupportedMode(t *testing.T) {
	service := NewElementCaptureService(&fakeCaptureRepo{}, fakeExecutorReader{online: true}, []byte("secret"))

	err := service.SetMode(context.Background(), "admin", "session-1", "record")
	if err == nil || err.Error() != "仅支持 pick 或 operate 模式" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementCaptureGetSessionReturnsOwnerScopedDetail(t *testing.T) {
	repo := &fakeCaptureRepo{detail: model.ElementCaptureSessionDetail{ElementCaptureSession: model.ElementCaptureSession{ID: "session-1", PageID: 8}}}
	service := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))

	detail, err := service.GetSession(context.Background(), 7, "session-1")
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if detail.ID != "session-1" || detail.PageID != 8 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestElementCaptureSetModeUpdatesPickOrOperate(t *testing.T) {
	repo := &fakeCaptureRepo{setModeUpdated: true}
	service := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))

	if err := service.SetMode(context.Background(), "admin", "session-1", "operate"); err != nil {
		t.Fatalf("SetMode returned error: %v", err)
	}
	if repo.setMode != "operate" {
		t.Fatalf("unexpected persisted mode: %q", repo.setMode)
	}
}

func TestElementCaptureHeartbeatRejectsWrongExecutorOrToken(t *testing.T) {
	service := NewElementCaptureService(&fakeCaptureRepo{}, fakeExecutorReader{online: true}, []byte("secret"))

	err := service.Heartbeat(context.Background(), "session-1", "wrong-executor", "wrong-token", "https://example.test")
	if err == nil || err.Error() != "采集会话不存在、执行器或令牌无效，或恢复窗口已过期" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementCaptureHeartbeatHashesOneTimeToken(t *testing.T) {
	repo := &fakeCaptureRepo{heartbeat: true}
	service := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))

	if err := service.Heartbeat(context.Background(), "session-1", "exec-1", "token-1", "https://example.test/path"); err != nil {
		t.Fatalf("Heartbeat returned error: %v", err)
	}
	hash := sha256.Sum256([]byte("token-1"))
	if repo.heartbeatHash != hex.EncodeToString(hash[:]) {
		t.Fatalf("unexpected token hash: %q", repo.heartbeatHash)
	}
}

func TestElementCaptureStopSessionIsIdempotentForTerminalSession(t *testing.T) {
	service := NewElementCaptureService(&fakeCaptureRepo{}, fakeExecutorReader{online: true}, []byte("secret"))

	if err := service.StopSession(context.Background(), "admin", "completed-session"); err != nil {
		t.Fatalf("StopSession returned error: %v", err)
	}
}

func TestElementCaptureExpireSessionsDelegatesConfiguredTime(t *testing.T) {
	repo := &fakeCaptureRepo{}
	service := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)

	if err := service.ExpireSessions(context.Background(), now); err != nil {
		t.Fatalf("ExpireSessions returned error: %v", err)
	}
	if !repo.expiredAt.Equal(now) {
		t.Fatalf("unexpected expiry time: %v", repo.expiredAt)
	}
}

func TestElementCaptureCreateSessionRejectsBusyExecutor(t *testing.T) {
	repo := &fakeCaptureRepo{activeExecutor: "exec-1", pageExists: true}
	service := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))

	_, err := service.CreateSession(context.Background(), "admin", model.CaptureSessionCreateRequest{
		PageID: 8, ExecutorID: "exec-1", URL: "https://example.test", BrowserChannel: "chrome",
	})
	if err == nil || err.Error() != "执行器正在采集页面元素" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementCaptureCreateSessionValidatesExecutorAndRequest(t *testing.T) {
	tests := []struct {
		name     string
		repo     *fakeCaptureRepo
		executor fakeExecutorReader
		req      model.CaptureSessionCreateRequest
		wantErr  string
	}{
		{
			name:     "rejects executor without UI support",
			repo:     &fakeCaptureRepo{pageExists: true},
			executor: fakeExecutorReader{online: true, uiUnsupported: true},
			req:      model.CaptureSessionCreateRequest{PageID: 8, ExecutorID: "exec-1", URL: "https://example.test", BrowserChannel: "chrome"},
			wantErr:  "执行器不支持 UI",
		},
		{
			name:     "rejects relative URL",
			repo:     &fakeCaptureRepo{pageExists: true},
			executor: fakeExecutorReader{online: true},
			req:      model.CaptureSessionCreateRequest{PageID: 8, ExecutorID: "exec-1", URL: "/login", BrowserChannel: "chrome"},
			wantErr:  "页面地址必须是绝对 http/https URL",
		},
		{
			name:     "rejects unsupported browser channel",
			repo:     &fakeCaptureRepo{pageExists: true},
			executor: fakeExecutorReader{online: true},
			req:      model.CaptureSessionCreateRequest{PageID: 8, ExecutorID: "exec-1", URL: "https://example.test", BrowserChannel: "firefox"},
			wantErr:  "仅支持 Chrome 或 Edge 浏览器",
		},
		{
			name:     "rejects unsupported mode",
			repo:     &fakeCaptureRepo{pageExists: true},
			executor: fakeExecutorReader{online: true},
			req:      model.CaptureSessionCreateRequest{PageID: 8, ExecutorID: "exec-1", URL: "https://example.test", BrowserChannel: "chrome", Mode: "record"},
			wantErr:  "仅支持 pick 或 operate 模式",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewElementCaptureService(tt.repo, tt.executor, []byte("secret"))
			_, err := service.CreateSession(context.Background(), "admin", tt.req)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestElementCaptureCreateSessionRejectsOfflineExecutor(t *testing.T) {
	service := NewElementCaptureService(&fakeCaptureRepo{pageExists: true}, fakeExecutorReader{}, []byte("secret"))

	_, err := service.CreateSession(context.Background(), "admin", model.CaptureSessionCreateRequest{
		PageID: 8, ExecutorID: "exec-1", URL: "https://example.test", BrowserChannel: "chrome",
	})
	if err == nil || err.Error() != "执行器不在线" {
		t.Fatalf("unexpected error: %v", err)
	}
}
