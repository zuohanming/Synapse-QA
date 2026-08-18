package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"synapseqa/backend/internal/model"
)

func TestResolvePerfTargetWithEnvironmentBasePath(t *testing.T) {
	base := "http://127.0.0.1:8080/api/v1"
	for target, expected := range map[string]string{
		"":                        "http://127.0.0.1:8080/api/v1",
		"orders":                  "http://127.0.0.1:8080/api/v1/orders",
		"/orders":                 "http://127.0.0.1:8080/orders",
		"https://x.io/orders?q=1": "http://127.0.0.1:8080/orders?q=1",
	} {
		actual, err := resolvePerfTarget(base, target)
		if err != nil || actual != expected {
			t.Fatalf("resolvePerfTarget(%q) = %q, %v; want %q", target, actual, err, expected)
		}
	}
}

func TestResolvePerfPlanURLsResolvesMixedRelativeOnly(t *testing.T) {
	id := int64(7)
	plan := model.PerfTestPlan{ScenarioType: "mixed", EnvironmentID: &id, EnvironmentBaseURL: "http://127.0.0.1:8080/api", TargetURL: "orders", LoadConfig: []byte(`{"scenarios":[{"url":"/health"},{"url":"https://other.test/x"}]}`)}
	if err := resolvePerfPlanURLs(&plan); err != nil {
		t.Fatal(err)
	}
	if plan.TargetURL != "http://127.0.0.1:8080/api/orders" {
		t.Fatalf("unexpected target: %s", plan.TargetURL)
	}
	if string(plan.LoadConfig) == "" || !strings.Contains(string(plan.LoadConfig), "http://127.0.0.1:8080/health") {
		t.Fatalf("relative mixed URL was not resolved: %s", plan.LoadConfig)
	}
	if !strings.Contains(string(plan.LoadConfig), "https://other.test/x") {
		t.Fatalf("absolute mixed URL changed: %s", plan.LoadConfig)
	}
}

func TestPerfComparisonStrictBoundaryAndNullZero(t *testing.T) {
	base := 100.0
	current := 110.0
	metric := compareMetric("lower", &current, &base, 0.10)
	if metric.Degraded {
		t.Fatal("exactly 10 percent must not degrade")
	}
	zero := 0.0
	positive := 1.0
	metric = compareMetric("lower", &positive, &zero, 0.10)
	if !metric.Degraded || metric.ChangeRate != nil {
		t.Fatalf("zero lower-is-better baseline mismatch: %+v", metric)
	}
	metric = compareMetric("higher", &positive, &zero, 0.10)
	if metric.Degraded || metric.ChangeRate != nil {
		t.Fatalf("zero higher-is-better baseline mismatch: %+v", metric)
	}
	metric = compareMetric("lower", &zero, &zero, 0.10)
	if metric.ChangeRate == nil || *metric.ChangeRate != 0 || metric.DegradationRate == nil || *metric.DegradationRate != 0 {
		t.Fatalf("zero/zero rates must be zero: %+v", metric)
	}
	metric = compareMetric("lower", nil, &base, 0.10)
	if metric.Degraded || metric.Delta != nil {
		t.Fatalf("null metric mismatch: %+v", metric)
	}
}

func TestParsePerfScheduleRejectsDescriptorsAndSeconds(t *testing.T) {
	if _, _, err := parsePerfSchedule("@hourly", "UTC"); err == nil {
		t.Fatal("descriptor must be rejected")
	}
	if _, _, err := parsePerfSchedule("*/5 * * * * *", "UTC"); err == nil {
		t.Fatal("seconds expression must be rejected")
	}
	if _, _, err := parsePerfSchedule("*/5 * * * *", "Not/AZone"); err == nil {
		t.Fatal("invalid timezone must be rejected")
	}
	next, err := nextPerfScheduleAt("*/5 * * * *", "Asia/Shanghai", time.Date(2026, 8, 18, 0, 1, 0, 0, time.UTC))
	if err != nil || next == nil || next.Location() != time.UTC {
		t.Fatalf("next schedule must be UTC: %v %v", next, err)
	}
}

