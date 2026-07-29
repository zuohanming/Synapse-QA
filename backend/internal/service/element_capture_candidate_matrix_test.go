package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"synapseqa/backend/internal/model"
)

const (
	testFingerprintA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testFingerprintB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var reliableTestLocators = []byte(`[{"type":"testid","value":"submit","score":95,"unique":true}]`)

type candidateIssuesCarrier interface {
	CandidateIssues() []model.CandidateIssue
}

type captureCandidateTestRepo struct {
	session              model.ElementCaptureSession
	candidates           []model.ElementCaptureCandidate
	lockedCandidates     []model.ElementCaptureCandidate
	existingNames        map[string][]int64
	existingFingerprints map[string][]int64

	addCalls      int
	addExecutorID string
	addTokenHash  string

	listCalls     int
	listUserID    int64
	listSessionID string
	listAfterID   int64
	listLimit     int

	updateCalls       int
	updateActor       string
	updateSessionID   string
	updateCandidateID int64
	updateRequest     model.CaptureCandidateUpdateRequest

	batchDataCalls int
	saveCalls      int
	saveSucceeded  bool
}

var _ ElementCaptureRepository = (*captureCandidateTestRepo)(nil)
var _ CandidateCaptureRepository = (*captureCandidateTestRepo)(nil)

func newCaptureCandidateTestService(candidates ...model.ElementCaptureCandidate) (*ElementCaptureService, *captureCandidateTestRepo) {
	repo := &captureCandidateTestRepo{
		session:              model.ElementCaptureSession{ID: "session-1", PageID: 8, Status: CaptureActive},
		candidates:           candidates,
		existingNames:        map[string][]int64{},
		existingFingerprints: map[string][]int64{},
	}
	return NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret")), repo
}

func validCaptureCandidate(cursorID int64, name, fingerprint string) model.ElementCaptureCandidate {
	return model.ElementCaptureCandidate{
		ID:           "candidate-" + name,
		CursorID:     cursorID,
		SessionID:    "session-1",
		Name:         name,
		Fingerprint:  fingerprint,
		CaptureURL:   "https://example.test",
		Locators:     reliableTestLocators,
		QualityScore: 95,
		Status:       "pending",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
}

func requireCandidateIssues(t *testing.T, err error) []model.CandidateIssue {
	t.Helper()
	if err == nil {
		t.Fatal("期望返回候选项问题")
	}
	var carrier candidateIssuesCarrier
	if !errors.As(err, &carrier) {
		t.Fatalf("期望结构化 CandidateIssue，实际错误类型为 %T：%v", err, err)
	}
	issues := carrier.CandidateIssues()
	if len(issues) == 0 {
		t.Fatal("结构化 CandidateIssue 不能为空")
	}
	return issues
}

func requireIssueField(t *testing.T, issues []model.CandidateIssue, field string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Field == field {
			return
		}
	}
	t.Fatalf("未找到字段 %q 的问题：%+v", field, issues)
}

func (r *captureCandidateTestRepo) PageExists(context.Context, int64) (bool, error) {
	return true, nil
}

func (r *captureCandidateTestRepo) HasActiveCapture(context.Context, string) (bool, error) {
	return false, nil
}

func (r *captureCandidateTestRepo) CreateSession(_ context.Context, session model.ElementCaptureSession) error {
	r.session = session
	return nil
}

func (r *captureCandidateTestRepo) GetSession(context.Context, int64, string) (model.ElementCaptureSessionDetail, error) {
	return model.ElementCaptureSessionDetail{ElementCaptureSession: r.session}, nil
}

func (r *captureCandidateTestRepo) SetMode(context.Context, string, string, string) (bool, error) {
	return true, nil
}

func (r *captureCandidateTestRepo) StopSession(context.Context, string, string) (bool, error) {
	return true, nil
}

func (r *captureCandidateTestRepo) Heartbeat(context.Context, string, string, string, string, string) (bool, error) {
	return true, nil
}

func (r *captureCandidateTestRepo) ExpireSessions(context.Context, time.Time) error {
	return nil
}

func (r *captureCandidateTestRepo) AddCandidate(_ context.Context, candidate model.ElementCaptureCandidate, executorID, tokenHash string) (model.ElementCaptureCandidate, error) {
	r.addCalls++
	r.addExecutorID = executorID
	r.addTokenHash = tokenHash
	return candidate, nil
}

func (r *captureCandidateTestRepo) ListCandidates(_ context.Context, userID int64, sessionID string, afterID int64, limit int) ([]model.ElementCaptureCandidate, error) {
	r.listCalls++
	r.listUserID = userID
	r.listSessionID = sessionID
	r.listAfterID = afterID
	r.listLimit = limit
	return r.candidates, nil
}

func (r *captureCandidateTestRepo) UpdateCandidate(_ context.Context, actor, sessionID string, candidateID int64, req model.CaptureCandidateUpdateRequest) (bool, error) {
	r.updateCalls++
	r.updateActor = actor
	r.updateSessionID = sessionID
	r.updateCandidateID = candidateID
	r.updateRequest = req
	return true, nil
}

func (r *captureCandidateTestRepo) GetBatchSaveData(context.Context, string, []int64) (model.CaptureBatchData, error) {
	r.batchDataCalls++
	return r.batchData(r.candidates), nil
}

func (r *captureCandidateTestRepo) SaveCandidates(_ context.Context, _ string, _ model.CandidateBatchSaveRequest, validate func(model.CaptureBatchData) error) (model.BatchSaveResult, error) {
	r.saveCalls++
	candidates := r.lockedCandidates
	if candidates == nil {
		candidates = r.candidates
	}
	if err := validate(r.batchData(candidates)); err != nil {
		return model.BatchSaveResult{}, err
	}
	r.saveSucceeded = true
	return model.BatchSaveResult{}, nil
}

func (r *captureCandidateTestRepo) batchData(candidates []model.ElementCaptureCandidate) model.CaptureBatchData {
	return model.CaptureBatchData{
		Session:              r.session,
		Candidates:           candidates,
		ExistingNames:        r.existingNames,
		ExistingFingerprints: r.existingFingerprints,
	}
}

func TestAddCandidatePassesTokenHashToProductionRepositoryBoundary(t *testing.T) {
	service, repo := newCaptureCandidateTestService()
	_, err := service.AddCandidate(context.Background(), "executor-1", "one-time-token", model.CaptureCandidateCreateRequest{
		SessionID:    "session-1",
		Name:         "submit",
		Fingerprint:  testFingerprintA,
		CaptureURL:   "https://example.test/login",
		Locators:     reliableTestLocators,
		QualityScore: 95,
	})
	if err != nil {
		t.Fatalf("AddCandidate 返回错误：%v", err)
	}
	sum := sha256.Sum256([]byte("one-time-token"))
	if repo.addCalls != 1 || repo.addExecutorID != "executor-1" || repo.addTokenHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("令牌或执行器未正确传递：calls=%d executor=%q hash=%q", repo.addCalls, repo.addExecutorID, repo.addTokenHash)
	}
}

