package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"synapseqa/backend/internal/model"
)

const (
	CaptureStarting    = "starting"
	CaptureActive      = "active"
	CaptureInterrupted = "interrupted"
	CaptureCompleted   = "completed"
	CaptureExpired     = "expired"
	CaptureFailed      = "failed"
)

type ElementCaptureRepository interface {
	PageExists(ctx context.Context, pageID int64) (bool, error)
	HasActiveCapture(ctx context.Context, executorID string) (bool, error)
	CreateSession(ctx context.Context, session model.ElementCaptureSession) error
	GetSession(ctx context.Context, userID int64, sessionID string) (model.ElementCaptureSessionDetail, error)
	SetMode(ctx context.Context, actor, sessionID, mode string) (bool, error)
	StopSession(ctx context.Context, actor, sessionID string) (bool, error)
	Heartbeat(ctx context.Context, sessionID, executorID, tokenHash, browserContextID, currentURL string) (bool, error)
	ExpireSessions(ctx context.Context, now time.Time) error
}

type CaptureExecutorReader interface {
	GetByID(ctx context.Context, executorID string) (model.ExecutorView, error)
}

const (
	maxCaptureCandidates     = 500
	warningCaptureCandidates = 400
	maxBatchSaveCandidates   = 200
	reliableLocatorScore     = 70
)

var captureFingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type CandidateCaptureRepository interface {
	AddCandidate(ctx context.Context, candidate model.ElementCaptureCandidate, executorID, tokenHash string) (model.ElementCaptureCandidate, error)
	ListCandidates(ctx context.Context, userID int64, sessionID string, afterID int64, limit int) ([]model.ElementCaptureCandidate, error)
	UpdateCandidate(ctx context.Context, actor, sessionID string, candidateID int64, req model.CaptureCandidateUpdateRequest) (bool, error)
	GetBatchSaveData(ctx context.Context, sessionID string, candidateIDs []int64) (model.CaptureBatchData, error)
	SaveCandidates(ctx context.Context, actor string, req model.CandidateBatchSaveRequest, validate func(model.CaptureBatchData) error) (model.BatchSaveResult, error)
}

type CaptureVersionRepository interface {
	ListVersions(ctx context.Context, elementID int64) ([]model.PageElementVersion, error)
	RollbackVersion(ctx context.Context, actor string, elementID int64, version int) (model.PageElementVersion, error)
}

type CaptureExecutorRepository interface {
	FailSession(ctx context.Context, sessionID, executorID, tokenHash, reason string) (bool, error)
	ListCommands(ctx context.Context, executorID string) ([]model.ElementCaptureCommand, error)
}

type captureLocator struct {
	Type   string  `json:"type"`
	Value  string  `json:"value"`
	Score  float64 `json:"score"`
	Unique bool    `json:"unique"`
}

type ElementCaptureService struct {
	repo           ElementCaptureRepository
	executorReader CaptureExecutorReader
	now            func() time.Time
}

func NewElementCaptureService(repo ElementCaptureRepository, executorReader CaptureExecutorReader, _ []byte) *ElementCaptureService {
	return &ElementCaptureService{repo: repo, executorReader: executorReader, now: time.Now}
}