func TestPerfP99SummaryShapesAndStringJSONSanitize(t *testing.T) {
	shapes := []string{
		`{"metrics":{"http_req_duration":{"values":{"p(99)":12.5}}}}`,
		`{"metrics":{"http_req_duration":{"p99_duration_ms":13.5}}}`,
		`{"metrics":{"p99_duration_ms":14.5}}`,
	}
	for index, raw := range shapes {
		value := perfP99FromSummary([]byte(raw))
		if value == nil || *value != float64(12+index)+0.5 {
			t.Fatalf("shape %d p99=%v", index, value)
		}
	}
	body := `{"password":"secret-body","nested":[{"access_token":"secret-token"}]}`
	safe := sanitizePerfValue(body).(string)
	if strings.Contains(safe, "secret-body") || strings.Contains(safe, "secret-token") {
		t.Fatalf("string JSON was not recursively sanitized: %s", safe)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(safe), &decoded); err != nil {
		t.Fatalf("sanitized body is not JSON: %v", err)
	}
}

func TestRunPlanRejectsDraftAndDisabledPlans(t *testing.T) {
	for _, status := range []string{"draft", "disabled"} {
		repo := &fakePerformanceRepo{plan: defaultPerfPlan(1)}
		repo.plan.Status = status
		svc := NewPerformanceService(repo, nil, nil, "http://localhost")
		if _, _, err := svc.RunPlan(context.Background(), "admin", status, 1); err == nil {
			t.Fatalf("status %s was executable", status)
		}
	}
}

func TestMixedAbsoluteNonHTTPIsRejectedDuringPlanValidation(t *testing.T) {
	req := validPerfPlanRequest()
	req.ScenarioType = "mixed"
	req.TargetURL = "http://example.test"
	req.LoadConfig = []byte(`{"vus":1,"duration":"1s","thinkTime":"1ms","scenarios":[{"name":"ftp","weight":1,"method":"GET","url":"ftp://example.test/file"}]}`)
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil, nil, "")
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("non-http mixed absolute URL was accepted")
	}
}

type compensationNotifier struct{ calls []model.NotificationCreate }

func (n *compensationNotifier) Create(_ context.Context, item model.NotificationCreate) error {
	n.calls = append(n.calls, item)
	return nil
}

type compensationRepo struct {
	*fakePerformanceRepo
	performanceP2Repository
	candidates []model.PerfTestRun
	marked     map[int64]bool
	runs       map[int64]model.PerfTestRun
	baselines  map[int64]model.PerfBaseline
}

func (r *compensationRepo) ListPerfDegradationCandidates(_ context.Context, limit int) ([]model.PerfTestRun, error) {
	items := make([]model.PerfTestRun, 0, limit)
	for _, item := range r.candidates {
		if !r.marked[item.ID] {
			items = append(items, item)
			if len(items) == limit {
				break
			}
		}
	}
	return items, nil
}

func (r *compensationRepo) GetRun(_ context.Context, id int64) (model.PerfTestRun, error) {
	return r.runs[id], nil
}

func (r *compensationRepo) GetPerfBaseline(_ context.Context, _ int64, _ string, _ string, runID *int64) (model.PerfBaseline, error) {
	if runID != nil {
		if baseline, ok := r.baselines[*runID]; ok {
			return baseline, nil
		}
	}
	return model.PerfBaseline{}, errors.New("baseline not found")
}

func (r *compensationRepo) MarkPerfDegradationChecked(_ context.Context, id int64) (int64, error) {
	r.marked[id] = true
	return 1, nil
}

