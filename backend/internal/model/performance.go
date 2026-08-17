package model

import (
	"encoding/json"
	"time"
)

// 性能测试运行状态机（SPEC §3.3）。
const (
	PerfRunPending         = "pending"
	PerfRunQueued          = "queued"
	PerfRunDispatching     = "dispatching"
	PerfRunDispatched      = "dispatched"
	PerfRunRunning         = "running"
	PerfRunStopping        = "stopping"
	PerfRunCompleted       = "completed"
	PerfRunThresholdFailed = "threshold_failed"
	PerfRunExecutionFailed = "execution_failed"
	PerfRunTimedOut        = "timed_out"
	PerfRunCanceled        = "canceled"
)

// 失败阶段枚举（SPEC §3.2）。
const (
	PerfFailureDispatch         = "dispatch"
	PerfFailureStartup          = "startup"
	PerfFailureScriptGeneration = "script_generation"
	PerfFailureK6Runtime        = "k6_runtime"
	PerfFailureCallback         = "callback"
	PerfFailureTimeout          = "timeout"
	PerfFailureExecutorOffline  = "executor_offline"
	PerfFailureCancel           = "cancel"
)

// PerfThreshold 是平台结构化的阈值模型（SPEC §3.1）。
type PerfThreshold struct {
	Metric         string  `json:"metric"`
	Aggregation    string  `json:"aggregation"`
	Operator       string  `json:"operator"`
	Value          float64 `json:"value"`
	Unit           string  `json:"unit"`
	AbortOnFail    bool    `json:"abortOnFail"`
	DelayAbortEval string  `json:"delayAbortEval"`
}

