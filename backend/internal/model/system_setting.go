package model

import (
	"encoding/json"
	"time"
)

type SystemSettingGroup struct {
	GroupKey  string          `json:"groupKey"`
	Value     json.RawMessage `json:"value"`
	Revision  int64           `json:"revision"`
	UpdatedBy string          `json:"updatedBy"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type SystemSettingUpdateRequest struct {
	Value         json.RawMessage `json:"value"`
	Revision      int64           `json:"revision"`
	ChangeSummary string          `json:"changeSummary"`
}

type SystemSettingHistory struct {
	ID            int64           `json:"id"`
	GroupKey      string          `json:"groupKey"`
	Revision      int64           `json:"revision"`
	Value         json.RawMessage `json:"value"`
	ChangeSummary string          `json:"changeSummary"`
	CreatedBy     string          `json:"createdBy"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type SystemSettingRollbackRequest struct {
	TargetRevision int64 `json:"targetRevision"`
	Revision       int64 `json:"revision"`
}
