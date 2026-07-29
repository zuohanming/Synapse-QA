package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"synapseqa/backend/internal/model"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestBatchSaveRejectsUnreliableCandidate(t *testing.T) {
	service := newCaptureServiceWithCandidates(candidateFixture{
		Name: "未命名元素", Locators: nil, QualityScore: 20,
	})

	_, err := service.BatchSave(context.Background(), "admin", model.CandidateBatchSaveRequest{
		SessionID: "session-1",
		Items:     []model.CandidateSaveItem{{CandidateID: 1, Resolution: "create"}},
	})
	if err == nil || !strings.Contains(err.Error(), "候选项 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBatchSaveAllowsIgnoringUnreliableCandidate(t *testing.T) {
	service := newCaptureServiceWithCandidates(candidateFixture{Name: "未命名元素", Locators: nil, QualityScore: 0})
	_, err := service.BatchSave(context.Background(), "admin", model.CandidateBatchSaveRequest{
		SessionID: "session-1",
		Items:     []model.CandidateSaveItem{{CandidateID: 1, Resolution: "ignore"}},
	})
	if err != nil {
		t.Fatalf("ignore should bypass quality gates: %v", err)
	}
}

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
	authorized     bool
	authorizedID   string
	authorizedExec string
	authorizedHash string
	commands       []model.ElementCaptureCommand
}

type fakeExecutorReader struct {
	online        bool
	uiUnsupported bool
}

type candidateFixture struct {
	ID           string
	CursorID     int64
	Name         string
	Locators     []byte
	QualityScore float64
	Fingerprint  string
	Conflict     string
	DuplicateID  int64
}

type candidateCaptureRepo struct {
	candidates []model.ElementCaptureCandidate
	session    model.ElementCaptureSession
	saved      bool
}

var _ CandidateCaptureRepository = (*candidateCaptureRepo)(nil)

func newCaptureServiceWithCandidates(fixtures ...candidateFixture) *ElementCaptureService {
	repo := &candidateCaptureRepo{session: model.ElementCaptureSession{ID: "session-1", PageID: 8, Status: CaptureActive}}
	for index, fixture := range fixtures {
		id := fixture.ID
		if id == "" {
			id = fmt.Sprintf("candidate-%d", index+1)
		}
		cursorID := fixture.CursorID
		if cursorID == 0 {
			cursorID = int64(index + 1)
		}
		repo.candidates = append(repo.candidates, model.ElementCaptureCandidate{
			ID: id, CursorID: cursorID, SessionID: "session-1", Name: fixture.Name, Locators: fixture.Locators,
			QualityScore: fixture.QualityScore, Fingerprint: fixture.Fingerprint,
			ConflictStatus: fixture.Conflict, DuplicateElementID: fixture.DuplicateID, Status: "pending",
		})
	}
	return NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))
}

func (r *candidateCaptureRepo) PageExists(_ context.Context, _ int64) (bool, error) { return true, nil }
func (r *candidateCaptureRepo) HasActiveCapture(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *candidateCaptureRepo) CreateSession(_ context.Context, session model.ElementCaptureSession) error {
	r.session = session
	return nil
}
func (r *candidateCaptureRepo) GetSession(_ context.Context, _ int64, _ string) (model.ElementCaptureSessionDetail, error) {
	return model.ElementCaptureSessionDetail{ElementCaptureSession: r.session}, nil
}
func (r *candidateCaptureRepo) SetMode(_ context.Context, _, _ string, mode string) (bool, error) {
	r.session.Mode = mode
	return true, nil
}
func (r *candidateCaptureRepo) StopSession(_ context.Context, _, _ string) (bool, error) {
	r.session.Status = CaptureCompleted
	return true, nil
}
func (r *candidateCaptureRepo) Heartbeat(_ context.Context, _, _, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *candidateCaptureRepo) ExpireSessions(_ context.Context, _ time.Time) error { return nil }

func (r *candidateCaptureRepo) AddCandidate(_ context.Context, candidate model.ElementCaptureCandidate, _, _ string) (model.ElementCaptureCandidate, error) {
	candidate.ID = fmt.Sprintf("candidate-%d", len(r.candidates)+1)
	candidate.CursorID = int64(len(r.candidates) + 1)
	r.candidates = append(r.candidates, candidate)
	return candidate, nil
}

