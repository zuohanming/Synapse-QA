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
	ID                   int64           `json:"id"`
	ProductID            int64           `json:"productId"`
	ProductName          string          `json:"productName"`
	Name                 string          `json:"name"`
	TargetURL            string          `json:"targetUrl"`
	Method               string          `json:"method"`
	Headers              json.RawMessage `json:"headers"`
	Body                 string          `json:"body"`
	ScenarioType         string          `json:"scenarioType"`
	LoadConfig           json.RawMessage `json:"loadConfig"`
	Environment          string          `json:"environment"`
	EnvironmentID        *int64          `json:"environmentId"`
	EnvironmentName      string          `json:"environmentName"`
	EnvironmentBaseURL   string          `json:"environmentBaseUrl"`
	EnvironmentDeployEnv string          `json:"environmentDeployEnv"`
	Thresholds           []PerfThreshold `json:"thresholds"`
	Status               string          `json:"status"`
	Priority             string          `json:"priority"`
	Owner                string          `json:"owner"`
	Tags                 string          `json:"tags"`
	Description          string          `json:"description"`
	CreatedBy            string          `json:"createdBy"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
}

type PerfTestPlanRequest struct {
	ProductID     int64           `json:"productId"`
	Name          string          `json:"name"`
	TargetURL     string          `json:"targetUrl"`
	Method        string          `json:"method"`
	Headers       json.RawMessage `json:"headers"`
	Body          string          `json:"body"`
	ScenarioType  string          `json:"scenarioType"`
	LoadConfig    json.RawMessage `json:"loadConfig"`
	Environment   string          `json:"environment"`
	EnvironmentID *int64          `json:"environmentId"`
	Thresholds    []PerfThreshold `json:"thresholds"`
	Status        string          `json:"status"`
	Priority      string          `json:"priority"`
	Owner         string          `json:"owner"`
	Tags          string          `json:"tags"`
	Description   string          `json:"description"`
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
	EnvironmentID  *int64 `json:"environmentId"`
}

type PerfEnvironment struct {
	EnvironmentID int64  `json:"environmentId"`
	ProductID     int64  `json:"productId"`
	EnvName       string `json:"envName"`
	BaseURL       string `json:"baseUrl"`
	DeployEnv     string `json:"deployEnv"`
	QueryEnabled  bool   `json:"queryEnabled"`
	WriteEnabled  bool   `json:"writeEnabled"`
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
	P99DurationMs    *float64
	ErrorRate        *float64
	RPS              *float64
	Summary          json.RawMessage
	ExitCode         *int
	DurationMs       *int
	ErrorMessage     string
	FailureStage     string
	DiagnosticOutput string
	NeedsAttention   bool
	Series           json.RawMessage
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
	P99DurationMs    *float64        `json:"p99DurationMs"`
	ErrorRate        *float64        `json:"errorRate"`
	RPS              *float64        `json:"rps"`
	Summary          json.RawMessage `json:"summary"`
	ErrorMessage     string          `json:"errorMessage"`
	FailureStage     string          `json:"failureStage"`
	DiagnosticOutput string          `json:"diagnosticOutput"`
	NeedsAttention   bool            `json:"needsAttention"`
	Series           json.RawMessage `json:"series"`
}

type PerfSampleEvent struct {
	TaskID        string                `json:"taskId"`
	Sequence      int64                 `json:"sequence"`
	Timestamp     time.Time             `json:"timestamp"`
	Type          string                `json:"type"`
	Status        string                `json:"status"`
	Stage         string                `json:"stage"`
	RemainingMs   int64                 `json:"remainingMs"`
	WindowMs      int64                 `json:"windowMs"`
	VUs           int                   `json:"vus"`
	TotalRequests int                   `json:"totalRequests"`
	RPS           *float64              `json:"rps"`
	P50           *float64              `json:"p50"`
	P90           *float64              `json:"p90"`
	P95           *float64              `json:"p95"`
	P99           *float64              `json:"p99"`
	ErrorRate     *float64              `json:"errorRate"`
	StatusCodes   map[string]int        `json:"statusCodes,omitempty"`
	ErrorTopN     []PerfErrorTop        `json:"errorTopN,omitempty"`
	Thresholds    []PerfSampleThreshold `json:"thresholds,omitempty"`
	Message       string                `json:"message,omitempty"`
}

type PerfErrorTop struct {
	Key     string `json:"key"`
	Message string `json:"message"`
	Count   int    `json:"count"`
}

type PerfSampleThreshold struct {
	Metric     string   `json:"metric"`
	Expression string   `json:"expression"`
	Actual     *float64 `json:"actual"`
	Status     string   `json:"status"`
}

type PerfSeries struct {
	Version      int                   `json:"version"`
	IntervalMs   int                   `json:"intervalMs"`
	LastSequence int64                 `json:"lastSequence"`
	StartedAt    string                `json:"startedAt"`
	EndedAt      string                `json:"endedAt"`
	Partial      bool                  `json:"partial"`
	Points       []PerfSampleEvent     `json:"points"`
	StatusCodes  map[string]int        `json:"statusCodes"`
	ErrorTopN    []PerfErrorTop        `json:"errorTopN"`
	Thresholds   []PerfSampleThreshold `json:"thresholds"`
}

type PerfTestRun struct {
	ID                   int64           `json:"id"`
	PlanID               int64           `json:"planId"`
	PlanName             string          `json:"planName"`
	ScenarioType         string          `json:"scenarioType"`
	Status               string          `json:"status"`
	TriggeredBy          string          `json:"triggeredBy"`
	PlanSnapshot         json.RawMessage `json:"planSnapshot"`
	ConfigHash           string          `json:"configHash"`
	ExecutorID           string          `json:"executorId"`
	ExecutorName         string          `json:"executorName"`
	K6Version            string          `json:"k6Version"`
	Environment          string          `json:"environment"`
	EnvironmentID        *int64          `json:"environmentId"`
	EnvironmentName      string          `json:"environmentName"`
	EnvironmentBaseURL   string          `json:"environmentBaseUrl"`
	EnvironmentDeployEnv string          `json:"environmentDeployEnv"`
	RequestedAt          *time.Time      `json:"requestedAt"`
	DispatchedAt         *time.Time      `json:"dispatchedAt"`
	DispatchDeadlineAt   *time.Time      `json:"dispatchDeadlineAt"`
	StartDeadlineAt      *time.Time      `json:"startDeadlineAt"`
	ExpectedFinishAt     *time.Time      `json:"expectedFinishAt"`
	ScriptHash           string          `json:"scriptHash"`
	GeneratorVersion     string          `json:"generatorVersion"`
	TaskID               string          `json:"taskId"`
	CallbackTokenHash    string          `json:"-"`
	IdempotencyKey       string          `json:"idempotencyKey"`
	ExitCode             *int            `json:"exitCode"`
	DurationMs           *int            `json:"durationMs"`
	TotalRequests        int             `json:"totalRequests"`
	AvgDurationMs        *float64        `json:"avgDurationMs"`
	P95DurationMs        *float64        `json:"p95DurationMs"`
	P99DurationMs        *float64        `json:"p99DurationMs"`
	ErrorRate            *float64        `json:"errorRate"`
	RPS                  *float64        `json:"rps"`
	ErrorMessage         string          `json:"errorMessage"`
	FailureStage         string          `json:"failureStage"`
	DiagnosticOutput     string          `json:"diagnosticOutput"`
	NeedsAttention       bool            `json:"needsAttention"`
	Summary              json.RawMessage `json:"summary"`
	Series               json.RawMessage `json:"series"`
	StartedAt            *time.Time      `json:"startedAt"`
	FinishedAt           *time.Time      `json:"finishedAt"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
	DegradationCheckedAt *time.Time      `json:"-"`
}

