package model

import (
	"encoding/json"
	"time"
)

type APIGlobalVariable struct {
	ID          int64      `json:"id"`
	ScopeType   string     `json:"scopeType"`
	ProjectID   *int64     `json:"projectId"`
	ProductID   *int64     `json:"productId"`
	ProjectName string     `json:"projectName"`
	ProductName string     `json:"productName"`
	EnvName     string     `json:"envName"`
	Name        string     `json:"name"`
	ValueType   string     `json:"valueType"`
	Value       string     `json:"value"`
	Description string     `json:"description"`
	Enabled     bool       `json:"enabled"`
	Sensitive   bool       `json:"sensitive"`
	Revision    int64      `json:"revision"`
	CreatedBy   string     `json:"createdBy"`
	UpdatedBy   string     `json:"updatedBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	DeletedAt   *time.Time `json:"deletedAt"`
}

type APIGlobalVariableRequest struct {
	ScopeType   string `json:"scopeType"`
	ProjectID   int64  `json:"projectId"`
	ProductID   int64  `json:"productId"`
	EnvName     string `json:"envName"`
	Name        string `json:"name"`
	ValueType   string `json:"valueType"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
	Sensitive   bool   `json:"sensitive"`
	Revision    int64  `json:"revision"`
}

type APIGlobalVariableFilter struct {
	ScopeType string
	ProjectID string
	ProductID string
	EnvName   string
	Keyword   string
}

type APITestCase struct {
	ID             int64           `json:"id"`
	ProjectID      int64           `json:"projectId"`
	ProjectName    string          `json:"projectName"`
	ProductID      int64           `json:"productId"`
	ProductName    string          `json:"productName"`
	ModuleID       int64           `json:"moduleId"`
	ModuleName     string          `json:"moduleName"`
	Name           string          `json:"name"`
	Priority       string          `json:"priority"`
	Status         string          `json:"status"`
	Owner          string          `json:"owner"`
	Tags           []string        `json:"tags"`
	Draft          json.RawMessage `json:"draft"`
	Revision       int64           `json:"revision"`
	CurrentVersion int             `json:"currentVersion"`
	LastRunStatus  string          `json:"lastRunStatus"`
	LastRunAt      *time.Time      `json:"lastRunAt"`
	CreatedBy      string          `json:"createdBy"`
	UpdatedBy      string          `json:"updatedBy"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	DeletedAt      *time.Time      `json:"deletedAt"`
}

type APITestCaseRequest struct {
	ProjectID int64           `json:"projectId"`
	ProductID int64           `json:"productId"`
	ModuleID  int64           `json:"moduleId"`
	Name      string          `json:"name"`
	Priority  string          `json:"priority"`
	Status    string          `json:"status"`
	Owner     string          `json:"owner"`
	Tags      []string        `json:"tags"`
	Draft     json.RawMessage `json:"draft"`
	Revision  int64           `json:"revision"`
}

type APITestCaseFilter struct {
	ProjectID string
	ProductID string
	ModuleID  string
	Keyword   string
	Status    string
	Priority  string
	Owner     string
}

type APITestCaseVersion struct {
	ID            int64           `json:"id"`
	CaseID        int64           `json:"caseId"`
	Version       int             `json:"version"`
	Snapshot      json.RawMessage `json:"snapshot"`
	ChangeSummary string          `json:"changeSummary"`
	CreatedBy     string          `json:"createdBy"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type APITestStep struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Key              string          `json:"key"`
	InterfaceID      int64           `json:"interfaceId"`
	InterfaceVersion int             `json:"interfaceVersion"`
	Enabled          bool            `json:"enabled"`
	Phase            string          `json:"phase"`
	Condition        string          `json:"condition"`
	FailurePolicy    string          `json:"failurePolicy"`
	Overrides        json.RawMessage `json:"overrides"`
}

type APITestDataset struct {
	Name    string         `json:"name"`
	Enabled bool           `json:"enabled"`
	Values  map[string]any `json:"values"`
}

type APITestCaseDraft struct {
	Steps      []APITestStep    `json:"steps"`
	Datasets   []APITestDataset `json:"datasets"`
	Variables  []map[string]any `json:"variables"`
	DataSchema []map[string]any `json:"dataSchema"`
}

type APITestCasePublishRequest struct {
	Revision      int64  `json:"revision"`
	ChangeSummary string `json:"changeSummary"`
}

type APITestCaseValidation struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

type APITestRunStartRequest struct {
	CaseIDs            []int64        `json:"caseIds"`
	EnvName            string         `json:"envName"`
	Concurrency        int            `json:"concurrency"`
	ExecutorID         string         `json:"executorId"`
	RetryFailedOnce    bool           `json:"retryFailedOnce"`
	Remark             string         `json:"remark"`
	TemporaryVariables map[string]any `json:"temporaryVariables"`
}

type APITestRunBatch struct {
	ID                int64           `json:"id"`
	BatchID           string          `json:"batchId"`
	ProjectID         int64           `json:"projectId"`
	EnvName           string          `json:"envName"`
	Status            string          `json:"status"`
	TotalInstances    int             `json:"totalInstances"`
	QueuedInstances   int             `json:"queuedInstances"`
	RunningInstances  int             `json:"runningInstances"`
	PassedInstances   int             `json:"passedInstances"`
	FailedInstances   int             `json:"failedInstances"`
	CanceledInstances int             `json:"canceledInstances"`
	Options           json.RawMessage `json:"options"`
	TriggeredBy       string          `json:"triggeredBy"`
	StartedAt         *time.Time      `json:"startedAt"`
	FinishedAt        *time.Time      `json:"finishedAt"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type APITestRunInstance struct {
	ID           int64           `json:"id"`
	BatchID      string          `json:"batchId"`
	TaskID       string          `json:"taskId"`
	CaseID       int64           `json:"caseId"`
	CaseVersion  int             `json:"caseVersion"`
	DatasetIndex int             `json:"datasetIndex"`
	ExecutorID   string          `json:"executorId"`
	Status       string          `json:"status"`
	Snapshot     json.RawMessage `json:"snapshot"`
	Result       json.RawMessage `json:"result"`
	ErrorMessage string          `json:"errorMessage"`
	StartedAt    *time.Time      `json:"startedAt"`
	FinishedAt   *time.Time      `json:"finishedAt"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

type APITestRunCallbackRequest struct {
	TaskID string          `json:"taskId"`
	Status string          `json:"status"`
	Result json.RawMessage `json:"result"`
}
