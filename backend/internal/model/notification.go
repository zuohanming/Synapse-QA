package model

import "time"

type Notification struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"userId"`
	Type       string     `json:"type"`
	Level      string     `json:"level"`
	Title      string     `json:"title"`
	Content    string     `json:"content"`
	TargetType string     `json:"targetType"`
	TargetID   string     `json:"targetId"`
	TargetURL  string     `json:"targetUrl"`
	IsRead     bool       `json:"isRead"`
	ReadAt     *time.Time `json:"readAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type NotificationCreate struct {
	Username   string
	Type       string
	Level      string
	Title      string
	Content    string
	TargetType string
	TargetID   string
	TargetURL  string
}

type NotificationPreference struct {
	ExecutionSuccess bool `json:"executionSuccess"`
	ExecutionFailure bool `json:"executionFailure"`
	ExecutorAlert    bool `json:"executorAlert"`
	SystemNotice     bool `json:"systemNotice"`
}

type SystemNotificationRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Level   string `json:"level"`
}
