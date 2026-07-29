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
	ID                     string          `json:"id"`
	CursorID               int64           `json:"cursorId"`
	SessionID              string          `json:"sessionId"`
	Name                   string          `json:"name"`
	Fingerprint            string          `json:"fingerprint"`
	CaptureURL             string          `json:"captureUrl"`
	TagName                string          `json:"tagName"`
	AccessibleName         string          `json:"accessibleName"`
	Locators               json.RawMessage `json:"locators"`
	QualityScore           float64         `json:"qualityScore"`
	DuplicateElementID     int64           `json:"duplicateElementId"`
	DuplicateElementPageID int64           `json:"-"`
	ConflictStatus         string          `json:"conflictStatus"`
	ConflictResolution     string          `json:"conflictResolution"`
	Status                 string          `json:"status"`
	ExpiresAt              time.Time       `json:"expiresAt"`
	CandidateCount         int             `json:"candidateCount,omitempty"`
	Warning                string          `json:"warning,omitempty"`
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

type ElementCaptureCommand struct {
	ID             int64  `json:"id"`
	SessionID      string `json:"sessionId"`
	Type           string `json:"type"`
	Mode           string `json:"mode,omitempty"`
	URL            string `json:"url,omitempty"`
	BrowserChannel string `json:"browserChannel,omitempty"`
	// Token 仅在领取 start 命令的响应 DTO 中出现，绝不持久化。
	Token   string `json:"token,omitempty"`
	Receipt string `json:"receipt,omitempty"`
}

type CaptureCommandAckRequest struct {
	Receipt string `json:"receipt"`
}

type CaptureHeartbeatRequest struct {
	ExecutorID       string `json:"executorId"`
	Token            string `json:"token"`
	BrowserContextID string `json:"browserContextId"`
	CurrentURL       string `json:"currentUrl"`
	CommandReceipt   string `json:"commandReceipt"`
}

type CaptureFailureRequest struct {
	ExecutorID string `json:"executorId"`
	Token      string `json:"token"`
	Reason     string `json:"reason"`
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
	CandidateID     int64  `json:"candidateId"`
	Resolution      string `json:"resolution"`
	TargetElementID int64  `json:"targetElementId,omitempty"`
}

type CandidateBatchSaveRequest struct {
	SessionID string              `json:"sessionId"`
	Items     []CandidateSaveItem `json:"items"`
}

type CaptureCandidateCreateRequest struct {
	ExecutorID     string          `json:"executorId"`
	Token          string          `json:"token"`
	SessionID      string          `json:"sessionId"`
	Name           string          `json:"name"`
	Fingerprint    string          `json:"fingerprint"`
	CaptureURL     string          `json:"captureUrl"`
	TagName        string          `json:"tagName"`
	AccessibleName string          `json:"accessibleName"`
	Locators       json.RawMessage `json:"locators"`
	QualityScore   float64         `json:"qualityScore"`
}

type CaptureCandidateUpdateRequest struct {
	Name               string          `json:"name"`
	Locators           json.RawMessage `json:"locators"`
	QualityScore       *float64        `json:"qualityScore"`
	ConflictResolution string          `json:"conflictResolution"`
}

type CandidateIssue struct {
	CandidateID int64  `json:"candidateId"`
	Field       string `json:"field"`
	Message     string `json:"message"`
}

type BatchSaveResult struct {
	SavedCandidateIDs   []int64 `json:"savedCandidateIds"`
	IgnoredCandidateIDs []int64 `json:"ignoredCandidateIds"`
}

type CaptureBatchData struct {
	Session              ElementCaptureSession
	Candidates           []ElementCaptureCandidate
	ExistingNames        map[string][]int64
	ExistingFingerprints map[string][]int64
}