func (r *candidateCaptureRepo) ListCandidates(_ context.Context, _ int64, _ string, _ int64, _ int) ([]model.ElementCaptureCandidate, error) {
	return r.candidates, nil
}

func (r *candidateCaptureRepo) UpdateCandidate(_ context.Context, _, _ string, _ int64, _ model.CaptureCandidateUpdateRequest) (bool, error) {
	return true, nil
}

func (r *candidateCaptureRepo) GetBatchSaveData(_ context.Context, _, _ string, _ []int64) (model.CaptureBatchData, error) {
	return model.CaptureBatchData{Session: r.session, Candidates: r.candidates}, nil
}

func (r *candidateCaptureRepo) SaveCandidates(_ context.Context, _ string, _ model.CandidateBatchSaveRequest, validate func(model.CaptureBatchData) error) (model.BatchSaveResult, error) {
	if err := validate(model.CaptureBatchData{Session: r.session, Candidates: r.candidates}); err != nil {
		return model.BatchSaveResult{}, err
	}
	r.saved = true
	return model.BatchSaveResult{}, nil
}

type statefulCaptureRepo struct {
	session model.ElementCaptureSession
	now     time.Time
}

func (r *statefulCaptureRepo) PageExists(_ context.Context, _ int64) (bool, error) {
	return true, nil
}