func TestListCandidatesCursorAndLimitMatrix(t *testing.T) {
	tests := []struct {
		name      string
		afterID   int64
		limit     int
		wantErr   string
		wantLimit int
	}{
		{name: "reject negative cursor", afterID: -1, limit: 10, wantErr: "afterID"},
		{name: "default limit", afterID: 4, limit: 0, wantLimit: 100},
		{name: "accept maximum limit", afterID: 4, limit: 200, wantLimit: 200},
		{name: "reject negative limit", afterID: 4, limit: -1, wantErr: "limit"},
		{name: "reject over maximum limit", afterID: 4, limit: 201, wantErr: "limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, repo := newCaptureCandidateTestService()
			_, err := service.ListCandidates(context.Background(), 7, "session-1", tt.afterID, tt.limit)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("错误不符合预期：%v", err)
				}
				if repo.listCalls != 0 {
					t.Fatalf("非法参数不应访问仓储，实际调用 %d 次", repo.listCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListCandidates 返回错误：%v", err)
			}
			if repo.listCalls != 1 || repo.listUserID != 7 || repo.listSessionID != "session-1" || repo.listAfterID != tt.afterID || repo.listLimit != tt.wantLimit {
				t.Fatalf("分页/可见性参数错误：%+v", repo)
			}
		})
	}
}

