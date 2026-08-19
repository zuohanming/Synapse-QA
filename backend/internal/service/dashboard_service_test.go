package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"synapseqa/backend/internal/model"
)

type dashboardServiceFakeRepo struct {
	projects    []model.DashboardProject
	projectsErr error
	ui          model.DashboardSourceData
	uiErr       error
	api         model.DashboardSourceData
	apiErr      error
	perf        model.DashboardSourceData
	perfErr     error
	uiCalls     int
	apiCalls    int
	perfCalls   int
}

func (f *dashboardServiceFakeRepo) ListProjects(context.Context, int64, bool, *int64) ([]model.DashboardProject, error) {
	return f.projects, f.projectsErr
}

func (f *dashboardServiceFakeRepo) ListUIExecutions(context.Context, int64, bool, *int64, time.Time, time.Time) (model.DashboardSourceData, error) {
	f.uiCalls++
	return f.ui, f.uiErr
}

func (f *dashboardServiceFakeRepo) ListAPIExecutions(context.Context, int64, bool, *int64, time.Time, time.Time) (model.DashboardSourceData, error) {
	f.apiCalls++
	return f.api, f.apiErr
}

func (f *dashboardServiceFakeRepo) ListPerfExecutions(context.Context, int64, bool, *int64, time.Time, time.Time) (model.DashboardSourceData, error) {
	f.perfCalls++
	return f.perf, f.perfErr
}

func TestMapDashboardStatus(t *testing.T) {
	tests := map[string]string{
		"active":           "running",
		"pending":          "running",
		"running":          "running",
		"completed":        "success",
		"success":          "success",
		"failed":           "failed",
		"execution_failed": "failed",
		"threshold_failed": "failed",
		"timed_out":        "failed",
		"canceled":         "canceled",
		"not-a-status":     "unknown",
	}
	for input, want := range tests {
		if got := MapDashboardStatus(input); got != want {
			t.Errorf("MapDashboardStatus(%q) = %q, want %q", input, got, want)
		}
	}
	if got := MapDashboardStatus(" RUNNING "); got != "running" {
		t.Fatalf("status should be trimmed and case-insensitive, got %q", got)
	}
}

func TestDashboardRangeIsUTCAndHalfOpen(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 34, 56, 0, time.FixedZone("test", 8*60*60))
	for _, item := range []struct {
		value string
		delta time.Duration
	}{
		{value: "24h", delta: 24 * time.Hour},
		{value: "7d", delta: 7 * 24 * time.Hour},
		{value: "30d", delta: 30 * 24 * time.Hour},
	} {
		from, to, err := DashboardRange(item.value, now)
		if err != nil {
			t.Fatalf("DashboardRange(%q) returned error: %v", item.value, err)
		}
		if !from.Equal(now.UTC().Add(-item.delta)) || !to.Equal(now.UTC()) {
			t.Fatalf("DashboardRange(%q) = [%s, %s), want [%s, %s)", item.value, from, to, now.UTC().Add(-item.delta), now.UTC())
		}
		if from.Location() != time.UTC || to.Location() != time.UTC {
			t.Fatalf("DashboardRange(%q) must return UTC values", item.value)
		}
	}
	if _, _, err := DashboardRange("90d", now); !errors.Is(err, model.ErrValidation) {
		t.Fatalf("invalid range error = %v, want validation error", err)
	}
	from, to, err := DashboardRange("", now)
	if err != nil || !from.Equal(now.UTC().AddDate(0, 0, -7)) || !to.Equal(now.UTC()) {
		t.Fatalf("empty range should default to 7d, got [%s, %s), err=%v", from, to, err)
	}
}

func TestDashboardRecentSortsGloballyAndLimits(t *testing.T) {
	base := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	records := []model.DashboardExecutionRecord{
		{Type: "ui", ID: "old", CreatedAt: base.Add(1 * time.Hour), Status: "success"},
		{Type: "api", ID: "newest", CreatedAt: base.Add(4 * time.Hour), Status: "completed"},
		{Type: "perf", ID: "middle", CreatedAt: base.Add(3 * time.Hour), Status: "running"},
	}
	items := DashboardRecent(records, 2)
	if len(items) != 2 || items[0].ID != "newest" || items[1].ID != "middle" {
		t.Fatalf("unexpected recent order/limit: %+v", items)
	}
	if items[0].Status != "success" || items[1].Status != "running" {
		t.Fatalf("recent statuses were not normalized: %+v", items)
	}
	if items[0].UID != "api:newest" || items[1].UID != "perf:middle" {
		t.Fatalf("recent uid contract mismatch: %+v", items)
	}
}

func TestDashboardAttentionCountsAllFailuresAndLimitsItems(t *testing.T) {
	base := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	records := make([]model.DashboardExecutionRecord, 0, 5)
	for i := 0; i < 4; i++ {
		records = append(records, model.DashboardExecutionRecord{
			Type: "api", ID: string(rune('a' + i)), Status: "failed", CreatedAt: base.Add(time.Duration(i) * time.Hour),
		})
	}
	records = append(records, model.DashboardExecutionRecord{Type: "ui", ID: "success", Status: "success", CreatedAt: base.Add(5 * time.Hour)})

	attention := DashboardAttentionItems(records, true)
	if attention.State != "partial" || attention.Total != 4 || len(attention.Items) != 3 {
		t.Fatalf("unexpected attention result: %+v", attention)
	}
	if attention.Items[0].ID != "d" || attention.Items[2].ID != "b" {
		t.Fatalf("attention is not sorted by recency: %+v", attention.Items)
	}
	if attention.Items[0].UID != "api:d" || attention.Items[0].Source != "api" || attention.Items[0].Type != "failed" {
		t.Fatalf("attention uid/source contract mismatch: %+v", attention.Items[0])
	}
}

