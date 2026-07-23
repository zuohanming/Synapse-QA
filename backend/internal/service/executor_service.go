package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/repository"
)

// ExecutorService 负责执行器注册、心跳鉴权和在线状态判定。
type ExecutorService struct {
	repo         *repository.ExecutorRepository
	token        string
	offlineAfter time.Duration
	suspectAfter time.Duration
	notifier     interface {
		Broadcast(context.Context, model.NotificationCreate) error
	}
	stateMu     sync.Mutex
	states      map[string]string
	tokenAlerts map[string]bool
}

func NewExecutorService(repo *repository.ExecutorRepository, token string) *ExecutorService {
	return &ExecutorService{
		repo:         repo,
		token:        token,
		suspectAfter: 30 * time.Second,
		offlineAfter: 60 * time.Second,
		states:       map[string]string{}, tokenAlerts: map[string]bool{},
	}
}

func (s *ExecutorService) SetNotifier(notifier interface {
	Broadcast(context.Context, model.NotificationCreate) error
}) {
	s.notifier = notifier
}

func (s *ExecutorService) Register(ctx context.Context, token string, req model.ExecutorRegisterRequest) (model.ExecutorView, error) {
	if strings.TrimSpace(req.ExecutorID) == "" {
		return model.ExecutorView{}, errors.New("执行器 ID 不能为空")
	}
	if err := s.validateToken(ctx, req.ExecutorID, token); err != nil {
		return model.ExecutorView{}, err
	}
	if req.Name == "" {
		req.Name = req.ExecutorID
	}
	if len(req.SupportedTypes) == 0 {
		return model.ExecutorView{}, errors.New("执行器能力不能为空")
	}
	item, err := s.repo.UpsertRegister(ctx, req)
	if err == nil {
		s.notifyLifecycle(ctx, item, "connected")
	}
	return item, err
}

func (s *ExecutorService) Heartbeat(ctx context.Context, token string, req model.ExecutorHeartbeatRequest) (model.ExecutorView, error) {
	if strings.TrimSpace(req.ExecutorID) == "" {
		return model.ExecutorView{}, errors.New("执行器 ID 不能为空")
	}
	if err := s.validateToken(ctx, req.ExecutorID, token); err != nil {
		return model.ExecutorView{}, err
	}
	if req.Status == "" {
		req.Status = "online"
	}
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}
	item, err := s.repo.UpsertHeartbeat(ctx, req)
	if err == nil {
		s.notifyLifecycle(ctx, item, req.Status)
	}
	return item, err
}

func (s *ExecutorService) notifyLifecycle(ctx context.Context, item model.ExecutorView, state string) {
	s.stateMu.Lock()
	previous := s.states[item.ExecutorID]
	if previous == state {
		s.stateMu.Unlock()
		return
	}
	s.states[item.ExecutorID] = state
	s.stateMu.Unlock()
	if s.notifier == nil {
		return
	}
	notificationType, level, title := "", "info", ""
	switch state {
	case "connected":
		if previous == "" || previous == "offline" || previous == "stopped" {
			notificationType, level, title = "executor.connected", "success", "执行器已连接平台"
		}
	case "online":
		if previous == "offline" || previous == "suspect" {
			notificationType, level, title = "executor.recovered", "success", "执行器已恢复"
		} else {
			notificationType, level, title = "executor.started", "success", "执行器已启动"
		}
	case "offline":
		notificationType, level, title = "executor.stopped", "warning", "执行器已停止"
	}
	if notificationType != "" {
		_ = s.notifier.Broadcast(ctx, model.NotificationCreate{Type: notificationType, Level: level, Title: title, Content: item.Name, TargetType: "executor", TargetID: item.ExecutorID, TargetURL: "#/测试配置/执行器配置"})
	}
}

func (s *ExecutorService) List(ctx context.Context) ([]model.ExecutorView, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for index := range items {
		elapsed := now.Sub(items[index].LastHeartbeatAt)
		if elapsed > s.offlineAfter {
			items[index].Status = "offline"
		} else if elapsed > s.suspectAfter {
			items[index].Status = "suspect"
		}
		s.notifyStateChange(ctx, items[index])
	}
	return items, nil
}