func TestPerfDegradationCompensationAdvancesPastTwentyNonDegradedRuns(t *testing.T) {
	repo := &compensationRepo{fakePerformanceRepo: &fakePerformanceRepo{}, marked: map[int64]bool{}, runs: map[int64]model.PerfTestRun{}, baselines: map[int64]model.PerfBaseline{}}
	for index := int64(1); index <= 21; index++ {
		currentP95 := 100.0
		if index == 21 {
			currentP95 = 200
		}
		environmentID := index
		current := model.PerfTestRun{ID: index, PlanID: 1, Status: model.PerfRunCompleted, ScenarioType: "baseline", Environment: "test", EnvironmentID: &environmentID, TriggeredBy: "admin", P95DurationMs: floatPtr(currentP95), P99DurationMs: floatPtr(100), ErrorRate: floatPtr(0), RPS: floatPtr(100)}
		baseID := 1000 + index
		base := current
		base.ID = baseID
		base.P95DurationMs = floatPtr(100)
		repo.candidates = append(repo.candidates, current)
		repo.runs[current.ID] = current
		repo.runs[base.ID] = base
		repo.baselines[environmentID] = model.PerfBaseline{ID: index, PlanID: 1, ScenarioType: "baseline", Environment: "test", EnvironmentID: &environmentID, RunID: baseID}
	}
	notifier := &compensationNotifier{}
	svc := NewPerformanceService(repo, nil, nil, "")
	svc.SetNotifier(notifier)
	svc.RecoverPerfDegradation(context.Background())
	if len(repo.marked) != 20 || len(notifier.calls) != 0 {
		t.Fatalf("first compensation pass marked=%d notifications=%d", len(repo.marked), len(notifier.calls))
	}
	svc.RecoverPerfDegradation(context.Background())
	if len(repo.marked) != 21 || len(notifier.calls) != 1 {
		t.Fatalf("second compensation pass marked=%d notifications=%d", len(repo.marked), len(notifier.calls))
	}
}

type scheduleActiveRunRepo struct {
	*fakePerformanceRepo
	performanceP2Repository
	schedule         model.PerfSchedule
	active           model.PerfTestRun
	environment      model.PerfEnvironment
	finishResult     string
	finishNext       *time.Time
	finishRunID      *int64
	claimWasReleased bool
	lastError        string
}

func (r *scheduleActiveRunRepo) ClaimPerfSchedules(context.Context, string, time.Time, int) ([]model.PerfSchedule, error) {
	return []model.PerfSchedule{r.schedule}, nil
}

func (r *scheduleActiveRunRepo) GetPerfEnvironment(context.Context, int64) (model.PerfEnvironment, error) {
	return r.environment, nil
}

func (r *scheduleActiveRunRepo) GetActivePerfRun(context.Context, int64) (model.PerfTestRun, error) {
	return r.active, nil
}

func (r *scheduleActiveRunRepo) FinishPerfSchedule(_ context.Context, _ int64, _ string, _ *time.Time, next *time.Time, result string, runID *int64, _ string) (int64, error) {
	r.finishResult = result
	r.finishNext = next
	r.finishRunID = runID
	r.claimWasReleased = true
	return 1, nil
}

func (r *scheduleActiveRunRepo) SetPerfScheduleError(_ context.Context, _ int64, _ string, lastError string) (int64, error) {
	r.lastError = lastError
	return 1, nil
}

