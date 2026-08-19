package model

import "time"

// DashboardOverview 是首页聚合接口的固定响应 DTO。
type DashboardOverview struct {
	GeneratedAt  time.Time             `json:"generatedAt"`
	Filter       DashboardFilter       `json:"filter"`
	Projects     []DashboardProject    `json:"projects"`
	Capabilities DashboardCapabilities `json:"capabilities"`
	Executions   DashboardExecutions   `json:"executions"`
	Attention    DashboardAttention    `json:"attention"`
}

type DashboardFilter struct {
	ProjectID *int64    `json:"projectId"`
	Range     string    `json:"range"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
}

type DashboardProject struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type DashboardCapabilities struct {
	UIRuns   bool `json:"uiRuns"`
	APIRuns  bool `json:"apiRuns"`
	PerfRuns bool `json:"perfRuns"`
}

type DashboardExecutions struct {
	State        string                   `json:"state"`
	SourceStates DashboardSourceStates    `json:"sourceStates"`
	Counts       DashboardExecutionCounts `json:"counts"`
	Recent       []DashboardRecentItem    `json:"recent"`
}

type DashboardSourceStates struct {
	UI   string `json:"ui"`
	API  string `json:"api"`
	Perf string `json:"perf"`
}

type DashboardExecutionCounts struct {
	Total    int64 `json:"total"`
	Success  int64 `json:"success"`
	Failed   int64 `json:"failed"`
	Running  int64 `json:"running"`
	Canceled int64 `json:"canceled"`
	Unknown  int64 `json:"unknown"`
}

type DashboardRecentItem struct {
	UID         string     `json:"uid"`
	Type        string     `json:"type"`
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	ProjectID   *int64     `json:"projectId"`
	ProjectName string     `json:"projectName"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
	DurationMS  *int64     `json:"durationMs"`
	TargetURL   string     `json:"targetUrl"`
}

type DashboardAttention struct {
	State string                   `json:"state"`
	Total int64                    `json:"total"`
	Items []DashboardAttentionItem `json:"items"`
}

type DashboardAttentionItem struct {
	UID         string    `json:"uid"`
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Source      string    `json:"source"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	ProjectID   *int64    `json:"projectId"`
	ProjectName string    `json:"projectName"`
	CreatedAt   time.Time `json:"createdAt"`
	TargetURL   string    `json:"targetUrl"`
}

// DashboardExecutionRecord 是三个执行来源共用的仓储内部投影。
// 它不直接作为 HTTP 响应返回，状态统一由 DashboardService 映射。
type DashboardExecutionRecord struct {
	Type        string
	ID          string
	Title       string
	Description string
	ProjectID   *int64
	ProjectName string
	Status      string
	CreatedAt   time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
	DurationMS  *int64
}

// DashboardSourceData 是单个执行来源的数据库聚合结果。
// Recent 和 Failed 都由仓储层限制数量，避免把整个时间范围载入 Go。
type DashboardSourceData struct {
	Counts DashboardExecutionCounts
	Recent []DashboardExecutionRecord
	Failed []DashboardExecutionRecord
}