type PerfTestPlan struct {
	ID           int64           `json:"id"`
	ProductID    int64           `json:"productId"`
	ProductName  string          `json:"productName"`
	Name         string          `json:"name"`
	TargetURL    string          `json:"targetUrl"`
	Method       string          `json:"method"`
	Headers      json.RawMessage `json:"headers"`
	Body         string          `json:"body"`
	ScenarioType string          `json:"scenarioType"`
	LoadConfig   json.RawMessage `json:"loadConfig"`
	Environment  string          `json:"environment"`
	Thresholds   []PerfThreshold `json:"thresholds"`
	Status       string          `json:"status"`
	Priority     string          `json:"priority"`
	Owner        string          `json:"owner"`
	Tags         string          `json:"tags"`
	Description  string          `json:"description"`
	CreatedBy    string          `json:"createdBy"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

type PerfTestPlanRequest struct {
	ProductID    int64           `json:"productId"`
	Name         string          `json:"name"`
	TargetURL    string          `json:"targetUrl"`
	Method       string          `json:"method"`
	Headers      json.RawMessage `json:"headers"`
	Body         string          `json:"body"`
	ScenarioType string          `json:"scenarioType"`
	LoadConfig   json.RawMessage `json:"loadConfig"`
	Environment  string          `json:"environment"`
	Thresholds   []PerfThreshold `json:"thresholds"`
	Status       string          `json:"status"`
	Priority     string          `json:"priority"`
	Owner        string          `json:"owner"`
	Tags         string          `json:"tags"`
	Description  string          `json:"description"`
}

type PerfTestPlanFilter struct {
	ID           string
	Name         string
	ProductID    string
	ScenarioType string
	Environment  string
	Priority     string
	Status       string
	Owner        string
}

// PerfRunRequest 是触发执行的入参，幂等键由前端生成且永久唯一。
type PerfRunRequest struct {
	IdempotencyKey string `json:"idempotencyKey"`
}

// PerfSmokeRequest 是冒烟测试请求入参（SPEC §8.5），单请求、无并发/时长。
type PerfSmokeRequest struct {
	ExecutorID string          `json:"executorId"`
	TargetURL  string          `json:"targetUrl"`
	Method     string          `json:"method"`
	Headers    json.RawMessage `json:"headers"`
	Body       string          `json:"body"`
}

// PerfRunResult 是执行器终态回调回填的指标与诊断信息。
type PerfRunResult struct {
	ScriptHash       string
	GeneratorVersion string
	K6Version        string
	TotalRequests    int
	AvgDurationMs    *float64
	P95DurationMs    *float64
	ErrorRate        *float64
	RPS              *float64
	Summary          json.RawMessage
	ExitCode         *int
	DurationMs       *int
	ErrorMessage     string
	FailureStage     string
	DiagnosticOutput string
	NeedsAttention   bool
}

// PerfCallbackRequest 是执行器回调请求（SPEC §4.2）。
type PerfCallbackRequest struct {
	Status           string          `json:"status"`
	ScriptHash       string          `json:"scriptHash"`
	GeneratorVersion string          `json:"generatorVersion"`
	K6Version        string          `json:"k6Version"`
	ExitCode         *int            `json:"exitCode"`
	DurationMs       *int            `json:"durationMs"`
	TotalRequests    int             `json:"totalRequests"`
	AvgDurationMs    *float64        `json:"avgDurationMs"`
	P95DurationMs    *float64        `json:"p95DurationMs"`
	ErrorRate        *float64        `json:"errorRate"`
	RPS              *float64        `json:"rps"`
	Summary          json.RawMessage `json:"summary"`
	ErrorMessage     string          `json:"errorMessage"`
	FailureStage     string          `json:"failureStage"`
	DiagnosticOutput string          `json:"diagnosticOutput"`
	NeedsAttention   bool            `json:"needsAttention"`
}

type PerfTestRun struct {
	ID                 int64           `json:"id"`
	PlanID             int64           `json:"planId"`
	PlanName           string          `json:"planName"`
	ScenarioType       string          `json:"scenarioType"`
	Status             string          `json:"status"`
	TriggeredBy        string          `json:"triggeredBy"`
	PlanSnapshot       json.RawMessage `json:"planSnapshot"`
	ConfigHash         string          `json:"configHash"`
	ExecutorID         string          `json:"executorId"`
	ExecutorName       string          `json:"executorName"`
	K6Version          string          `json:"k6Version"`
	Environment        string          `json:"environment"`
	RequestedAt        *time.Time      `json:"requestedAt"`
	DispatchedAt       *time.Time      `json:"dispatchedAt"`
	DispatchDeadlineAt *time.Time      `json:"dispatchDeadlineAt"`
	StartDeadlineAt    *time.Time      `json:"startDeadlineAt"`
	ExpectedFinishAt   *time.Time      `json:"expectedFinishAt"`
	ScriptHash         string          `json:"scriptHash"`
	GeneratorVersion   string          `json:"generatorVersion"`
	TaskID             string          `json:"taskId"`
	CallbackTokenHash  string          `json:"-"`
	IdempotencyKey     string          `json:"idempotencyKey"`
	ExitCode           *int            `json:"exitCode"`
	DurationMs         *int            `json:"durationMs"`
	TotalRequests      int             `json:"totalRequests"`
	AvgDurationMs      *float64        `json:"avgDurationMs"`
	P95DurationMs      *float64        `json:"p95DurationMs"`
	ErrorRate          *float64        `json:"errorRate"`
	RPS                *float64        `json:"rps"`
	ErrorMessage       string          `json:"errorMessage"`
	FailureStage       string          `json:"failureStage"`
	DiagnosticOutput   string          `json:"diagnosticOutput"`
	NeedsAttention     bool            `json:"needsAttention"`
	Summary            json.RawMessage `json:"summary"`
	Series             json.RawMessage `json:"series"`
	StartedAt          *time.Time      `json:"startedAt"`
	FinishedAt         *time.Time      `json:"finishedAt"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

type PerfTestRunFilter struct {
	ID           string
	PlanID       string
	ScenarioType string
	Status       string
	Environment  string
}
