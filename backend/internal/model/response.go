package model

// APIResponse 是所有 HTTP 接口统一返回结构。
type APIResponse struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// PageResult 是列表接口的统一分页结构。
type PageResult struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}
