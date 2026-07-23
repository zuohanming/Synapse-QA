package model

import (
	"encoding/json"
	"time"
)

// ExecutionRun 表示一次执行批次。
type ExecutionRun struct {
	ID          int64           `json:"id"`
	RunType     string          `json:"runType"`
	Headless    bool            `json:"headless"`
	Status      string          `json:"status"`
	TriggeredBy string          `json:"triggeredBy"`
	CaseIDs     []int64         `json:"caseIds"`
	Summary     json.RawMessage `json:"summary"`
	StartedAt   *time.Time      `json:"startedAt"`
	FinishedAt  *time.Time      `json:"finishedAt"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// ExecutionTask 表示一次执行批次中的单个任务。
type ExecutionTask struct {
	ID          int64           `json:"id"`
	RunID       int64           `json:"runId"`
	TaskID      string          `json:"taskId"`
	CaseID      int64           `json:"caseId"`
	ExecutorID  string          `json:"executorId"`
	TaskType    string          `json:"taskType"`
	Payload     json.RawMessage `json:"payload"`
	CallbackURL string          `json:"callbackUrl"`
	Status      string          `json:"status"`
	Result      json.RawMessage `json:"result"`
	StartedAt   *time.Time      `json:"startedAt"`
	FinishedAt  *time.Time      `json:"finishedAt"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// ExecutionLog 表示任务执行过程中产生的日志。
type ExecutionLog struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"taskId"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

// ExecutionRunDetail 返回执行批次详情，包含任务列表。
type ExecutionRunDetail struct {
	ExecutionRun
	Tasks []ExecutionTask `json:"tasks"`
}

// ExecutionRunRequest 创建执行批次请求。
type ExecutionRunRequest struct {
	RunType  string  `json:"runType"`
	CaseIDs  []int64 `json:"caseIds"`
	Headless *bool   `json:"headless"`
}

// ExecutionDebugRequest 表示页面步骤工作台提交的即时调试任务。
type ExecutionDebugRequest struct {
	URL            string           `json:"url"`
	Actions        []map[string]any `json:"actions"`
	TimeoutSeconds int              `json:"timeoutSeconds"`
	Headless       *bool            `json:"headless"`
}

// ExecutionDebugStart 返回调试任务的查询凭据。
type ExecutionDebugStart struct {
	TaskID     string `json:"taskId"`
	ExecutorID string `json:"executorId"`
	Status     string `json:"status"`
}

// ExecutionRunFilter 执行批次分页查询条件。
type ExecutionRunFilter struct {
	ID      string
	RunType string
	Status  string
}

// ExecutionCallbackRequest 执行器回调请求。
type ExecutionCallbackRequest struct {
	TaskID      string          `json:"taskId"`
	Type        string          `json:"type"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	CallbackURL string          `json:"callbackUrl"`
	Result      json.RawMessage `json:"result"`
}

// ExecutionTaskResult 执行器返回的任务结果。
type ExecutionTaskResult struct {
	ExitCode  int      `json:"exitCode"`
	Output    string   `json:"output"`
	Error     string   `json:"error"`
	Artifacts []string `json:"artifacts"`
}

// ExecutionSummary 执行批次结果摘要。
type ExecutionSummary struct {
	Total   int64 `json:"total"`
	Passed  int64 `json:"passed"`
	Failed  int64 `json:"failed"`
	Skipped int64 `json:"skipped"`
	Waiting int64 `json:"waiting"`
	Active  int64 `json:"active"`
}

type ExecutionTrendPoint struct {
	Date     string  `json:"date"`
	Runs     int64   `json:"runs"`
	Cases    int64   `json:"cases"`
	PassRate float64 `json:"passRate"`
}

type ExecutionStatistics struct {
	TotalRuns      int64                 `json:"totalRuns"`
	TotalCases     int64                 `json:"totalCases"`
	PassedCases    int64                 `json:"passedCases"`
	FailedCases    int64                 `json:"failedCases"`
	FailedRuns     int64                 `json:"failedRuns"`
	RunningRuns    int64                 `json:"runningRuns"`
	PassRate       float64               `json:"passRate"`
	RunChange      float64               `json:"runChange"`
	CaseChange     float64               `json:"caseChange"`
	PassRateChange float64               `json:"passRateChange"`
	Trend          []ExecutionTrendPoint `json:"trend"`
}

// ExecutionLogCreateRequest 创建执行日志请求。
type ExecutionLogCreateRequest struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// ExecutionTaskFilter 任务查询条件。
type ExecutionTaskFilter struct {
	RunID      string
	ExecutorID string
	Status     string
}