type PerfTestRunFilter struct {
	ID           string
	PlanID       string
	ScenarioType string
	Status       string
	Environment  string
}

type PerfBaseline struct {
	ID            int64     `json:"id"`
	PlanID        int64     `json:"planId"`
	ScenarioType  string    `json:"scenarioType"`
	EnvironmentID *int64    `json:"environmentId"`
	Environment   string    `json:"environment"`
	RunID         int64     `json:"runId"`
	SetBy         string    `json:"setBy"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type PerfBaselineResponse struct {
	Exists       bool          `json:"exists"`
	IsCurrentRun bool          `json:"isCurrentRun"`
	Baseline     *PerfBaseline `json:"baseline"`
}

type PerfComparisonMetric struct {
	Direction       string   `json:"direction"`
	Baseline        *float64 `json:"baseline"`
	Current         *float64 `json:"current"`
	Delta           *float64 `json:"delta"`
	ChangeRate      *float64 `json:"changeRate"`
	DegradationRate *float64 `json:"degradationRate"`
	Comparable      bool     `json:"comparable"`
	RateAvailable   bool     `json:"rateAvailable"`
	Degraded        bool     `json:"degraded"`
	Reason          string   `json:"reason"`
}

type PerfComparison struct {
	CurrentRunID    int64                 `json:"currentRunId"`
	BaselineRunID   *int64                `json:"baselineRunId"`
	Threshold       float64               `json:"threshold"`
	Comparable      bool                  `json:"comparable"`
	Degraded        bool                  `json:"degraded"`
	DegradedMetrics []string              `json:"degradedMetrics"`
	Metrics         PerfComparisonMetrics `json:"metrics"`
}

type PerfComparisonMetrics struct {
	P95DurationMs PerfComparisonMetric `json:"p95DurationMs"`
	P99DurationMs PerfComparisonMetric `json:"p99DurationMs"`
	ErrorRate     PerfComparisonMetric `json:"errorRate"`
	RPS           PerfComparisonMetric `json:"rps"`
}

type PerfTrendPoint struct {
	RunID           int64      `json:"runId"`
	FinishedAt      *time.Time `json:"finishedAt"`
	Status          string     `json:"status"`
	TotalRequests   int        `json:"totalRequests"`
	P95DurationMs   *float64   `json:"p95DurationMs"`
	P99DurationMs   *float64   `json:"p99DurationMs"`
	ErrorRate       *float64   `json:"errorRate"`
	RPS             *float64   `json:"rps"`
	IsBaseline      bool       `json:"isBaseline"`
	Degraded        bool       `json:"degraded"`
	DegradedMetrics []string   `json:"degradedMetrics"`
}

type PerfTrendResponse struct {
	PlanID               int64            `json:"planId"`
	ScenarioType         string           `json:"scenarioType"`
	EnvironmentID        *int64           `json:"environmentId"`
	Environment          string           `json:"environment"`
	EnvironmentName      string           `json:"environmentName"`
	EnvironmentBaseURL   string           `json:"environmentBaseUrl"`
	EnvironmentDeployEnv string           `json:"environmentDeployEnv"`
	BaselineRunID        *int64           `json:"baselineRunId"`
	Limit                int              `json:"limit"`
	Items                []PerfTrendPoint `json:"items"`
}

type PerfSchedule struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	PlanID           int64      `json:"planId"`
	PlanName         string     `json:"planName"`
	EnvironmentID    *int64     `json:"environmentId"`
	EnvironmentName  string     `json:"environmentName"`
	CronExpression   string     `json:"cronExpression"`
	Timezone         string     `json:"timezone"`
	Enabled          bool       `json:"enabled"`
	NextRunAt        *time.Time `json:"nextRunAt"`
	LastScheduledFor *time.Time `json:"lastScheduledFor"`
	LastTriggeredAt  *time.Time `json:"lastTriggeredAt"`
	LastRunID        *int64     `json:"lastRunId"`
	LastResult       string     `json:"lastResult"`
	LastError        string     `json:"lastError"`
	CreatedBy        string     `json:"createdBy"`
	UpdatedBy        string     `json:"updatedBy"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	DeletedAt        *time.Time `json:"-"`
	ClaimToken       string     `json:"-"`
	ClaimOwner       string     `json:"-"`
	ClaimUntil       *time.Time `json:"-"`
}

type PerfScheduleRequest struct {
	Name           string `json:"name"`
	PlanID         int64  `json:"planId"`
	EnvironmentID  *int64 `json:"environmentId"`
	CronExpression string `json:"cronExpression"`
	Timezone       string `json:"timezone"`
	Enabled        *bool  `json:"enabled"`
}

type PerfScheduleFilter struct {
	PlanID  string
	Enabled *bool
}

type PerfSchedulePatch struct {
	Name           *string `json:"name"`
	PlanID         *int64  `json:"planId"`
	EnvironmentID  **int64 `json:"environmentId"`
	CronExpression *string `json:"cronExpression"`
	Timezone       *string `json:"timezone"`
}
