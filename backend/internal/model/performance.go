package model

import (
	"encoding/json"
	"time"
)

type PerfTestPlan struct {
	ID          int64           `json:"id"`
	ProductID   int64           `json:"productId"`
	ProductName string          `json:"productName"`
	Name        string          `json:"name"`
	TargetURL   string          `json:"targetUrl"`
	Method      string          `json:"method"`
	Headers     json.RawMessage `json:"headers"`
	Body        string          `json:"body"`
	LoadMode    string          `json:"loadMode"`
	VUs         int             `json:"vus"`
	Duration    string          `json:"duration"`
	Stages      json.RawMessage `json:"stages"`
	Thresholds  json.RawMessage `json:"thresholds"`
	Status      string          `json:"status"`
	Priority    string          `json:"priority"`
	Owner       string          `json:"owner"`
	Tags        string          `json:"tags"`
	Description string          `json:"description"`
	CreatedBy   string          `json:"createdBy"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type PerfTestPlanRequest struct {
	ProductID   int64           `json:"productId"`
	Name        string          `json:"name"`
	TargetURL   string          `json:"targetUrl"`
	Method      string          `json:"method"`
	Headers     json.RawMessage `json:"headers"`
	Body        string          `json:"body"`
	LoadMode    string          `json:"loadMode"`
	VUs         int             `json:"vus"`
	Duration    string          `json:"duration"`
	Stages      json.RawMessage `json:"stages"`
	Thresholds  json.RawMessage `json:"thresholds"`
	Status      string          `json:"status"`
	Priority    string          `json:"priority"`
	Owner       string          `json:"owner"`
	Tags        string          `json:"tags"`
	Description string          `json:"description"`
}

type PerfTestPlanFilter struct {
	ID        string
	Name      string
	ProductID string
	LoadMode  string
	Priority  string
	Status    string
	Owner     string
}

type PerfTestRun struct {
	ID            int64           `json:"id"`
	PlanID        int64           `json:"planId"`
	PlanName      string          `json:"planName"`
	Status        string          `json:"status"`
	TriggeredBy   string          `json:"triggeredBy"`
	ExitCode      *int            `json:"exitCode"`
	TotalRequests int             `json:"totalRequests"`
	AvgDurationMs *float64        `json:"avgDurationMs"`
	P95DurationMs *float64        `json:"p95DurationMs"`
	ErrorRate     *float64        `json:"errorRate"`
	RPS           *float64        `json:"rps"`
	Summary       json.RawMessage `json:"summary"`
	StartedAt     *time.Time      `json:"startedAt"`
	FinishedAt    *time.Time      `json:"finishedAt"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

type PerfTestRunFilter struct {
	ID     string
	PlanID string
	Status string
}