func (r *statefulCaptureRepo) HasActiveCapture(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (r *statefulCaptureRepo) CreateSession(_ context.Context, session model.ElementCaptureSession) error {
	r.session = session
	return nil
}

func (r *statefulCaptureRepo) GetSession(_ context.Context, _ int64, _ string) (model.ElementCaptureSessionDetail, error) {
	return model.ElementCaptureSessionDetail{ElementCaptureSession: r.session}, nil
}

func (r *statefulCaptureRepo) SetMode(_ context.Context, _, _ string, mode string) (bool, error) {
	r.session.Mode = mode
	return true, nil
}

func (r *statefulCaptureRepo) StopSession(_ context.Context, _, _ string) (bool, error) {
	if r.session.Status == CaptureCompleted || r.session.Status == CaptureExpired {
		return false, nil
	}
	r.session.Status = CaptureCompleted
	return true, nil
}

func (r *statefulCaptureRepo) Heartbeat(_ context.Context, sessionID, executorID, tokenHash, browserContextID, currentURL string) (bool, error) {
	if r.session.ID != sessionID || r.session.ExecutorID != executorID || r.session.TokenHash != tokenHash || !r.now.Before(r.session.ExpiresAt) {
		return false, nil
	}
	if r.session.Status == CaptureInterrupted && (r.session.RecoveryExpiresAt == nil || r.now.After(*r.session.RecoveryExpiresAt)) {
		return false, nil
	}
	if r.session.Status == CaptureStarting {
		if r.session.BrowserContextID != "" || browserContextID == "" {
			return false, nil
		}
		r.session.BrowserContextID = browserContextID
	} else if (r.session.Status != CaptureActive && r.session.Status != CaptureInterrupted) || r.session.BrowserContextID != browserContextID {
		return false, nil
	}
	r.session.Status = CaptureActive
	r.session.CurrentURL = currentURL
	r.session.LastHeartbeatAt = r.now
	r.session.InterruptedAt = nil
	r.session.RecoveryExpiresAt = nil
	return true, nil
}

func (r *statefulCaptureRepo) ExpireSessions(_ context.Context, now time.Time) error {
	r.now = now
	if r.session.Status == CaptureStarting || r.session.Status == CaptureActive || r.session.Status == CaptureInterrupted {
		if !now.Before(r.session.ExpiresAt) {
			r.session.Status = CaptureExpired
			return nil
		}
	}
	if r.session.Status == CaptureActive && now.Sub(r.session.LastHeartbeatAt) > 60*time.Second {
		r.session.Status = CaptureInterrupted
		r.session.InterruptedAt = &now
		recovery := now.Add(60 * time.Second)
		r.session.RecoveryExpiresAt = &recovery
		return nil
	}
	if r.session.Status == CaptureInterrupted && r.session.RecoveryExpiresAt != nil && !now.Before(*r.session.RecoveryExpiresAt) {
		r.session.Status = CaptureExpired
	}
	return nil
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

func (f *fakeCaptureRepo) PageAccessible(_ context.Context, _ int64, _ string) (bool, error) {
	return f.pageExists, nil
}

func (f *fakeCaptureRepo) CreateSessionWithStartCommand(ctx context.Context, _ string, session model.ElementCaptureSession, command model.ElementCaptureCommand) error {
	if err := f.CreateSession(ctx, session); err != nil {
		return err
	}
	f.commands = append(f.commands, command)
	return nil
}

func (f *fakeCaptureRepo) SetModeWithCommand(ctx context.Context, actor, sessionID, mode string) (bool, error) {
	updated, err := f.SetMode(ctx, actor, sessionID, mode)
	if updated && err == nil {
		f.commands = append(f.commands, model.ElementCaptureCommand{SessionID: sessionID, Type: "set_mode", Mode: mode})
	}
	return updated, err
}

func (f *fakeCaptureRepo) StopSessionWithCommand(ctx context.Context, actor, sessionID string) (bool, error) {
	updated, err := f.StopSession(ctx, actor, sessionID)
	if updated && err == nil {
		f.commands = append(f.commands, model.ElementCaptureCommand{SessionID: sessionID, Type: "stop"})
	}
	return updated, err
}

func (f *fakeCaptureRepo) AuthorizeCommandExecutor(_ context.Context, executorID, token string) (bool, error) {
	f.authorizedExec = executorID
	return f.authorized && token == "long-token", nil
}

func (f *fakeCaptureRepo) ClaimCommands(_ context.Context, executorID string, _ int) ([]model.ElementCaptureCommand, error) {
	if executorID != "exec-1" {
		return nil, nil
	}
	items := f.commands
	f.commands = nil
	return items, nil
}

func (f *fakeCaptureRepo) SetMode(_ context.Context, _, _ string, mode string) (bool, error) {
	f.setMode = mode
	return f.setModeUpdated, nil
}

func (f *fakeCaptureRepo) StopSession(_ context.Context, _, _ string) (bool, error) {
	if !f.stopped {
		return false, model.NewDomainError(model.ErrConflict, "采集会话已结束")
	}
	return true, nil
}

func (f *fakeCaptureRepo) Heartbeat(_ context.Context, _, _, tokenHash, _, _ string) (bool, error) {
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

func (f *fakeCaptureRepo) AuthorizeExecutor(_ context.Context, sessionID, executorID, tokenHash string) (bool, error) {
	f.authorizedID, f.authorizedExec, f.authorizedHash = sessionID, executorID, tokenHash
	return f.authorized, nil
}

func (f *fakeCaptureRepo) FailSession(_ context.Context, _, _, _, _ string) (bool, error) {
	return f.authorized, nil
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
	if repo.created.BrowserContextID != "" {
		t.Fatalf("starting session must wait for first heartbeat context binding: %+v", repo.created)
	}
}

func TestElementCaptureCreateSessionRejectsInvalidFinalCurrentURL(t *testing.T) {
	service := NewElementCaptureService(&fakeCaptureRepo{pageExists: true}, fakeExecutorReader{online: true}, []byte("secret"))

	_, err := service.CreateSession(context.Background(), "admin", model.CaptureSessionCreateRequest{
		PageID: 8, ExecutorID: "exec-1", URL: "https://example.test", CurrentURL: "javascript:alert(1)", BrowserChannel: "chrome",
	})
	if err == nil || err.Error() != "页面地址必须是绝对 http/https URL" {
		t.Fatalf("unexpected error: %v", err)
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

	err := service.Heartbeat(context.Background(), "session-1", "wrong-executor", "wrong-token", "context-1", "https://example.test")
	if !errors.Is(err, model.ErrUnauthorized) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementCaptureHeartbeatHashesOneTimeToken(t *testing.T) {
	repo := &fakeCaptureRepo{heartbeat: true}
	service := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))

	if err := service.Heartbeat(context.Background(), "session-1", "exec-1", "token-1", "context-1", "https://example.test/path"); err != nil {
		t.Fatalf("Heartbeat returned error: %v", err)
	}
	hash := sha256.Sum256([]byte("token-1"))
	if repo.heartbeatHash != hex.EncodeToString(hash[:]) {
		t.Fatalf("unexpected token hash: %q", repo.heartbeatHash)
	}
}

func TestElementCaptureHeartbeatRejectsInvalidCurrentURL(t *testing.T) {
	service := NewElementCaptureService(&fakeCaptureRepo{heartbeat: true}, fakeExecutorReader{online: true}, []byte("secret"))

	err := service.Heartbeat(context.Background(), "session-1", "exec-1", "token-1", "context-1", "file:///secret")
	if err == nil || err.Error() != "页面地址必须是绝对 http/https URL" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementCaptureHeartbeatRequiresBrowserContextOnFirstActivation(t *testing.T) {
	service := NewElementCaptureService(&fakeCaptureRepo{heartbeat: true}, fakeExecutorReader{online: true}, []byte("secret"))

	err := service.Heartbeat(context.Background(), "session-1", "exec-1", "token-1", "", "https://example.test")
	if !errors.Is(err, model.ErrConflict) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestElementCaptureStateTransitionsBindAndRecoverBrowserContext(t *testing.T) {
	base := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	tokenHash := sha256.Sum256([]byte("token-1"))
	repo := &statefulCaptureRepo{now: base, session: model.ElementCaptureSession{
		ID: "session-1", ExecutorID: "exec-1", TokenHash: hex.EncodeToString(tokenHash[:]), Status: CaptureStarting,
		ExpiresAt: base.Add(30 * time.Minute), LastHeartbeatAt: base,
	}}
	service := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))

	if err := service.Heartbeat(context.Background(), "session-1", "exec-1", "token-1", "context-1", "https://example.test"); err != nil {
		t.Fatalf("first Heartbeat returned error: %v", err)
	}
	if repo.session.Status != CaptureActive || repo.session.BrowserContextID != "context-1" {
		t.Fatalf("starting session was not activated and bound: %+v", repo.session)
	}

	idleAt := base.Add(61 * time.Second)
	if err := service.ExpireSessions(context.Background(), idleAt); err != nil {
		t.Fatalf("ExpireSessions returned error: %v", err)
	}
	if repo.session.Status != CaptureInterrupted || repo.session.RecoveryExpiresAt == nil || !repo.session.RecoveryExpiresAt.Equal(idleAt.Add(60*time.Second)) {
		t.Fatalf("active session was not interrupted with 60 second recovery: %+v", repo.session)
	}
	if err := service.Heartbeat(context.Background(), "session-1", "other-executor", "token-1", "context-1", "https://example.test"); err == nil {
		t.Fatal("expected original executor enforcement")
	}
	if err := service.Heartbeat(context.Background(), "session-1", "exec-1", "token-1", "other-context", "https://example.test"); err == nil {
		t.Fatal("expected browser context immutability")
	}
	if err := service.Heartbeat(context.Background(), "session-1", "exec-1", "token-1", "context-1", "https://example.test/recovered"); err != nil {
		t.Fatalf("original executor did not recover session: %v", err)
	}
	if repo.session.Status != CaptureActive || repo.session.RecoveryExpiresAt != nil {
		t.Fatalf("session did not recover: %+v", repo.session)
	}

	repo.session.LastHeartbeatAt = idleAt
	if err := service.ExpireSessions(context.Background(), idleAt.Add(61*time.Second)); err != nil {
		t.Fatalf("second interruption returned error: %v", err)
	}
	expiredAt := idleAt.Add(121 * time.Second)
	if err := service.ExpireSessions(context.Background(), expiredAt); err != nil {
		t.Fatalf("recovery expiry returned error: %v", err)
	}
	if repo.session.Status != CaptureExpired {
		t.Fatalf("session did not expire after recovery window: %+v", repo.session)
	}

	expiringRepo := &statefulCaptureRepo{now: base, session: model.ElementCaptureSession{Status: CaptureStarting, ExpiresAt: base.Add(30 * time.Minute)}}
	if err := NewElementCaptureService(expiringRepo, fakeExecutorReader{online: true}, []byte("secret")).ExpireSessions(context.Background(), base.Add(30*time.Minute)); err != nil {
		t.Fatalf("30 minute expiry returned error: %v", err)
	}
	if expiringRepo.session.Status != CaptureExpired {
		t.Fatalf("session did not expire after 30 minutes: %+v", expiringRepo.session)
	}
}

func TestElementCaptureStopSessionRejectsInvisibleSession(t *testing.T) {
	service := NewElementCaptureService(&fakeCaptureRepo{}, fakeExecutorReader{online: true}, []byte("secret"))
	if err := service.StopSession(context.Background(), "admin", "completed-session"); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("StopSession error=%v", err)
	}
}

func TestElementCaptureQueuesConsumableCommandsForAuthorizedExecutor(t *testing.T) {
	repo := &fakeCaptureRepo{pageExists: true, setModeUpdated: true, stopped: true, authorized: true}
	svc := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))
	created, err := svc.CreateSession(context.Background(), "admin", model.CaptureSessionCreateRequest{PageID: 8, ExecutorID: "exec-1", URL: "https://example.test", BrowserChannel: "chrome"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err = svc.SetMode(context.Background(), "admin", created.Session.ID, "operate"); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if err = svc.StopSession(context.Background(), "admin", created.Session.ID); err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	commands, err := svc.ListCommands(context.Background(), "exec-1", "", "long-token")
	if err != nil {
		t.Fatalf("ListCommands: %v", err)
	}
	if len(commands) != 3 || commands[0].Type != "start" || commands[1].Type != "set_mode" || commands[2].Type != "stop" {
		t.Fatalf("commands=%+v", commands)
	}
	if repo.authorizedExec != "exec-1" {
		t.Fatalf("authorization=%q", repo.authorizedExec)
	}
	commands, err = svc.ListCommands(context.Background(), "exec-1", "", "long-token")
	if err != nil || len(commands) != 0 {
		t.Fatalf("commands should be consumed: %+v err=%v", commands, err)
	}
}

func TestElementCaptureRejectsUnauthorizedCommandRead(t *testing.T) {
	svc := NewElementCaptureService(&fakeCaptureRepo{}, fakeExecutorReader{online: true}, []byte("secret"))
	_, err := svc.ListCommands(context.Background(), "exec-1", "", "bad")
	if !errors.Is(err, model.ErrUnauthorized) {
		t.Fatalf("err=%v", err)
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

func TestBatchSaveRejectsUnresolvedDuplicateAndMissingUpdateTarget(t *testing.T) {
	service := newCaptureServiceWithCandidates(
		candidateFixture{CursorID: 1, Name: "submit", Locators: []byte(`[{"type":"testid","value":"submit","score":95,"unique":true}]`), QualityScore: 95, Conflict: "duplicate"},
		candidateFixture{CursorID: 2, Name: "cancel", Locators: []byte(`[{"type":"testid","value":"cancel","score":95,"unique":true}]`), QualityScore: 95, Conflict: "duplicate"},
	)
	_, err := service.BatchSave(context.Background(), "admin", model.CandidateBatchSaveRequest{SessionID: "session-1", Items: []model.CandidateSaveItem{
		{CandidateID: 1, Resolution: ""}, {CandidateID: 2, Resolution: "update"},
	}})
	if err == nil || !strings.Contains(err.Error(), "候选项 1") || !strings.Contains(err.Error(), "候选项 2") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddCandidateRejectsInvalidFingerprint(t *testing.T) {
	service := newCaptureServiceWithCandidates()
	_, err := service.AddCandidate(context.Background(), "exec-1", "token", model.CaptureCandidateCreateRequest{
		SessionID: "session-1", Name: "submit", Fingerprint: "ABC", CaptureURL: "https://example.test",
		Locators: []byte(`[{"type":"testid","value":"submit","score":95,"unique":true}]`), QualityScore: 95,
	})
	if err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBatchSaveRequiresExplicitTargetForMultipleFingerprintMatches(t *testing.T) {
	data := model.CaptureBatchData{
		Session:              model.ElementCaptureSession{PageID: 8, Status: CaptureActive},
		Candidates:           []model.ElementCaptureCandidate{{CursorID: 1, ID: "candidate-1", Name: "save", Fingerprint: "fp", Status: "pending", QualityScore: 95, Locators: []byte(`[{"type":"testid","value":"save","score":95,"unique":true}]`)}},
		ExistingFingerprints: map[string][]int64{"fp": {11, 12}},
	}
	issues := validateBatchSave(data, model.CandidateBatchSaveRequest{Items: []model.CandidateSaveItem{{CandidateID: 1, Resolution: "update"}}})
	if len(issues) == 0 || !strings.Contains(issues[0].Message, "请选择目标") {
		t.Fatalf("expected ambiguous target issue, got %+v", issues)
	}
}