func TestPerfScheduleSkipsAnyActiveRunAcrossEnvironmentOverride(t *testing.T) {
	environmentID := int64(2)
	plan := defaultPerfPlan(1)
	plan.ProductID = 1
	plan.Environment = "env-a"
	plan.Status = "active"
	scheduledFor := time.Now().UTC().Add(-time.Minute)
	active := model.PerfTestRun{ID: 99, PlanID: 1, Status: model.PerfRunRunning, Environment: "env-a", IdempotencyKey: "perf-schedule:7:" + scheduledFor.Format(time.RFC3339)}
	repo := &scheduleActiveRunRepo{
		fakePerformanceRepo: &fakePerformanceRepo{plan: plan},
		schedule:            model.PerfSchedule{ID: 7, PlanID: 1, EnvironmentID: &environmentID, CronExpression: "* * * * *", Timezone: "UTC", Enabled: true, NextRunAt: &scheduledFor, ClaimToken: "claim-token", CreatedBy: "admin"},
		active:              active,
		environment:         model.PerfEnvironment{EnvironmentID: environmentID, ProductID: 1, EnvName: "env-b", BaseURL: "http://env-b.test"},
	}
	NewPerformanceService(repo, nil, nil, "").ProcessPerfSchedules(context.Background())
	if repo.finishResult != "skipped_active_run" || repo.finishNext == nil || repo.finishRunID != nil || !repo.claimWasReleased {
		t.Fatalf("active run was not skipped and finalized: result=%q next=%v runID=%v released=%v", repo.finishResult, repo.finishNext, repo.finishRunID, repo.claimWasReleased)
	}
	if repo.lastError != "" {
		t.Fatalf("active run entered transient error path: %s", repo.lastError)
	}
}

type exportP2Repo struct {
	*fakePerformanceRepo
	performanceP2Repository
	baseline model.PerfBaseline
	current  model.PerfTestRun
	baseRun  model.PerfTestRun
	plan     model.PerfTestPlan
}

func (r *exportP2Repo) GetRun(ctx context.Context, id int64) (model.PerfTestRun, error) {
	if id == r.baseRun.ID {
		return r.baseRun, nil
	}
	return r.current, nil
}

func (r *exportP2Repo) GetPlan(context.Context, int64) (model.PerfTestPlan, error) {
	return r.plan, nil
}
func (r *exportP2Repo) GetPerfBaseline(context.Context, int64, string, string, *int64) (model.PerfBaseline, error) {
	return r.baseline, nil
}

func TestExportPerfJSONSanitizesDiagnosticJSONAndUsesFrozenFields(t *testing.T) {
	run := model.PerfTestRun{ID: 2, PlanID: 1, Status: model.PerfRunCompleted, ScenarioType: "baseline", TriggeredBy: "admin", TotalRequests: 10, P95DurationMs: floatPtr(10), P99DurationMs: floatPtr(12), ErrorRate: floatPtr(0), RPS: floatPtr(2), EnvironmentID: int64Ptr(7), EnvironmentName: "固定环境", EnvironmentBaseURL: "http://example.test", ErrorMessage: `{"password":"diagnostic-password"}`, DiagnosticOutput: `{"access_token":"diagnostic-access","nested":{"client_secret":"diagnostic-client"}}`, PlanSnapshot: []byte(`{"targetUrl":"/health","resolvedTargetUrl":"http://example.test/health?refresh_token=query-secret","environmentName":"固定环境","environmentBaseUrl":"http://example.test","environmentId":7,"body":"{\"password\":\"body-secret\"}"}`), Summary: []byte(`{"metrics":{}}`), Series: []byte(`{"version":1,"points":[]}`)}
	base := run
	base.ID = 1
	base.P95DurationMs = floatPtr(10)
	base.P99DurationMs = floatPtr(12)
	base.RPS = floatPtr(2)
	repo := &exportP2Repo{fakePerformanceRepo: &fakePerformanceRepo{}, baseline: model.PerfBaseline{ID: 1, PlanID: 1, ScenarioType: "baseline", RunID: 1}, current: run, baseRun: base, plan: model.PerfTestPlan{ID: 1, Name: "导出方案", ScenarioType: "baseline"}}
	data, _, _, err := NewPerformanceService(repo, nil, nil, "").ExportPerfRun(context.Background(), 2, "json")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["exportedAt"] == nil {
		t.Fatal("exportedAt missing")
	}
	environment := payload["environment"].(map[string]any)
	if environment["environmentId"] != float64(7) || environment["resolvedTargetUrl"] == nil {
		t.Fatalf("environment payload incomplete: %+v", environment)
	}
	diagnostic := payload["diagnostic"].(map[string]any)
	if diagnostic["output"] == nil {
		t.Fatalf("diagnostic output missing: %+v", diagnostic)
	}
	for _, secret := range []string{"diagnostic-password", "diagnostic-access", "diagnostic-client", "body-secret", "query-secret"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("secret leaked in export: %s", secret)
		}
	}
}

