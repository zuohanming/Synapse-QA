package model

import (
	"encoding/json"
	"time"
)

type ElementCaptureSession struct {
	ID               string    `json:"id"`
	PageID           int64     `json:"pageId"`
	ExecutorID       string    `json:"executorId"`
	BrowserContextID string    `json:"browserContextId"`
	CreatedBy        string    `json:"createdBy"`
	Status           string    `json:"status"`
	Mode             string    `json:"mode"`
	CurrentURL       string    `json:"currentUrl"`
	CandidateCount   int       `json:"candidateCount"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

type ElementCaptureCandidate struct {
	ID             string          `json:"id"`
	SessionID      string          `json:"sessionId"`
	Fingerprint    string          `json:"fingerprint"`
	TagName        string          `json:"tagName"`
	AccessibleName string          `json:"accessibleName"`
	Locators       json.RawMessage `json:"locators"`
	QualityScore   float64         `json:"qualityScore"`
	Status         string          `json:"status"`
	ExpiresAt      time.Time       `json:"expiresAt"`
}

type PageElementVersion struct {
	ID            int64           `json:"id"`
	PageElementID int64           `json:"pageElementId"`
	Version       int             `json:"version"`
	Snapshot      json.RawMessage `json:"snapshot"`
	ChangeSummary string          `json:"changeSummary"`
	CreatedBy     string          `json:"createdBy"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type CaptureSessionCreateRequest struct {
	PageID           int64  `json:"pageId"`
	ExecutorID       string `json:"executorId"`
	BrowserContextID string `json:"browserContextId"`
	Mode             string `json:"mode"`
	CurrentURL       string `json:"currentUrl"`
}

type CandidateSaveItem struct {
	CandidateID string `json:"candidateId"`
	Name        string `json:"name"`
}

type CandidateBatchSaveRequest struct {
	SessionID string              `json:"sessionId"`
	Items     []CandidateSaveItem `json:"items"`
}