func TestUpdateCandidateEditableFieldBoundaryMatrix(t *testing.T) {
	negativeScore := -1.0
	tests := []struct {
		name        string
		actor       string
		sessionID   string
		candidateID int64
		req         model.CaptureCandidateUpdateRequest
		wantErr     string
	}{
		{name: "reject empty actor", actor: " ", sessionID: "session-1", candidateID: 1, wantErr: "参数无效"},
		{name: "reject empty session", actor: "admin", sessionID: " ", candidateID: 1, wantErr: "参数无效"},
		{name: "reject invalid candidate cursor", actor: "admin", sessionID: "session-1", candidateID: 0, wantErr: "参数无效"},
		{name: "reject whitespace name", actor: "admin", sessionID: "session-1", candidateID: 1, req: model.CaptureCandidateUpdateRequest{Name: " "}, wantErr: "名称不能为空"},
		{name: "reject malformed locators", actor: "admin", sessionID: "session-1", candidateID: 1, req: model.CaptureCandidateUpdateRequest{Locators: []byte(`{}`)}, wantErr: "locators"},
		{name: "reject negative quality", actor: "admin", sessionID: "session-1", candidateID: 1, req: model.CaptureCandidateUpdateRequest{QualityScore: &negativeScore}, wantErr: "质量评分"},
		{name: "reject unsupported resolution", actor: "admin", sessionID: "session-1", candidateID: 1, req: model.CaptureCandidateUpdateRequest{ConflictResolution: "overwrite"}, wantErr: "冲突处理"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, repo := newCaptureCandidateTestService()
			err := service.UpdateCandidate(context.Background(), tt.actor, tt.sessionID, tt.candidateID, tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("错误不符合预期：%v", err)
			}
			if repo.updateCalls != 0 {
				t.Fatalf("非法更新不应访问仓储，实际调用 %d 次", repo.updateCalls)
			}
		})
	}

	score := 88.0
	service, repo := newCaptureCandidateTestService()
	req := model.CaptureCandidateUpdateRequest{
		Name:               "renamed",
		Locators:           reliableTestLocators,
		QualityScore:       &score,
		ConflictResolution: "update",
	}
	if err := service.UpdateCandidate(context.Background(), "admin", "session-1", 9, req); err != nil {
		t.Fatalf("合法更新返回错误：%v", err)
	}
	if repo.updateCalls != 1 || repo.updateActor != "admin" || repo.updateSessionID != "session-1" || repo.updateCandidateID != 9 {
		t.Fatalf("更新边界参数未原样传递：%+v", repo)
	}
}

func TestBatchSaveRejects201ItemsBeforeRepositoryAccess(t *testing.T) {
	service, repo := newCaptureCandidateTestService()
	items := make([]model.CandidateSaveItem, 201)
	for index := range items {
		items[index] = model.CandidateSaveItem{CandidateID: int64(index + 1), Resolution: "create"}
	}
	_, err := service.BatchSave(context.Background(), "admin", model.CandidateBatchSaveRequest{SessionID: "session-1", Items: items})
	if err == nil || !strings.Contains(err.Error(), "1 到 200") {
		t.Fatalf("错误不符合预期：%v", err)
	}
	if repo.batchDataCalls != 0 || repo.saveCalls != 0 {
		t.Fatalf("201 项不应访问仓储：batch=%d save=%d", repo.batchDataCalls, repo.saveCalls)
	}
}

