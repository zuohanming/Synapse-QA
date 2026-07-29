package model

import (
	"encoding/json"
	"time"
)

type ElementCaptureSession struct {
	ID                string     `json:"id"`
	PageID            int64      `json:"pageId"`
	ExecutorID        string     `json:"executorId"`
	BrowserContextID  string     `json:"browserContextId"`
	BrowserChannel    string     `json:"browserChannel"`
	CreatedBy         string     `json:"createdBy"`
	Status            string     `json:"status"`
	Mode              string     `json:"mode"`
	CurrentURL        string     `json:"currentUrl"`
	TokenHash         string     `json:"-"`
	CandidateCount    int        `json:"candidateCount"`
	LastHeartbeatAt   time.Time  `json:"lastHeartbeatAt"`
	InterruptedAt     *time.Time `json:"interruptedAt"`
	RecoveryExpiresAt *time.Time `json:"recoveryExpiresAt"`
	ExpiresAt         time.Time  `json:"expiresAt"`
}

// CaptureSessionCreated 仅在创建会话时返回明文令牌。
type CaptureSessionCreated struct {
	Session ElementCaptureSession `json:"session"`
	Token   string                `json:"token"`
}

type ElementCaptureSessionDetail struct {
	ElementCaptureSession
}

type ElementCaptureCandidate struct {
	ID                 string          `json:"id"`
	SessionID          string          `json:"sessionId"`
	Name               string          `json:"name"`
	Fingerprint        string          `json:"fingerprint"`
	CaptureURL         string          `json:"captureUrl"`
	TagName            string          `json:"tagName"`
	AccessibleName     string          `json:"accessibleName"`
	Locators           json.RawMessage `json:"locators"`
	QualityScore       float64         `json:"qualityScore"`
	DuplicateElementID int64           `json:"duplicateElementId"`
	ConflictStatus     string          `json:"conflictStatus"`
	ConflictResolution string          `json:"conflictResolution"`
	Status             string          `json:"status"`
	ExpiresAt          time.Time       `json:"expiresAt"`
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
	BrowserChannel   string `json:"browserChannel"`
	Mode             string `json:"mode"`
	URL              string `json:"url"`
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
