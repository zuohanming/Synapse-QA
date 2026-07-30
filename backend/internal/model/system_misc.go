package model

import "time"

type Menu struct {
	ID        int64  `json:"id"`
	ParentID  *int64 `json:"parentId,omitempty"`
	Title     string `json:"title"`
	Code      string `json:"code"`
	SortOrder int    `json:"sortOrder"`
}

type Dictionary struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Code    string `json:"code"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type OperationLog struct {
	ID        int64     `json:"id"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
}

type SystemOverview struct {
	Users           int64                `json:"users"`
	ActiveUsers     int64                `json:"activeUsers"`
	Roles           int64                `json:"roles"`
	OnlineExecutors int64                `json:"onlineExecutors"`
	TotalExecutors  int64                `json:"totalExecutors"`
	RunsToday       int64                `json:"runsToday"`
	FailuresToday   int64                `json:"failuresToday"`
	RecentLogs      []OperationLog       `json:"recentLogs"`
	RunTrend        []SystemRunTrendItem `json:"runTrend"`
}

type SystemRunTrendItem struct {
	Date   string `json:"date"`
	Total  int64  `json:"total"`
	Failed int64  `json:"failed"`
}

type OperationLogFilter struct {
	Actor    string
	Action   string
	Keyword  string
	DateFrom string
	DateTo   string
	Page     int
	PageSize int
}