func TestBatchSaveQualityGateMatrix(t *testing.T) {
	tests := []struct {
		name      string
		session   string
		candidate model.ElementCaptureCandidate
		item      model.CandidateSaveItem
		wantField string
	}{
		{name: "blank name", candidate: validCaptureCandidate(1, " ", testFingerprintA), item: model.CandidateSaveItem{CandidateID: 1, Resolution: "create"}, wantField: "name"},
		{name: "unnamed element", candidate: validCaptureCandidate(1, "未命名元素", testFingerprintA), item: model.CandidateSaveItem{CandidateID: 1, Resolution: "create"}, wantField: "name"},
		{name: "empty locators", candidate: func() model.ElementCaptureCandidate {
			c := validCaptureCandidate(1, "save", testFingerprintA)
			c.Locators = nil
			return c
		}(), item: model.CandidateSaveItem{CandidateID: 1, Resolution: "create"}, wantField: "locators"},
		{name: "locator score below threshold", candidate: func() model.ElementCaptureCandidate {
			c := validCaptureCandidate(1, "save", testFingerprintA)
			c.Locators = []byte(`[{"type":"css","value":"button","score":69,"unique":true}]`)
			c.QualityScore = 69
			return c
		}(), item: model.CandidateSaveItem{CandidateID: 1, Resolution: "create"}, wantField: "qualityScore"},
		{name: "all locators non unique", candidate: func() model.ElementCaptureCandidate {
			c := validCaptureCandidate(1, "save", testFingerprintA)
			c.Locators = []byte(`[{"type":"css","value":"button","score":95,"unique":false}]`)
			return c
		}(), item: model.CandidateSaveItem{CandidateID: 1, Resolution: "create"}, wantField: "locators"},
		{name: "invalid resolution", candidate: validCaptureCandidate(1, "save", testFingerprintA), item: model.CandidateSaveItem{CandidateID: 1, Resolution: "overwrite"}, wantField: "resolution"},
		{name: "unresolved duplicate", candidate: func() model.ElementCaptureCandidate {
			c := validCaptureCandidate(1, "save", testFingerprintA)
			c.ConflictStatus = "duplicate"
			return c
		}(), item: model.CandidateSaveItem{CandidateID: 1}, wantField: "resolution"},
		{name: "processed candidate", candidate: func() model.ElementCaptureCandidate {
			c := validCaptureCandidate(1, "save", testFingerprintA)
			c.Status = "saved"
			return c
		}(), item: model.CandidateSaveItem{CandidateID: 1, Resolution: "create"}, wantField: "status"},
		{name: "session outside review state", session: CaptureInterrupted, candidate: validCaptureCandidate(1, "save", testFingerprintA), item: model.CandidateSaveItem{CandidateID: 1, Resolution: "create"}, wantField: "sessionId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, repo := newCaptureCandidateTestService(tt.candidate)
			if tt.session != "" {
				repo.session.Status = tt.session
			}
			_, err := service.BatchSave(context.Background(), "admin", model.CandidateBatchSaveRequest{
				SessionID: "session-1",
				Items:     []model.CandidateSaveItem{tt.item},
			})
			issues := requireCandidateIssues(t, err)
			requireIssueField(t, issues, tt.wantField)
			if repo.saveCalls != 0 {
				t.Fatalf("预检失败不应启动保存事务，实际调用 %d 次", repo.saveCalls)
			}
		})
	}
}

func TestBatchSaveReturnsStructuredIssuesWithoutStartingSaveTransaction(t *testing.T) {
	candidate := validCaptureCandidate(1, "未命名元素", testFingerprintA)
	service, repo := newCaptureCandidateTestService(candidate)
	_, err := service.BatchSave(context.Background(), "admin", model.CandidateBatchSaveRequest{
		SessionID: "session-1",
		Items:     []model.CandidateSaveItem{{CandidateID: 1, Resolution: "create"}},
	})
	issues := requireCandidateIssues(t, err)
	if issues[0].CandidateID != 1 {
		t.Fatalf("结构化问题未携带候选游标：%+v", issues)
	}
	if repo.saveCalls != 0 {
		t.Fatalf("预检失败不应启动保存事务，实际调用 %d 次", repo.saveCalls)
	}
}