func TestDashboardOverviewReturns404ForUnauthorizedProject(t *testing.T) {
	repo := &dashboardServiceFakeRepo{projects: []model.DashboardProject{{ID: 1, Name: "可访问项目"}}}
	svc := NewDashboardService(repo)
	projectID := int64(2)
	_, err := svc.Overview(context.Background(), model.Claims{UserID: 7, RoleCode: "viewer"}, DashboardRequest{ProjectID: &projectID, Range: "24h"})
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("unauthorized project error = %v, want not found", err)
	}
	if repo.uiCalls != 0 || repo.apiCalls != 0 || repo.perfCalls != 0 {
		t.Fatal("source queries must not run for an unauthorized project")
	}
}

func TestDashboardOverviewMarksSourceFailurePartial(t *testing.T) {
	now := time.Now().UTC()
	repo := &dashboardServiceFakeRepo{
		projects: []model.DashboardProject{{ID: 1, Name: "项目"}},
		uiErr:    errors.New("ui unavailable"),
		api: model.DashboardSourceData{
			Counts: model.DashboardExecutionCounts{Total: 1, Failed: 1},
			Recent: []model.DashboardExecutionRecord{{Type: "api", ID: "a1", Status: "failed", CreatedAt: now}},
			Failed: []model.DashboardExecutionRecord{{Type: "api", ID: "a1", Status: "failed", CreatedAt: now}},
		},
		perf: model.DashboardSourceData{
			Counts: model.DashboardExecutionCounts{Total: 1, Running: 1},
			Recent: []model.DashboardExecutionRecord{{Type: "perf", ID: "p1", Status: "running", CreatedAt: now}},
		},
	}
	result, err := NewDashboardService(repo).Overview(context.Background(), model.Claims{UserID: 7, RoleCode: "admin"}, DashboardRequest{Range: "24h"})
	if err != nil {
		t.Fatalf("Overview returned error: %v", err)
	}
	if result.Executions.State != "partial" || result.Executions.SourceStates.UI != "error" {
		t.Fatalf("source failure was not exposed as partial: %+v", result.Executions)
	}
	if result.Executions.Counts.Total != 2 || result.Executions.Counts.Failed != 1 || result.Executions.Counts.Running != 1 {
		t.Fatalf("counts should include only successful source queries: %+v", result.Executions.Counts)
	}
	if result.Attention.Total != 1 || len(result.Attention.Items) != 1 || result.Attention.State != "partial" {
		t.Fatalf("attention partial contract mismatch: %+v", result.Attention)
	}
}

func TestDashboardOverviewForbidsUIAndPerfForNonAdmin(t *testing.T) {
	repo := &dashboardServiceFakeRepo{projects: []model.DashboardProject{{ID: 1, Name: "项目"}}}
	result, err := NewDashboardService(repo).Overview(context.Background(), model.Claims{
		UserID: 7, RoleCode: "viewer", Permissions: []string{"menu.execution.read", "api.interface.read", "perf.plan.read"},
	}, DashboardRequest{Range: "24h"})
	if err != nil {
		t.Fatalf("Overview returned error: %v", err)
	}
	if result.Executions.State != "ok" || result.Executions.SourceStates.UI != "forbidden" || result.Executions.SourceStates.Perf != "forbidden" {
		t.Fatalf("non-admin UI/perf sources must be forbidden: %+v", result.Executions)
	}
	if repo.uiCalls != 0 || repo.perfCalls != 0 {
		t.Fatal("non-admin UI/perf sources must not call the repository")
	}
}

func TestDashboardOverviewUsesAggregatedCountsAndBoundedCandidates(t *testing.T) {
	now := time.Now().UTC()
	recent := make([]model.DashboardExecutionRecord, 5)
	failed := make([]model.DashboardExecutionRecord, 3)
	for i := range recent {
		recent[i] = model.DashboardExecutionRecord{Type: "api", ID: string(rune('a' + i)), Status: "success", CreatedAt: now.Add(-time.Duration(i) * time.Minute)}
	}
	for i := range failed {
		failed[i] = model.DashboardExecutionRecord{Type: "api", ID: string(rune('f' + i)), Status: "failed", CreatedAt: now.Add(-time.Duration(i) * time.Minute)}
	}
	repo := &dashboardServiceFakeRepo{
		projects: []model.DashboardProject{{ID: 1, Name: "项目"}},
		api: model.DashboardSourceData{
			Counts: model.DashboardExecutionCounts{Total: 100, Success: 80, Failed: 15, Running: 3, Canceled: 1, Unknown: 1},
			Recent: recent,
			Failed: failed,
		},
	}
	result, err := NewDashboardService(repo).Overview(context.Background(), model.Claims{UserID: 1, RoleCode: "admin"}, DashboardRequest{Range: "7d"})
	if err != nil {
		t.Fatalf("Overview returned error: %v", err)
	}
	if result.Executions.Counts.Total != 100 || result.Executions.Counts.Failed != 15 || len(result.Executions.Recent) != 5 {
		t.Fatalf("overview must use source aggregate counts and bounded recent candidates: %+v", result.Executions)
	}
	if result.Attention.Total != 15 || len(result.Attention.Items) != 3 {
		t.Fatalf("overview must use aggregate failed total and bounded failed candidates: %+v", result.Attention)
	}
}
