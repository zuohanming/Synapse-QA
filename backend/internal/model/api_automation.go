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
	TestObjectID       int64                `json:"testObjectId"`
	Snapshot           *APIInterfaceRequest `json:"snapshot"`
	TemporaryVariables map[string]any       `json:"temporaryVariables"`
	TemporaryHeaders   map[string]string    `json:"temporaryHeaders"`
}

type APIRequestPreview struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers"`
	Body     string            `json:"body"`
	Warnings []string          `json:"warnings"`
}

type APICurlParseRequest struct {
	Curl string `json:"curl"`
}

type APICurlParseResult struct {
	Method        string            `json:"method"`
	URL           string            `json:"url"`
	Protocol      string            `json:"protocol"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	MaskedHeaders []string          `json:"maskedHeaders"`
}

type APICurlExportResult struct {
	Curl string `json:"curl"`
}

type APITempFile struct {
	ID           string    `json:"id"`
	ProjectID    int64     `json:"projectId"`
	OriginalName string    `json:"originalName"`
	MIMEType     string    `json:"mimeType"`
	SizeBytes    int64     `json:"sizeBytes"`
	SHA256       string    `json:"sha256"`
	ExpiresAt    time.Time `json:"expiresAt"`
	CreatedAt    time.Time `json:"createdAt"`
}

type APIDebugStartRequest struct {
	TestObjectID       int64                `json:"testObjectId"`
	Snapshot           *APIInterfaceRequest `json:"snapshot"`
	TemporaryVariables map[string]any       `json:"temporaryVariables"`
	TemporaryHeaders   map[string]string    `json:"temporaryHeaders"`
}

type APIDebugRun struct {
	ID           int64           `json:"id"`
	TaskID       string          `json:"taskId"`
	InterfaceID  int64           `json:"interfaceId"`
	ProjectID    int64           `json:"projectId"`
	ExecutorID   string          `json:"executorId"`
	Status       string          `json:"status"`
	Request      json.RawMessage `json:"request"`
	Result       json.RawMessage `json:"result"`
	ErrorMessage string          `json:"errorMessage"`
	TriggeredBy  string          `json:"triggeredBy"`
	StartedAt    *time.Time      `json:"startedAt"`
	FinishedAt   *time.Time      `json:"finishedAt"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

type APIDebugEvent struct {
	ID        int64           `json:"id"`
	TaskID    string          `json:"taskId"`
	Sequence  int             `json:"sequence"`
	Type      string          `json:"type"`
	Stage     string          `json:"stage"`
	Status    string          `json:"status"`
	Message   string          `json:"message"`
	Progress  int             `json:"progress"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"timestamp"`
}

type APIDebugCallbackRequest struct {
	TaskID string          `json:"taskId"`
	Status string          `json:"status"`
	Result json.RawMessage `json:"result"`
}