func (s *ElementCaptureService) CreateSession(ctx context.Context, actor string, req model.CaptureSessionCreateRequest) (model.CaptureSessionCreated, error) {
	pageExists, err := s.repo.PageExists(ctx, req.PageID)
	if err != nil {
		return model.CaptureSessionCreated{}, err
	}
	if !pageExists {
		return model.CaptureSessionCreated{}, errors.New("页面不存在")
	}
	executor, err := s.executorReader.GetByID(ctx, req.ExecutorID)
	if err != nil {
		return model.CaptureSessionCreated{}, err
	}
	if executor.Status != "online" {
		return model.CaptureSessionCreated{}, errors.New("执行器不在线")
	}
	if !supportsUI(executor.SupportedTypes) {
		return model.CaptureSessionCreated{}, errors.New("执行器不支持 UI")
	}
	if !isCaptureURL(req.URL) {
		return model.CaptureSessionCreated{}, errors.New("页面地址必须是绝对 http/https URL")
	}
	if req.BrowserChannel != "chrome" && req.BrowserChannel != "msedge" {
		return model.CaptureSessionCreated{}, errors.New("仅支持 Chrome 或 Edge 浏览器")
	}
	if req.Mode != "" && req.Mode != "pick" && req.Mode != "operate" {
		return model.CaptureSessionCreated{}, errors.New("仅支持 pick 或 operate 模式")
	}
	busy, err := s.repo.HasActiveCapture(ctx, req.ExecutorID)
	if err != nil {
		return model.CaptureSessionCreated{}, err
	}
	if busy {
		return model.CaptureSessionCreated{}, errors.New("执行器正在采集页面元素")
	}
	token, err := newCaptureToken()
	if err != nil {
		return model.CaptureSessionCreated{}, errors.New("生成采集会话令牌失败")
	}
	now := s.now()
	mode := req.Mode
	if mode == "" {
		mode = "pick"
	}
	currentURL := req.CurrentURL
	if currentURL == "" {
		currentURL = req.URL
	}
	if !isCaptureURL(currentURL) {
		return model.CaptureSessionCreated{}, errors.New("页面地址必须是绝对 http/https URL")
	}
	tokenHash := sha256.Sum256([]byte(token))
	session := model.ElementCaptureSession{
		ID:               newCaptureID(),
		PageID:           req.PageID,
		ExecutorID:       req.ExecutorID,
		BrowserContextID: "",
		BrowserChannel:   req.BrowserChannel,
		CreatedBy:        actor,
		Status:           CaptureStarting,
		Mode:             mode,
		CurrentURL:       currentURL,
		TokenHash:        hex.EncodeToString(tokenHash[:]),
		LastHeartbeatAt:  now,
		ExpiresAt:        now.Add(30 * time.Minute),
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.CaptureSessionCreated{}, errors.New("执行器正在采集页面元素")
		}
		return model.CaptureSessionCreated{}, err
	}
	return model.CaptureSessionCreated{Session: session, Token: token}, nil
}

func newCaptureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func newCaptureID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func (s *ElementCaptureService) SetMode(ctx context.Context, actor, sessionID, mode string) error {
	if mode != "pick" && mode != "operate" {
		return errors.New("仅支持 pick 或 operate 模式")
	}
	updated, err := s.repo.SetMode(ctx, actor, sessionID, mode)
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("采集会话不存在或已结束")
	}
	return nil
}

func (s *ElementCaptureService) GetSession(ctx context.Context, userID int64, sessionID string) (model.ElementCaptureSessionDetail, error) {
	return s.repo.GetSession(ctx, userID, sessionID)
}

func (s *ElementCaptureService) StopSession(ctx context.Context, actor, sessionID string) error {
	_, err := s.repo.StopSession(ctx, actor, sessionID)
	return err
}

func (s *ElementCaptureService) Heartbeat(ctx context.Context, sessionID, executorID, token, browserContextID, currentURL string) error {
	if browserContextID == "" {
		return errors.New("浏览器上下文不能为空")
	}
	if !isCaptureURL(currentURL) {
		return errors.New("页面地址必须是绝对 http/https URL")
	}
	tokenHash := sha256.Sum256([]byte(token))
	updated, err := s.repo.Heartbeat(ctx, sessionID, executorID, hex.EncodeToString(tokenHash[:]), browserContextID, currentURL)
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("采集会话不存在、执行器或令牌无效，或恢复窗口已过期")
	}
	return nil
}

func (s *ElementCaptureService) ExpireSessions(ctx context.Context, now time.Time) error {
	return s.repo.ExpireSessions(ctx, now)
}

func (s *ElementCaptureService) FailSession(ctx context.Context, sessionID, executorID, token, reason string) error {
	repo, ok := s.repo.(CaptureExecutorRepository)
	if !ok {
		return errors.New("采集执行器仓储未配置")
	}
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(executorID) == "" || strings.TrimSpace(token) == "" || strings.TrimSpace(reason) == "" {
		return errors.New("采集失败回调参数无效")
	}
	hash := sha256.Sum256([]byte(token))
	updated, err := repo.FailSession(ctx, sessionID, executorID, hex.EncodeToString(hash[:]), strings.TrimSpace(reason))
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("采集会话不存在、执行器或令牌无效")
	}
	return nil
}