func TestExportPerfCSVUsesFrozenContractAndSnapshotValues(t *testing.T) {
	requested := time.Date(2026, 8, 18, 3, 4, 5, 0, time.UTC)
	finished := requested.Add(2 * time.Second)
	duration := 200
	exitCode := 0
	run := model.PerfTestRun{
		ID: 5, PlanID: 9, ScenarioType: "-scenario", Status: "@status", TriggeredBy: "+operator", ConfigHash: "=hash", GeneratorVersion: "-generator", K6Version: "@k6",
		Environment: "database-env", EnvironmentName: "database-name", RequestedAt: &requested, StartedAt: nil, FinishedAt: &finished,
		DurationMs: &duration, TotalRequests: 10, AvgDurationMs: floatPtr(1.5), P95DurationMs: floatPtr(2.5), P99DurationMs: floatPtr(3.5), ErrorRate: floatPtr(0.1), RPS: floatPtr(5),
		ExitCode: &exitCode, FailureStage: "=failure", ErrorMessage: `{"password":"csv-secret"}`, NeedsAttention: true,
		PlanSnapshot: []byte(`{"environmentId":42,"environment":"=snapshot-env","environmentName":"+snapshot-name","resolvedTargetUrl":"https://example.test/path?access_token=csv-secret"}`),
	}
	comparison := model.PerfComparison{BaselineRunID: int64Ptr(5), Degraded: true, DegradedMetrics: []string{"p95DurationMs", "rps"}}
	data, err := exportPerfCSV(run, model.PerfTestPlan{ID: 9, Name: "=plan"}, comparison)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("CSV BOM missing")
	}
	reader := csv.NewReader(bytes.NewReader(data[3:]))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := []string{"run_id", "plan_id", "plan_name", "scenario_type", "status", "triggered_by", "environment_id", "environment", "environment_name", "resolved_target_url", "requested_at", "started_at", "finished_at", "duration_ms", "total_requests", "avg_duration_ms", "p95_duration_ms", "p99_duration_ms", "error_rate", "rps", "is_baseline", "baseline_run_id", "degraded", "degraded_metrics", "failure_stage", "error_message", "needs_attention", "config_hash", "generator_version", "k6_version"}
	if strings.Join(header, ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("unexpected CSV header: %v", header)
	}
	row, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(row) != len(wantHeader) {
		t.Fatalf("unexpected CSV column count: got %d want %d", len(row), len(wantHeader))
	}
	checks := map[int]string{
		0: "5", 1: "9", 6: "42", 10: "2026-08-18T03:04:05Z", 11: "", 12: "2026-08-18T03:04:07Z", 13: "200", 14: "10", 15: "1.5", 16: "2.5", 17: "3.5", 18: "0.1", 19: "5", 20: "true", 21: "5", 22: "true", 23: "p95DurationMs;rps", 26: "true",
	}
	for index, want := range checks {
		if row[index] != want {
			t.Fatalf("column %s=%q, want %q", wantHeader[index], row[index], want)
		}
	}
	for index, wantPrefix := range map[int]string{2: "'", 3: "'", 4: "'", 5: "'", 7: "'", 8: "'", 24: "'", 27: "'", 28: "'", 29: "'"} {
		if !strings.HasPrefix(row[index], wantPrefix) {
			t.Fatalf("formula protection missing for %s: %q", wantHeader[index], row[index])
		}
	}
	if strings.Contains(string(data), "csv-secret") {
		t.Fatal("CSV leaked a secret")
	}
}

func int64Ptr(value int64) *int64 { return &value }

func timePtr(value time.Time) *time.Time { return &value }
