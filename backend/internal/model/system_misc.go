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