func TestBatchSaveResolutionAndEffectiveTargetMatrix(t *testing.T) {
	tests := []struct {
		name                 string
		candidates           []model.ElementCaptureCandidate
		existingNames        map[string][]int64
		existingFingerprints map[string][]int64
		items                []model.CandidateSaveItem
		wantField            string
		wantSuccess          bool
	}{
		{
			name: "create explicitly allows duplicate fingerprint",
			candidates: []model.ElementCaptureCandidate{func() model.ElementCaptureCandidate {
				c := validCaptureCandidate(1, "copy", testFingerprintA)
				c.ConflictStatus = "duplicate"
				c.DuplicateElementID = 11
				c.DuplicateElementPageID = 8
				return c
			}()},
			existingFingerprints: map[string][]int64{testFingerprintA: {11}},
			items:                []model.CandidateSaveItem{{CandidateID: 1, Resolution: "create"}},
			wantSuccess:          true,
		},
		{
			name:        "ignore bypasses quality gates",
			candidates:  []model.ElementCaptureCandidate{{ID: "candidate-ignore", CursorID: 1, SessionID: "session-1", Name: "未命名元素", Status: "pending"}},
			items:       []model.CandidateSaveItem{{CandidateID: 1, Resolution: "ignore"}},
			wantSuccess: true,
		},
		{
			name:                 "create rejects target",
			candidates:           []model.ElementCaptureCandidate{validCaptureCandidate(1, "copy", testFingerprintA)},
			existingFingerprints: map[string][]int64{testFingerprintA: {11}},
			items:                []model.CandidateSaveItem{{CandidateID: 1, Resolution: "create", TargetElementID: 11}},
			wantField:            "targetElementId",
		},
		{
			name:                 "ignore rejects target",
			candidates:           []model.ElementCaptureCandidate{validCaptureCandidate(1, "copy", testFingerprintA)},
			existingFingerprints: map[string][]int64{testFingerprintA: {11}},
			items:                []model.CandidateSaveItem{{CandidateID: 1, Resolution: "ignore", TargetElementID: 11}},
			wantField:            "targetElementId",
		},
		{
			name: "explicit target overrides stale candidate target and self excludes name",
			candidates: []model.ElementCaptureCandidate{func() model.ElementCaptureCandidate {
				c := validCaptureCandidate(1, "save", testFingerprintA)
				c.DuplicateElementID = 99
				c.DuplicateElementPageID = 9
				return c
			}()},
			existingNames:        map[string][]int64{"save": {12}},
			existingFingerprints: map[string][]int64{testFingerprintA: {11, 12}},
			items:                []model.CandidateSaveItem{{CandidateID: 1, Resolution: "update", TargetElementID: 12}},
			wantSuccess:          true,
		},
		{
			name: "single current target overrides stale candidate target",
			candidates: []model.ElementCaptureCandidate{func() model.ElementCaptureCandidate {
				c := validCaptureCandidate(1, "save", testFingerprintA)
				c.DuplicateElementID = 99
				c.DuplicateElementPageID = 9
				return c
			}()},
			existingNames:        map[string][]int64{"save": {11}},
			existingFingerprints: map[string][]int64{testFingerprintA: {11}},
			items:                []model.CandidateSaveItem{{CandidateID: 1, Resolution: "update"}},
			wantSuccess:          true,
		},
		{
			name:                 "multiple current targets require explicit target",
			candidates:           []model.ElementCaptureCandidate{validCaptureCandidate(1, "save", testFingerprintA)},
			existingFingerprints: map[string][]int64{testFingerprintA: {11, 12}},
			items:                []model.CandidateSaveItem{{CandidateID: 1, Resolution: "update"}},
			wantField:            "targetElementId",
		},
		{
			name:                 "explicit target must match current fingerprint",
			candidates:           []model.ElementCaptureCandidate{validCaptureCandidate(1, "save", testFingerprintA)},
			existingFingerprints: map[string][]int64{testFingerprintA: {11, 12}},
			items:                []model.CandidateSaveItem{{CandidateID: 1, Resolution: "update", TargetElementID: 13}},
			wantField:            "targetElementId",
		},
		{
			name: "cross page explicit target is rejected using effective target",
			candidates: []model.ElementCaptureCandidate{func() model.ElementCaptureCandidate {
				c := validCaptureCandidate(1, "save", testFingerprintA)
				c.DuplicateElementID = 21
				c.DuplicateElementPageID = 9
				return c
			}()},
			existingFingerprints: map[string][]int64{testFingerprintA: {11}},
			items:                []model.CandidateSaveItem{{CandidateID: 1, Resolution: "update", TargetElementID: 21}},
			wantField:            "targetElementId",
		},
		{
			name:                 "deleted explicit target is rejected",
			candidates:           []model.ElementCaptureCandidate{validCaptureCandidate(1, "save", testFingerprintA)},
			existingFingerprints: map[string][]int64{testFingerprintA: {11}},
			items:                []model.CandidateSaveItem{{CandidateID: 1, Resolution: "update", TargetElementID: 22}},
			wantField:            "targetElementId",
		},
		{
			name: "same effective target cannot be updated twice",
			candidates: []model.ElementCaptureCandidate{
				validCaptureCandidate(1, "save-one", testFingerprintA),
				validCaptureCandidate(2, "save-two", testFingerprintA),
			},
			existingFingerprints: map[string][]int64{testFingerprintA: {11}},
			items: []model.CandidateSaveItem{
				{CandidateID: 1, Resolution: "update"},
				{CandidateID: 2, Resolution: "update"},
			},
			wantField: "targetElementId",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, repo := newCaptureCandidateTestService(tt.candidates...)
			if tt.existingNames != nil {
				repo.existingNames = tt.existingNames
			}
			if tt.existingFingerprints != nil {
				repo.existingFingerprints = tt.existingFingerprints
			}
			_, err := service.BatchSave(context.Background(), "admin", model.CandidateBatchSaveRequest{
				SessionID: "session-1",
				Items:     tt.items,
			})
			if tt.wantSuccess {
				if err != nil {
					t.Fatalf("合法决策返回错误：%v", err)
				}
				if repo.saveCalls != 1 || !repo.saveSucceeded {
					t.Fatalf("合法决策未完成锁后复检：saveCalls=%d succeeded=%v", repo.saveCalls, repo.saveSucceeded)
				}
				return
			}
			issues := requireCandidateIssues(t, err)
			requireIssueField(t, issues, tt.wantField)
			if repo.saveCalls != 0 {
				t.Fatalf("预检失败不应启动保存事务，实际调用 %d 次", repo.saveCalls)
			}
		})
	}
}

