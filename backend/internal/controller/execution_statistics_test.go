package controller

import (
	"encoding/json"
	"testing"
	"time"

	"synapseqa/backend/internal/model"
)

func TestBuildExecutionStatistics(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.Local)
	runs := []model.ExecutionRun{
		{ID: 1, Status: "completed", Summary: json.RawMessage(`{"total":10,"passed":9,"failed":1}`), CreatedAt: now.AddDate(0, 0, -1)},
		{ID: 2, Status: "failed", Summary: json.RawMessage(`{"total":10,"passed":5,"failed":5}`), CreatedAt: now.AddDate(0, 0, -8)},
		{ID: 3, Status: "running", Summary: json.RawMessage(`{"total":4,"passed":0,"failed":0}`), CreatedAt: now},
	}

	result := buildExecutionStatistics(runs, now)

	if result.TotalRuns != 3 || result.TotalCases != 24 || result.PassedCases != 14 || result.FailedRuns != 1 || result.RunningRuns != 1 {
		t.Fatalf("统计汇总不正确: %+v", result)
	}
	if result.PassRate < 58.3 || result.PassRate > 58.4 {
		t.Fatalf("累计通过率不正确: %v", result.PassRate)
	}
	if result.RunChange != 100 {
		t.Fatalf("批次环比不正确: %v", result.RunChange)
	}
	if len(result.Trend) != 14 || result.Trend[13].Runs != 1 {
		t.Fatalf("趋势数据不正确: %+v", result.Trend)
	}
}
