package model

import (
	"encoding/json"
	"time"
)

type TestCase struct {
	ID             int64     `json:"id"`
	ProductID      int64     `json:"productId"`
	ProductName    string    `json:"productName"`
	ModuleID       int64     `json:"moduleId"`
	ModuleName     string    `json:"moduleName"`
	PageID         int64     `json:"pageId"`
	PageName       string    `json:"pageName"`
	Name           string    `json:"name"`
	CaseType       string    `json:"caseType"`
	Priority       string    `json:"priority"`
	Status         string    `json:"status"`
	Owner          string    `json:"owner"`
	Tags           string    `json:"tags"`
	Description    string    `json:"description"`
	Preconditions  string    `json:"preconditions"`
	ExpectedResult string    `json:"expectedResult"`
	DataEnabled    bool      `json:"dataEnabled"`
	CreatedBy      string    `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type TestCaseStep struct {
	ID        int64     `json:"id"`
	CaseID    int64     `json:"caseId"`
	StepID    int64     `json:"stepId"`
	StepName  string    `json:"stepName"`
	SortOrder int       `json:"sortOrder"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

type TestCaseDataset struct {
	ID        int64           `json:"id"`
	CaseID    int64           `json:"caseId"`
	Name      string          `json:"name"`
	Variables json.RawMessage `json:"variables"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type TestCaseDetail struct {
	TestCase
	Steps    []TestCaseStep    `json:"steps"`
	Datasets []TestCaseDataset `json:"datasets"`
}

type TestCaseRequest struct {
	ProductID      int64   `json:"productId"`
	ModuleID       int64   `json:"moduleId"`
	PageID         int64   `json:"pageId"`
	Name           string  `json:"name"`
	CaseType       string  `json:"caseType"`
	Priority       string  `json:"priority"`
	Status         string  `json:"status"`
	Owner          string  `json:"owner"`
	Tags           string  `json:"tags"`
	Description    string  `json:"description"`
	Preconditions  string  `json:"preconditions"`
	ExpectedResult string  `json:"expectedResult"`
	DataEnabled    bool    `json:"dataEnabled"`
	StepIDs        []int64 `json:"stepIds"`
}

type TestCaseDatasetRequest struct {
	Name      string          `json:"name"`
	Variables json.RawMessage `json:"variables"`
	Enabled   bool            `json:"enabled"`
}

type TestCaseImportRequest struct {
	Items []TestCaseRequest `json:"items"`
}

type TestCaseFilter struct {
	ID        string
	Name      string
	ProductID string
	ModuleID  string
	CaseType  string
	Priority  string
	Status    string
	Owner     string
}