func (s *ElementCaptureService) ListCommands(ctx context.Context, executorID string) ([]model.ElementCaptureCommand, error) {
	repo, ok := s.repo.(CaptureExecutorRepository)
	if !ok {
		return nil, errors.New("采集执行器仓储未配置")
	}
	if strings.TrimSpace(executorID) == "" {
		return nil, errors.New("executorId 不能为空")
	}
	return repo.ListCommands(ctx, executorID)
}

func (s *ElementCaptureService) ListVersions(ctx context.Context, elementID int64) ([]model.PageElementVersion, error) {
	repo, ok := s.repo.(CaptureVersionRepository)
	if !ok {
		return nil, errors.New("页面元素版本仓储未配置")
	}
	if elementID <= 0 {
		return nil, errors.New("页面元素 ID 无效")
	}
	return repo.ListVersions(ctx, elementID)
}

func (s *ElementCaptureService) RollbackVersion(ctx context.Context, actor string, elementID int64, version int) (model.PageElementVersion, error) {
	repo, ok := s.repo.(CaptureVersionRepository)
	if !ok {
		return model.PageElementVersion{}, errors.New("页面元素版本仓储未配置")
	}
	if strings.TrimSpace(actor) == "" || elementID <= 0 || version <= 0 {
		return model.PageElementVersion{}, errors.New("版本回滚参数无效")
	}
	return repo.RollbackVersion(ctx, actor, elementID, version)
}

func (s *ElementCaptureService) AddCandidate(ctx context.Context, executorID, token string, req model.CaptureCandidateCreateRequest) (model.ElementCaptureCandidate, error) {
	repo, err := s.candidateRepo()
	if err != nil {
		return model.ElementCaptureCandidate{}, err
	}
	if err := validateCandidateCreate(req); err != nil {
		return model.ElementCaptureCandidate{}, err
	}
	tokenHash := sha256.Sum256([]byte(token))
	candidate := model.ElementCaptureCandidate{
		SessionID: req.SessionID, Name: strings.TrimSpace(req.Name), Fingerprint: req.Fingerprint,
		CaptureURL: req.CaptureURL, TagName: strings.TrimSpace(req.TagName), AccessibleName: strings.TrimSpace(req.AccessibleName),
		Locators: req.Locators, QualityScore: req.QualityScore, Status: "pending",
	}
	return repo.AddCandidate(ctx, candidate, executorID, hex.EncodeToString(tokenHash[:]))
}

func (s *ElementCaptureService) ListCandidates(ctx context.Context, userID int64, sessionID string, afterID int64, limit int) ([]model.ElementCaptureCandidate, error) {
	repo, err := s.candidateRepo()
	if err != nil {
		return nil, err
	}
	if afterID < 0 {
		return nil, errors.New("afterID 不能小于 0")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 0 || limit > maxBatchSaveCandidates {
		return nil, errors.New("limit 必须在 1 到 200 之间")
	}
	return repo.ListCandidates(ctx, userID, sessionID, afterID, limit)
}

func (s *ElementCaptureService) UpdateCandidate(ctx context.Context, actor, sessionID string, candidateID int64, req model.CaptureCandidateUpdateRequest) error {
	repo, err := s.candidateRepo()
	if err != nil {
		return err
	}
	if strings.TrimSpace(actor) == "" || candidateID <= 0 || strings.TrimSpace(sessionID) == "" {
		return errors.New("采集候选项参数无效")
	}
	if req.Name != "" && strings.TrimSpace(req.Name) == "" {
		return errors.New("候选项名称不能为空")
	}
	if req.Locators != nil {
		if _, err := parseCaptureLocators(req.Locators); err != nil {
			return err
		}
	}
	if req.QualityScore != nil && *req.QualityScore < 0 {
		return errors.New("质量评分不能小于 0")
	}
	if req.ConflictResolution != "" && !isResolution(req.ConflictResolution) {
		return errors.New("冲突处理方式无效")
	}
	updated, err := repo.UpdateCandidate(ctx, actor, sessionID, candidateID, req)
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("采集候选项不存在或会话不可审核")
	}
	return nil
}

