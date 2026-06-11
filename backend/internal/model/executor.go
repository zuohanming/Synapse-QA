package model

import "time"

// ExecutorRegisterRequest 是执行器启动时向平台登记的能力信息。
type ExecutorRegisterRequest struct {
	ExecutorID     string   `json:"executorId"`
	Name           string   `json:"name"`
	Endpoint       string   `json:"endpoint"`
	Version        string   `json:"version"`
	MaxWorkers     int      `json:"maxWorkers"`
	SupportedTypes []string `json:"supportedTypes"`
}

// ExecutorHeartbeatRequest 是执行器定时上报的运行状态。
type ExecutorHeartbeatRequest struct {
	ExecutorID     string          `json:"executorId"`
	Status         string          `json:"status"`
	Version        string          `json:"version"`
	MaxWorkers     int             `json:"maxWorkers"`
	RunningTasks   int             `json:"runningTasks"`
	QueuedTasks    int             `json:"queuedTasks"`
	SupportedTypes []string        `json:"supportedTypes"`
	Checks         map[string]bool `json:"checks"`
	Timestamp      time.Time       `json:"timestamp"`
}

// ExecutorView 是平台展示和调度时使用的执行器状态视图。
type ExecutorView struct {
	ExecutorID      string          `json:"executorId"`
	Name            string          `json:"name"`
	Endpoint        string          `json:"endpoint"`
	Status          string          `json:"status"`
	Version         string          `json:"version"`
	MaxWorkers      int             `json:"maxWorkers"`
	RunningTasks    int             `json:"runningTasks"`
	QueuedTasks     int             `json:"queuedTasks"`
	SupportedTypes  []string        `json:"supportedTypes"`
	Checks          map[string]bool `json:"checks"`
	LastHeartbeatAt time.Time       `json:"lastHeartbeatAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	CreatedAt       time.Time       `json:"createdAt"`
}
