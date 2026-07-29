package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
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