func TestBatchSaveRevalidatesLatestLockedCandidate(t *testing.T) {
	service, repo := newCaptureCandidateTestService(validCaptureCandidate(1, "save", testFingerprintA))
	latest := validCaptureCandidate(1, "save", testFingerprintA)
	latest.Status = "saved"
	repo.lockedCandidates = []model.ElementCaptureCandidate{latest}

	_, err := service.BatchSave(context.Background(), "admin", model.CandidateBatchSaveRequest{
		SessionID: "session-1",
		Items:     []model.CandidateSaveItem{{CandidateID: 1, Resolution: "create"}},
	})
	issues := requireCandidateIssues(t, err)
	requireIssueField(t, issues, "status")
	if repo.saveCalls != 1 || repo.saveSucceeded {
		t.Fatalf("锁后复检未阻止陈旧候选写入：saveCalls=%d succeeded=%v", repo.saveCalls, repo.saveSucceeded)
	}
}

func TestBatchSaveRejectsDuplicateCandidateNamesIndependently(t *testing.T) {
	service, repo := newCaptureCandidateTestService(
		validCaptureCandidate(1, "save", testFingerprintA),
		validCaptureCandidate(2, "SAVE", testFingerprintB),
	)
	_, err := service.BatchSave(context.Background(), "admin", model.CandidateBatchSaveRequest{
		SessionID: "session-1",
		Items: []model.CandidateSaveItem{
			{CandidateID: 1, Resolution: "create"},
			{CandidateID: 2, Resolution: "create"},
		},
	})
	issues := requireCandidateIssues(t, err)
	requireIssueField(t, issues, "name")
	if repo.saveCalls != 0 {
		t.Fatalf("名称预检失败不应启动保存事务，实际调用 %d 次", repo.saveCalls)
	}
}
