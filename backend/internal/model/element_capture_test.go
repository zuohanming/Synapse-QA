package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestElementCaptureModelsExposeRecoveryBrowserAndCandidateReviewData(t *testing.T) {
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(struct {
		Session   ElementCaptureSession       `json:"session"`
		Request   CaptureSessionCreateRequest `json:"request"`
		Candidate ElementCaptureCandidate     `json:"candidate"`
	}{
		Session: ElementCaptureSession{
			TokenHash:         "sha256",
			BrowserChannel:    "chrome",
			LastHeartbeatAt:   now,
			InterruptedAt:     &now,
			RecoveryExpiresAt: &now,
		},
		Request: CaptureSessionCreateRequest{
			URL:            "https://example.test/login",
			BrowserChannel: "msedge",
		},
		Candidate: ElementCaptureCandidate{
			Name:               "提交",
			CaptureURL:         "https://example.test/login",
			DuplicateElementID: 8,
			ConflictTargets: []ElementCaptureTarget{
				{ID: 8, Name: "提交按钮"},
				{ID: 9, Name: "提交副本"},
			},
			ConflictStatus:     "pending",
			ConflictResolution: "replace",
		},
	})
	if err != nil {
		t.Fatalf("序列化采集模型失败：%v", err)
	}
	for _, want := range []string{
		`"browserChannel":"chrome"`,
		`"url":"https://example.test/login"`,
		`"name":"提交"`,
		`"captureUrl":"https://example.test/login"`,
		`"duplicateElementId":8`,
		`"conflictTargets":[{"id":8,"name":"提交按钮"},{"id":9,"name":"提交副本"}]`,
		`"conflictStatus":"pending"`,
		`"conflictResolution":"replace"`,
	} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("采集模型 JSON 缺少 %s：%s", want, payload)
		}
	}
}
