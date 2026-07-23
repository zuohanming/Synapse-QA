package service

import (
	"context"
	"sync"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/repository"
)

type NotificationService struct {
	repo *repository.NotificationRepository
	mu   sync.RWMutex
	subs map[int64]map[chan model.Notification]struct{}
}

func NewNotificationService(repo *repository.NotificationRepository) *NotificationService {
	return &NotificationService{repo: repo, subs: map[int64]map[chan model.Notification]struct{}{}}
}
func (s *NotificationService) List(ctx context.Context, userID int64, unread bool, category string, page, size int) (model.PageResult, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	items, total, err := s.repo.List(ctx, userID, unread, category, page, size)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: size}, err
}
func (s *NotificationService) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	return s.repo.UnreadCount(ctx, userID)
}
func (s *NotificationService) MarkRead(ctx context.Context, userID, id int64) error {
	return s.repo.MarkRead(ctx, userID, id)
}
func (s *NotificationService) MarkAllRead(ctx context.Context, userID int64) error {
	return s.repo.MarkAllRead(ctx, userID)
}
func (s *NotificationService) Delete(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, userID, id)
}
func (s *NotificationService) Preferences(ctx context.Context, userID int64) (model.NotificationPreference, error) {
	return s.repo.GetPreferences(ctx, userID)
}
func (s *NotificationService) UpdatePreferences(ctx context.Context, userID int64, p model.NotificationPreference) error {
	return s.repo.UpdatePreferences(ctx, userID, p)
}
func (s *NotificationService) Create(ctx context.Context, req model.NotificationCreate) error {
	n, err := s.repo.Create(ctx, req)
	if err == nil {
		s.publish(n)
	}
	return err
}
func (s *NotificationService) Broadcast(ctx context.Context, req model.NotificationCreate) error {
	if err := s.repo.CreateForAll(ctx, req); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, channels := range s.subs {
		for ch := range channels {
			select {
			case ch <- model.Notification{}:
			default:
			}
		}
	}
	return nil
}
func (s *NotificationService) Subscribe(userID int64) (chan model.Notification, func()) {
	ch := make(chan model.Notification, 8)
	s.mu.Lock()
	if s.subs[userID] == nil {
		s.subs[userID] = map[chan model.Notification]struct{}{}
	}
	s.subs[userID][ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() { s.mu.Lock(); delete(s.subs[userID], ch); close(ch); s.mu.Unlock() }
}
func (s *NotificationService) publish(n model.Notification) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subs[n.UserID] {
		select {
		case ch <- n:
		default:
		}
	}
}