func (s *ExecutorService) notifyStateChange(ctx context.Context, item model.ExecutorView) {
	s.stateMu.Lock()
	previous := s.states[item.ExecutorID]
	s.states[item.ExecutorID] = item.Status
	s.stateMu.Unlock()
	if s.notifier == nil || previous == "" || previous == item.Status {
		return
	}
	if item.Status == "offline" {
		_ = s.notifier.Broadcast(ctx, model.NotificationCreate{Type: "executor.offline", Level: "error", Title: "执行器已离线", Content: item.Name, TargetType: "executor", TargetID: item.ExecutorID, TargetURL: "#/测试配置/执行器配置"})
	}
	if item.Status == "online" && previous != "online" {
		_ = s.notifier.Broadcast(ctx, model.NotificationCreate{Type: "executor.recovered", Level: "success", Title: "执行器已恢复", Content: item.Name, TargetType: "executor", TargetID: item.ExecutorID, TargetURL: "#/测试配置/执行器配置"})
	}
}

func (s *ExecutorService) Create(ctx context.Context, req model.ExecutorCreateRequest) (model.ExecutorTokenResult, error) {
	req.ExecutorID = strings.TrimSpace(req.ExecutorID)
	req.Name = strings.TrimSpace(req.Name)
	if req.ExecutorID == "" || req.Name == "" {
		return model.ExecutorTokenResult{}, errors.New("执行器 ID 和名称不能为空")
	}
	if err := s.repo.Create(ctx, req); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(err.Error(), "唯一") {
			return model.ExecutorTokenResult{}, errors.New("执行器 ID 已存在")
		}
		return model.ExecutorTokenResult{}, fmt.Errorf("创建执行器失败：%w", err)
	}
	return s.GenerateToken(ctx, req.ExecutorID)
}

func (s *ExecutorService) GenerateToken(ctx context.Context, executorID string) (model.ExecutorTokenResult, error) {
	if strings.TrimSpace(executorID) == "" {
		return model.ExecutorTokenResult{}, errors.New("执行器 ID 不能为空")
	}
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return model.ExecutorTokenResult{}, errors.New("生成执行器令牌失败")
	}
	token := "executor_" + hex.EncodeToString(random)
	updatedAt, err := s.repo.UpdateExecutorToken(ctx, executorID, token)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ExecutorTokenResult{}, errors.New("执行器不存在")
	}
	if err != nil {
		return model.ExecutorTokenResult{}, errors.New("保存执行器令牌失败")
	}
	return model.ExecutorTokenResult{ExecutorID: executorID, Token: token, UpdatedAt: updatedAt}, nil
}

func (s *ExecutorService) validateToken(ctx context.Context, executorID, token string) error {
	specific, err := s.repo.GetExecutorToken(ctx, executorID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.New("读取执行器令牌失败")
	}
	if specific != "" {
		if token == specific {
			s.stateMu.Lock()
			delete(s.tokenAlerts, executorID)
			s.stateMu.Unlock()
			return nil
		}
		s.notifyTokenFailure(ctx, executorID)
		return errors.New("执行器令牌无效")
	}
	expected, err := s.repo.GetSetting(ctx, "executor_shared_token")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.New("读取执行器令牌失败")
	}
	if expected == "" {
		expected = s.token
	}
	if expected == "" || token == expected {
		return nil
	}
	return errors.New("执行器令牌无效")
}

func (s *ExecutorService) notifyTokenFailure(ctx context.Context, executorID string) {
	s.stateMu.Lock()
	already := s.tokenAlerts[executorID]
	s.tokenAlerts[executorID] = true
	s.stateMu.Unlock()
	if !already && s.notifier != nil {
		_ = s.notifier.Broadcast(ctx, model.NotificationCreate{Type: "executor.token", Level: "error", Title: "执行器 Token 已失效", Content: executorID, TargetType: "executor", TargetID: executorID, TargetURL: "#/测试配置/执行器配置"})
	}
}
