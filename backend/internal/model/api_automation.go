package model

import (
	"encoding/json"
	"time"
)

type APIInterface struct {
	ID                  int64           `json:"id"`
	ProductID           int64           `json:"productId"`
	ProjectID           int64           `json:"projectId"`
	ProjectName         string          `json:"projectName"`
	ProductName         string          `json:"productName"`
	ModuleID            int64           `json:"moduleId"`
	ModuleName          string          `json:"moduleName"`
	Name                string          `json:"name"`
	Method              string          `json:"method"`
	Path                string          `json:"path"`
	Protocol            string          `json:"protocol"`
	EndpointType        string          `json:"endpointType"`
	LifecycleStatus     string          `json:"lifecycleStatus"`
	TimeoutSeconds      int             `json:"timeoutSeconds"`
	FollowRedirects     bool            `json:"followRedirects"`
	Configuration       json.RawMessage `json:"configuration"`
	Revision            int64           `json:"revision"`
	LastDebugStatus     string          `json:"lastDebugStatus"`
	LastDebugDurationMS *int64          `json:"lastDebugDurationMs"`
	LastDebugAt         *time.Time      `json:"lastDebugAt"`
	CreatedBy           string          `json:"createdBy"`
	UpdatedBy           string          `json:"updatedBy"`
	CreatedAt           time.Time       `json:"createdAt"`
	UpdatedAt           time.Time       `json:"updatedAt"`
}

type APIInterfaceRequest struct {
	ProductID       int64           `json:"productId"`
	ModuleID        int64           `json:"moduleId"`
	Name            string          `json:"name"`
	Method          string          `json:"method"`
	Path            string          `json:"path"`
	Protocol        string          `json:"protocol"`
	EndpointType    string          `json:"endpointType"`
	LifecycleStatus string          `json:"lifecycleStatus"`
	TimeoutSeconds  int             `json:"timeoutSeconds"`
	FollowRedirects *bool           `json:"followRedirects"`
	Configuration   json.RawMessage `json:"configuration"`
	Revision        int64           `json:"revision"`
}

type APIInterfaceFilter struct {
	ProjectID       string
	ProductID       string
	ModuleID        string
	Keyword         string
	Method          string
	LifecycleStatus string
}

type APIProjectHeader struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"projectId"`
	ProjectName string    `json:"projectName"`
	Name        string    `json:"name"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	Sensitive   bool      `json:"sensitive"`
	CreatedBy   string    `json:"createdBy"`
	UpdatedBy   string    `json:"updatedBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type APIProjectHeaderRequest struct {
	ProjectID   int64  `json:"projectId"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
	Sensitive   bool   `json:"sensitive"`
}

type APIRequestPreviewRequest struct {
	TestObjectID      int64                  `json:"testObjectId"`
	Snapshot          *APIInterfaceRequest   `json:"snapshot"`
	TemporaryVariables map[string]any         `json:"temporaryVariables"`
	TemporaryHeaders   map[string]string      `json:"temporaryHeaders"`
}

type APIRequestPreview struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers"`
	Body     string            `json:"body"`
	Warnings []string          `json:"warnings"`
}