func (s *ElementCaptureService) BatchSave(ctx context.Context, actor string, req model.CandidateBatchSaveRequest) (model.BatchSaveResult, error) {
	repo, err := s.candidateRepo()
	if err != nil {
		return model.BatchSaveResult{}, err
	}
	if strings.TrimSpace(actor) == "" {
		return model.BatchSaveResult{}, errors.New("操作人不能为空")
	}
	if strings.TrimSpace(req.SessionID) == "" || len(req.Items) == 0 || len(req.Items) > maxBatchSaveCandidates {
		return model.BatchSaveResult{}, errors.New("批量保存项必须在 1 到 200 之间")
	}
	ids := make([]int64, 0, len(req.Items))
	for _, item := range req.Items {
		ids = append(ids, item.CandidateID)
	}
	data, err := repo.GetBatchSaveData(ctx, req.SessionID, ids)
	if err != nil {
		return model.BatchSaveResult{}, err
	}
	issues := validateBatchSave(data, req)
	if len(issues) > 0 {
		return model.BatchSaveResult{}, candidateIssuesError(issues)
	}
	return repo.SaveCandidates(ctx, actor, req, func(locked model.CaptureBatchData) error {
		issues := validateBatchSave(locked, req)
		if len(issues) > 0 {
			return candidateIssuesError(issues)
		}
		return nil
	})
}

func (s *ElementCaptureService) candidateRepo() (CandidateCaptureRepository, error) {
	repo, ok := s.repo.(CandidateCaptureRepository)
	if !ok {
		return nil, errors.New("采集候选项仓储未配置")
	}
	return repo, nil
}

func validateCandidateCreate(req model.CaptureCandidateCreateRequest) error {
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Name) == "" || !isCaptureURL(req.CaptureURL) {
		return errors.New("候选项会话、名称和采集地址无效")
	}
	if !captureFingerprintPattern.MatchString(req.Fingerprint) {
		return errors.New("fingerprint 必须是 64 位小写十六进制 SHA-256")
	}
	_, err := parseCaptureLocators(req.Locators)
	return err
}

func parseCaptureLocators(raw json.RawMessage) ([]captureLocator, error) {
	var locators []captureLocator
	if len(raw) == 0 || json.Unmarshal(raw, &locators) != nil || len(locators) == 0 || len(locators) > 3 {
		return nil, errors.New("locators 必须是非空数组")
	}
	for _, locator := range locators {
		if strings.TrimSpace(locator.Type) == "" || strings.TrimSpace(locator.Value) == "" {
			return nil, errors.New("locators 包含无效定位器")
		}
	}
	return locators, nil
}

