package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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
}

func NewExecutorService(repo *repository.ExecutorRepository, token string) *ExecutorService {
	return &ExecutorService{
		repo:         repo,
		token:        token,
		suspectAfter: 30 * time.Second,
		offlineAfter: 60 * time.Second,
	}
}

func (s *ExecutorService) Register(ctx context.Context, token string, req model.ExecutorRegisterRequest) (model.ExecutorView, error) {
	if err := s.validateToken(ctx, token); err != nil {
		return model.ExecutorView{}, err
	}
	if strings.TrimSpace(req.ExecutorID) == "" {
		return model.ExecutorView{}, errors.New("执行器 ID 不能为空")
	}
	if req.Name == "" {
		req.Name = req.ExecutorID
	}
	if len(req.SupportedTypes) == 0 {
		return model.ExecutorView{}, errors.New("执行器能力不能为空")
	}
	return s.repo.UpsertRegister(ctx, req)
}

func (s *ExecutorService) Heartbeat(ctx context.Context, token string, req model.ExecutorHeartbeatRequest) (model.ExecutorView, error) {
	if err := s.validateToken(ctx, token); err != nil {
		return model.ExecutorView{}, err
	}
	if strings.TrimSpace(req.ExecutorID) == "" {
		return model.ExecutorView{}, errors.New("执行器 ID 不能为空")
	}
	if req.Status == "" {
		req.Status = "online"
	}
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}
	return s.repo.UpsertHeartbeat(ctx, req)
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
	}
	return items, nil
}

func (s *ExecutorService) validateToken(ctx context.Context, token string) error {
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