func validateBatchSave(data model.CaptureBatchData, req model.CandidateBatchSaveRequest) []model.CandidateIssue {
	issues := make([]model.CandidateIssue, 0)
	if data.Session.Status != CaptureActive && data.Session.Status != CaptureCompleted {
		return append(issues, model.CandidateIssue{Field: "sessionId", Message: "会话当前状态不能保存候选项"})
	}
	byID := make(map[int64]model.ElementCaptureCandidate, len(data.Candidates))
	for _, candidate := range data.Candidates {
		byID[candidate.CursorID] = candidate
	}
	seenIDs, names := map[int64]bool{}, map[string]bool{}
	updateTargets := map[int64]bool{}
	for _, item := range req.Items {
		candidate, ok := byID[item.CandidateID]
		if !ok || seenIDs[item.CandidateID] {
			issues = append(issues, model.CandidateIssue{CandidateID: item.CandidateID, Field: "candidateId", Message: "候选项不存在或重复"})
			continue
		}
		seenIDs[item.CandidateID] = true
		if candidate.Status != "pending" {
			issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "status", Message: "候选项已处理"})
		}
		if !isResolution(item.Resolution) {
			issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "resolution", Message: "冲突处理方式无效"})
		}
		if candidate.ConflictStatus == "duplicate" && item.Resolution == "" {
			issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "resolution", Message: "重复候选项必须明确处理方式"})
		}
		if item.TargetElementID != 0 && item.Resolution != "update" {
			issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "targetElementId", Message: "仅 update 可以指定目标元素"})
		}
		if item.Resolution == "ignore" {
			continue
		}

		targets := data.ExistingFingerprints[candidate.Fingerprint]
		effectiveTarget := int64(0)
		if item.Resolution == "update" {
			if item.TargetElementID != 0 {
				effectiveTarget = item.TargetElementID
			} else if len(targets) == 1 {
				effectiveTarget = targets[0]
			}

			switch {
			case len(targets) == 0 && item.TargetElementID != 0:
				issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "targetElementId", Message: "更新目标不属于会话页面、已失效或不是当前同指纹元素"})
			case len(targets) == 0:
				issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "duplicateElementId", Message: "当前不存在可更新的重复元素"})
			case effectiveTarget == 0:
				issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "targetElementId", Message: "当前指纹对应多个元素，请选择目标"})
			case !containsElementID(targets, effectiveTarget):
				issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "targetElementId", Message: "更新目标不属于会话页面、已失效或不是当前同指纹元素"})
			default:
				if updateTargets[effectiveTarget] {
					issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "targetElementId", Message: "同一批次不能重复更新同一元素"})
				}
				updateTargets[effectiveTarget] = true
			}
		}

		issues = append(issues, validateCandidateForSave(candidate, item)...)
		name := strings.ToLower(strings.TrimSpace(candidate.Name))
		if names[name] {
			issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "name", Message: "页面内元素名称重复"})
		}
		if len(data.ExistingNames[name]) > 0 {
			for _, existingID := range data.ExistingNames[name] {
				if item.Resolution != "update" || existingID != effectiveTarget {
					issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: "name", Message: "页面内元素名称重复"})
					break
				}
			}
		}
		names[name] = true
	}
	return issues
}

func containsElementID(ids []int64, wanted int64) bool {
	for _, id := range ids {
		if id == wanted {
			return true
		}
	}
	return false
}

func validateCandidateForSave(candidate model.ElementCaptureCandidate, _ model.CandidateSaveItem) []model.CandidateIssue {
	issues := make([]model.CandidateIssue, 0)
	issue := func(field, message string) {
		issues = append(issues, model.CandidateIssue{CandidateID: candidate.CursorID, Field: field, Message: message})
	}
	if strings.TrimSpace(candidate.Name) == "" || strings.TrimSpace(candidate.Name) == "未命名元素" {
		issue("name", "候选项名称不可靠")
	}
	locators, err := parseCaptureLocators(candidate.Locators)
	if err != nil {
		issue("locators", "候选项缺少可靠定位器")
	} else {
		maxScore, reliableUnique := candidate.QualityScore, false
		for _, locator := range locators {
			if locator.Score > maxScore {
				maxScore = locator.Score
			}
			reliableUnique = reliableUnique || (locator.Unique && locator.Score >= reliableLocatorScore)
		}
		if maxScore < reliableLocatorScore {
			issue("qualityScore", "候选项定位器评分不足")
		}
		if !reliableUnique {
			issue("locators", "候选项没有可靠唯一定位器")
		}
	}
	return issues
}

func isResolution(value string) bool {
	return value == "update" || value == "ignore" || value == "create"
}

// CandidateIssuesError 保留批量审核的逐候选结构化问题，同时兼容既有错误文本。
type CandidateIssuesError struct {
	Issues []model.CandidateIssue
}

func (e *CandidateIssuesError) Error() string {
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		if issue.CandidateID == 0 {
			parts = append(parts, issue.Message)
			continue
		}
		parts = append(parts, fmt.Sprintf("候选项 %d：%s", issue.CandidateID, issue.Message))
	}
	return strings.Join(parts, "；")
}

func (e *CandidateIssuesError) CandidateIssues() []model.CandidateIssue {
	return append([]model.CandidateIssue(nil), e.Issues...)
}

func candidateIssuesError(issues []model.CandidateIssue) error {
	return &CandidateIssuesError{Issues: append([]model.CandidateIssue(nil), issues...)}
}

func supportsUI(types []string) bool {
	for _, item := range types {
		if strings.EqualFold(item, "ui") {
			return true
		}
	}
	return false
}

func isCaptureURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.IsAbs() && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
