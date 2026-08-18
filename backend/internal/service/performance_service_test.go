package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"synapseqa/backend/internal/model"
)

type fakePerformanceRepo struct {
	productExists bool
	createPlanID  int64
	createRunID   int64
	listErr       error
	getErr        error
	createErr     error
	updateErr     error
	deleteErr     error
	updateRows    int64
	deleteRows    int64
	getRunErr     error
	listRunsErr   error
	createRunErr  error

	plan            model.PerfTestPlan
	idempotentRun   model.PerfTestRun
	idempotentErr   error
	idempotentMiss  bool
	idempotentCalls int

	run               model.PerfTestRun
	runByTaskID       model.PerfTestRun
	runByTaskErr      error
	updateStatusRows  int64
	updateStatusExtra map[string]any
	updateResultRows  int64
	updateResultTo    string
	updateResult      model.PerfRunResult
	listRunsReturn    []model.PerfTestRun
}

func (f *fakePerformanceRepo) ListPlans(ctx context.Context, filter model.PerfTestPlanFilter, page, pageSize int) ([]model.PerfTestPlan, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return []model.PerfTestPlan{{ID: 1, Name: "登录接口压测", ScenarioType: "baseline"}}, 1, nil
}

func (f *fakePerformanceRepo) GetPlan(ctx context.Context, id int64) (model.PerfTestPlan, error) {
	if f.getErr != nil {
		return model.PerfTestPlan{}, f.getErr
	}
	if f.plan.ID == 0 {
		f.plan = defaultPerfPlan(id)
	}
	return f.plan, nil
}

func (f *fakePerformanceRepo) CreatePlan(ctx context.Context, req model.PerfTestPlanRequest, actor string) (int64, error) {
	if f.createErr != nil {
		return 0, f.createErr
	}
	if f.createPlanID == 0 {
		f.createPlanID = 1
	}
	return f.createPlanID, nil
}

func (f *fakePerformanceRepo) UpdatePlan(ctx context.Context, id int64, req model.PerfTestPlanRequest) (int64, error) {
	if f.updateErr != nil {
		return 0, f.updateErr
	}
	return f.updateRows, nil
}

func (f *fakePerformanceRepo) DeletePlan(ctx context.Context, id int64) (int64, error) {
	if f.deleteErr != nil {
		return 0, f.deleteErr
	}
	return f.deleteRows, nil
}

func (f *fakePerformanceRepo) ExistsProduct(ctx context.Context, id int64) bool {
	return f.productExists
}

func (f *fakePerformanceRepo) ListRuns(ctx context.Context, filter model.PerfTestRunFilter, page, pageSize int) ([]model.PerfTestRun, int64, error) {
	if f.listRunsErr != nil {
		return nil, 0, f.listRunsErr
	}
	if len(f.listRunsReturn) > 0 {
		items := []model.PerfTestRun{}
		for _, run := range f.listRunsReturn {
			if filter.Status == "" || run.Status == filter.Status {
				items = append(items, run)
			}
		}
		return items, int64(len(items)), nil
	}
	if filter.Status != "" && filter.Status != model.PerfRunPending {
		return []model.PerfTestRun{}, 0, nil
	}
	return []model.PerfTestRun{{ID: 1, PlanID: 1, Status: model.PerfRunPending}}, 1, nil
}

func (f *fakePerformanceRepo) GetRun(ctx context.Context, id int64) (model.PerfTestRun, error) {
	if f.getRunErr != nil {
		return model.PerfTestRun{}, f.getRunErr
	}
	if f.run.ID == 0 {
		f.run = model.PerfTestRun{ID: id, PlanID: 1, Status: "pending"}
	}
	return f.run, nil
}

func (f *fakePerformanceRepo) GetRunByTaskID(ctx context.Context, taskID string) (model.PerfTestRun, error) {
	if f.runByTaskErr != nil {
		return model.PerfTestRun{}, f.runByTaskErr
	}
	if f.runByTaskID.ID == 0 {
		f.runByTaskID = model.PerfTestRun{ID: 1, PlanID: 1, Status: "dispatching"}
	}
	return f.runByTaskID, nil
}

func (f *fakePerformanceRepo) GetRunByIdempotencyKey(ctx context.Context, triggeredBy, key string) (model.PerfTestRun, error) {
	f.idempotentCalls++
	if f.idempotentMiss && f.idempotentCalls == 1 {
		return model.PerfTestRun{}, errors.New("not found")
	}
	if f.idempotentErr != nil {
		return model.PerfTestRun{}, f.idempotentErr
	}
	if f.idempotentRun.ID == 0 {
		return model.PerfTestRun{}, errors.New("not found")
	}
	return f.idempotentRun, nil
}

func (f *fakePerformanceRepo) CreateRun(ctx context.Context, planID int64, scenarioType, environment, configHash, idempotencyKey, triggeredBy string, planSnapshot json.RawMessage, requestedAt, expectedFinishAt time.Time) (int64, error) {
	if f.createRunErr != nil {
		return 0, f.createRunErr
	}
	if f.createRunID == 0 {
		f.createRunID = 100
	}
	return f.createRunID, nil
}

func (f *fakePerformanceRepo) MarkNeedsAttention(ctx context.Context, id int64) (int64, error) {
	return 1, nil
}

func (f *fakePerformanceRepo) UpdateRunStatus(ctx context.Context, id int64, from, to string, extra map[string]any) (int64, error) {
	f.updateStatusExtra = extra
	return f.updateStatusRows, nil
}

func (f *fakePerformanceRepo) UpdateRunResult(ctx context.Context, id int64, from []string, to string, result model.PerfRunResult) (int64, error) {
	f.updateResultTo = to
	f.updateResult = result
	return f.updateResultRows, nil
}

func defaultPerfPlan(id int64) model.PerfTestPlan {
	return model.PerfTestPlan{
		ID:           id,
		Name:         "登录接口压测",
		TargetURL:    "https://example.com/login",
		Method:       "GET",
		ScenarioType: "baseline",
		Status:       "active",
		Environment:  "test",
		LoadConfig:   json.RawMessage(`{"vus":3,"duration":"2m"}`),
		Thresholds:   []model.PerfThreshold{{Metric: "http_req_duration", Aggregation: "p(95)", Operator: "<", Value: 500}},
	}
}

func validPerfPlanRequest() model.PerfTestPlanRequest {
	return model.PerfTestPlanRequest{
		ProductID:    1,
		Name:         "登录接口压测",
		TargetURL:    "https://example.com/login",
		Method:       "GET",
		ScenarioType: "baseline",
		LoadConfig:   json.RawMessage(`{"vus":3,"duration":"2m"}`),
		Environment:  "test",
		Priority:     "P1",
		Status:       "active",
		Owner:        "admin",
		Headers:      json.RawMessage(`{}`),
		Thresholds:   []model.PerfThreshold{{Metric: "http_req_duration", Aggregation: "p(95)", Operator: "<", Value: 500}},
	}
}

func TestPerformanceServiceCreateSuccess(t *testing.T) {
	repo := &fakePerformanceRepo{productExists: true, createPlanID: 9}
	logger := &fakeOperationLogger{}
	svc := NewPerformanceServiceWithLogger(repo, logger)
	id, err := svc.CreatePlan(context.Background(), "admin", validPerfPlanRequest())
	if err != nil {
		t.Fatalf("CreatePlan returned error: %v", err)
	}
	if id != 9 {
		t.Fatalf("expected id 9, got %d", id)
	}
	if logger.calls != 1 {
		t.Fatalf("expected one log call, got %d", logger.calls)
	}
}

func TestPerformanceServiceScenarioValidation(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil, nil, "http://localhost")
	cases := []struct {
		name    string
		scnType string
		config  string
	}{
		{"baseline 缺 vus", "baseline", `{"duration":"2m"}`},
		{"soak 缺 duration", "soak", `{"vus":100}`},
		{"ramp 缺 stages", "ramp", `{"stages":[]}`},
		{"peak 缺 peakVus", "peak", `{"rampDuration":"2m","holdDuration":"30m"}`},
		{"stress 缺 stepVus", "stress", `{"startVus":10,"stepDuration":"1m","maxVus":500}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validPerfPlanRequest()
			req.ScenarioType = tc.scnType
			req.LoadConfig = json.RawMessage(tc.config)
			if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
				t.Fatalf("expected %s error", tc.name)
			}
		})
	}
}

func TestPerformanceServiceMixedScenarioAllowed(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil, nil, "http://localhost")
	req := validPerfPlanRequest()
	req.ScenarioType = "mixed"
	req.LoadConfig = json.RawMessage(`{"vus":1,"duration":"3s","thinkTime":"10ms","scenarios":[{"name":"首页","weight":1,"method":"GET","url":"/health"}]}`)
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err != nil {
		t.Fatalf("mixed 场景应允许持久化，返回错误：%v", err)
	}
}

func TestNormalizeThresholdDelayAndMixedValidation(t *testing.T) {
	req := validPerfPlanRequest()
	req.Thresholds = []model.PerfThreshold{{Metric: "http_req_duration", Aggregation: "p(95)", Operator: "<", Value: 500, AbortOnFail: false, DelayAbortEval: "1s"}}
	if err := normalizeThresholds(&req); err != nil {
		t.Fatalf("non-abort delay should be normalized: %v", err)
	}
	if req.Thresholds[0].DelayAbortEval != "" {
		t.Fatal("non-abort threshold must not retain delayAbortEval")
	}
	req.Thresholds[0].AbortOnFail = true
	req.Thresholds[0].DelayAbortEval = "not-duration"
	if err := normalizeThresholds(&req); err == nil {
		t.Fatal("invalid delayAbortEval should be rejected")
	}
	if err := validateLoadConfig("mixed", json.RawMessage(`{"vus":1,"duration":"1s","thinkTime":"0.5s","scenarios":[{"name":"a","weight":0.6,"method":"GET","url":"/a","headers":{},"body":{"x":1}},{"name":"b","weight":0.4,"method":"GET","url":"relative","headers":{},"body":"x"}]}`)); err != nil {
		t.Fatalf("valid mixed config rejected: %v", err)
	}
	if err := validateLoadConfig("mixed", json.RawMessage(`{"vus":1,"duration":"1s","thinkTime":"0.5s","scenarios":[{"name":"a","weight":0.600001,"method":"GET","url":"/a"},{"name":"b","weight":0.4,"method":"GET","url":"/b"}]}`)); err == nil {
		t.Fatal("mixed weight tolerance should be strict")
	}
}

func TestPerformanceVUsHardLimitAndMixedTargetSemantics(t *testing.T) {
	for _, test := range []struct {
		name    string
		config  string
		wantErr bool
	}{
		{"baseline500", `{"vus":500,"duration":"1s"}`, false},
		{"baseline501", `{"vus":501,"duration":"1s"}`, true},
		{"ramp500", `{"stages":[{"duration":"1s","target":500}]}`, false},
		{"ramp501", `{"stages":[{"duration":"1s","target":501}]}`, true},
		{"stress500", `{"startVus":10,"stepVus":10,"maxVus":500,"stepDuration":"1m"}`, false},
		{"stress501", `{"startVus":10,"stepVus":10,"maxVus":501,"stepDuration":"1m"}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := validateLoadConfig(map[string]string{"baseline500": "baseline", "baseline501": "baseline", "ramp500": "ramp", "ramp501": "ramp", "stress500": "stress", "stress501": "stress"}[test.name], json.RawMessage(test.config)); (got != nil) != test.wantErr {
				t.Fatalf("validateLoadConfig error=%v wantErr=%v", got, test.wantErr)
			}
		})
	}
	if err := validateMixedTarget("", json.RawMessage(`{"scenarios":[{"url":"https://example.com/a"}]}`)); err != nil {
		t.Fatalf("absolute mixed URL should not require target: %v", err)
	}
	if err := validateMixedTarget("", json.RawMessage(`{"scenarios":[{"url":"/a"}]}`)); err == nil {
		t.Fatal("relative mixed URL should require target")
	}
}

func TestPerformanceStressDurationUsesFiftyStages(t *testing.T) {
	duration := stressDuration(map[string]any{"startVus": float64(10), "stepVus": float64(10), "maxVus": float64(500), "stepDuration": "1m"})
	if duration != 50*time.Minute {
		t.Fatalf("expected 50 minute stress duration, got %v", duration)
	}
}

func TestPerformanceServiceThresholdValidation(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil, nil, "http://localhost")
	req := validPerfPlanRequest()
	req.Thresholds = []model.PerfThreshold{{Metric: "http_req_duration", Aggregation: "p(95)", Operator: "contains", Value: 500}}
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected invalid operator error")
	}
	req = validPerfPlanRequest()
	req.Thresholds = []model.PerfThreshold{{Metric: "", Aggregation: "p(95)", Operator: "<", Value: 500}}
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected empty metric error")
	}
}

func TestPerformanceServiceAbortOnFailNormalized(t *testing.T) {
	repo := &fakePerformanceRepo{productExists: true, createPlanID: 1}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	req := validPerfPlanRequest()
	req.Thresholds = []model.PerfThreshold{{Metric: "http_req_duration", Aggregation: "p(95)", Operator: "<", Value: 500, AbortOnFail: true}}
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err != nil {
		t.Fatalf("CreatePlan returned error: %v", err)
	}
	// P1 允许 abortOnFail，只有关闭自动中止时才清空 delayAbortEval。
	normalized, err := svc.normalizeRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("normalizeRequest returned error: %v", err)
	}
	if !normalized.Thresholds[0].AbortOnFail {
		t.Fatal("expected abortOnFail to remain enabled in P1")
	}
}

func TestPerformanceServiceEnvironmentValidation(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil, nil, "http://localhost")
	req := validPerfPlanRequest()
	req.Environment = "prod"
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected invalid environment error")
	}
}

func TestPerformanceServiceRejectsNameMissing(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil, nil, "http://localhost")
	req := validPerfPlanRequest()
	req.Name = ""
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected name required error")
	}
	req = validPerfPlanRequest()
	req.Name = strings.Repeat("测", 121)
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected name length error")
	}
}

func TestPerformanceServiceRunPlanIdempotentSameKey(t *testing.T) {
	plan := defaultPerfPlan(1)
	_, expectedHash, _ := buildPlanSnapshotAndHash(plan)
	repo := &fakePerformanceRepo{
		productExists:    true,
		plan:             plan,
		idempotentRun:    model.PerfTestRun{ID: 5, PlanID: 1, ConfigHash: expectedHash, Status: "pending"},
		updateStatusRows: 1,
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	run, reused, err := svc.RunPlan(context.Background(), "admin", "key-1", 1)
	if err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}
	if !reused {
		t.Fatal("expected reused=true for same key and config")
	}
	if run.ID != 5 {
		t.Fatalf("expected reused run id 5, got %d", run.ID)
	}
	if run.Status != model.PerfRunQueued {
		t.Fatalf("expected reused pending run to be queued, got %s", run.Status)
	}
}

func TestPerformanceServiceRunPlanQueuesCreatedRun(t *testing.T) {
	repo := &fakePerformanceRepo{
		productExists:    true,
		updateStatusRows: 1,
		idempotentMiss:   true,
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	run, reused, err := svc.RunPlan(context.Background(), "admin", "key-new", 1)
	if err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}
	if reused {
		t.Fatal("expected a newly created run")
	}
	if run.Status != model.PerfRunQueued {
		t.Fatalf("expected created run to be queued, got %s", run.Status)
	}
}

func TestPerformanceServiceRunPlanRejectsUnqueuedRun(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil, nil, "http://localhost")
	if _, _, err := svc.RunPlan(context.Background(), "admin", "key-unqueueable", 1); err == nil {
		t.Fatal("expected RunPlan to fail when pending→queued does not affect a row")
	}
}

func TestPerformanceServiceRunPlanIdempotentDifferentConfig(t *testing.T) {
	repo := &fakePerformanceRepo{
		productExists: true,
		plan:          defaultPerfPlan(1),
		idempotentRun: model.PerfTestRun{ID: 5, PlanID: 1, ConfigHash: "different-hash", Status: "pending"},
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	_, _, err := svc.RunPlan(context.Background(), "admin", "key-1", 1)
	if !errors.Is(err, ErrPerfConfigConflict) {
		t.Fatalf("expected ErrPerfConfigConflict, got %v", err)
	}
}

func TestPerformanceServiceRunPlanConcurrentConflictRechecks(t *testing.T) {
	plan := defaultPerfPlan(1)
	_, expectedHash, _ := buildPlanSnapshotAndHash(plan)
	repo := &fakePerformanceRepo{
		productExists:    true,
		plan:             plan,
		idempotentRun:    model.PerfTestRun{ID: 7, PlanID: 1, ConfigHash: expectedHash, Status: "pending"},
		idempotentMiss:   true,
		createRunErr:     errors.New("unique violation"),
		updateStatusRows: 1,
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	run, reused, err := svc.RunPlan(context.Background(), "admin", "key-1", 1)
	if err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}
	if !reused || run.ID != 7 {
		t.Fatalf("expected conflict recheck to return reused run 7, got reused=%v id=%d", reused, run.ID)
	}
}

func TestPerformanceServiceRunPlanEmptyKey(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil, nil, "http://localhost")
	if _, _, err := svc.RunPlan(context.Background(), "admin", "  ", 1); err == nil {
		t.Fatal("expected empty idempotency key error")
	}
}

func TestPerformanceServiceRunPlanMissingPlan(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{getErr: errors.New("missing")}, nil, nil, "http://localhost")
	if _, _, err := svc.RunPlan(context.Background(), "admin", "key-1", 1); err == nil {
		t.Fatal("expected run missing plan error")
	}
}

func TestPerformanceServiceCancelRun(t *testing.T) {
	repo := &fakePerformanceRepo{updateStatusRows: 1, updateResultRows: 1}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	if err := svc.CancelRun(context.Background(), "admin", 1); err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}
}

func TestPerformanceServiceCancelRunAlreadyFinal(t *testing.T) {
	repo := &fakePerformanceRepo{run: model.PerfTestRun{ID: 1, PlanID: 1, Status: "completed"}}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	if err := svc.CancelRun(context.Background(), "admin", 1); err == nil {
		t.Fatal("expected already final error")
	}
}

func TestPerformanceServiceHandleCallbackRunning(t *testing.T) {
	token := "secret-token"
	repo := &fakePerformanceRepo{
		runByTaskID:      model.PerfTestRun{ID: 1, PlanID: 1, Status: "dispatching", CallbackTokenHash: sha256Hex(token)},
		updateStatusRows: 1,
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	req := model.PerfCallbackRequest{Status: "running", ScriptHash: "abc", K6Version: "0.52.0"}
	if err := svc.HandleCallback(context.Background(), "task-1", token, req); err != nil {
		t.Fatalf("HandleCallback returned error: %v", err)
	}
	if _, ok := repo.updateStatusExtra["expected_finish_at"]; !ok {
		t.Fatal("running callback should recalculate expected_finish_at from startedAt")
	}
}

func TestPerformanceServiceAllowsStartupFailureFromDispatched(t *testing.T) {
	token := "startup-token"
	repo := &fakePerformanceRepo{
		runByTaskID:      model.PerfTestRun{ID: 1, PlanID: 1, Status: model.PerfRunDispatched, CallbackTokenHash: sha256Hex(token)},
		updateResultRows: 1,
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	err := svc.HandleCallback(context.Background(), "task-1", token, model.PerfCallbackRequest{
		Status:           model.PerfRunExecutionFailed,
		FailureStage:     model.PerfFailureStartup,
		DiagnosticOutput: "k6 启动失败",
	})
	if err != nil {
		t.Fatalf("startup failure callback returned error: %v", err)
	}
	if repo.updateResultTo != model.PerfRunExecutionFailed || repo.updateResult.Series == nil {
		t.Fatalf("startup failure result was not persisted: %+v", repo.updateResult)
	}
	var series model.PerfSeries
	_ = json.Unmarshal(repo.updateResult.Series, &series)
	if !series.Partial {
		t.Fatal("startup failure series must be partial")
	}
}

func TestPerformanceServiceSanitizesSeriesStructAndEvent(t *testing.T) {
	series := normalizePerfSeries(json.RawMessage(`{"points":[{"message":"token=secret"}],"errorTopN":[{"message":"password=secret"}],"statusCodes":{},"thresholds":[]}`))
	if strings.Contains(string(series), "secret") {
		t.Fatalf("series secret was not sanitized: %s", series)
	}
	token := "event-token"
	repo := &fakePerformanceRepo{runByTaskID: model.PerfTestRun{ID: 1, Status: model.PerfRunRunning, CallbackTokenHash: sha256Hex(token)}}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	if err := svc.HandleSampleEvent(context.Background(), "task-1", token, model.PerfSampleEvent{Sequence: 1, WindowMs: 1000, Message: "token=secret"}); err != nil {
		t.Fatalf("sample event returned error: %v", err)
	}
	snapshot, ok := svc.events.Snapshot(1)
	if !ok || strings.Contains(snapshot.Points[0].Message, "secret") {
		t.Fatalf("event secret was not sanitized: %+v", snapshot)
	}
}

func TestPerformanceServiceSampleEventAuthAndSequenceIdempotency(t *testing.T) {
	token := "sample-token"
	repo := &fakePerformanceRepo{runByTaskID: model.PerfTestRun{ID: 1, PlanID: 1, Status: model.PerfRunRunning, CallbackTokenHash: sha256Hex(token)}}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	event := model.PerfSampleEvent{Sequence: 1, WindowMs: 2000, StatusCodes: map[string]int{"200": 3}}
	if err := svc.HandleSampleEvent(context.Background(), "task-1", token, event); err != nil {
		t.Fatalf("HandleSampleEvent returned error: %v", err)
	}
	if err := svc.HandleSampleEvent(context.Background(), "task-1", token, event); err != nil {
		t.Fatalf("duplicate sample should be ignored: %v", err)
	}
	series, ok := svc.events.Snapshot(1)
	if !ok || len(series.Points) != 1 || series.LastSequence != 1 {
		t.Fatalf("unexpected hub series: %+v", series)
	}
	if err := svc.HandleSampleEvent(context.Background(), "task-1", "wrong", event); err == nil || err.Error() != "回调凭据无效" {
		t.Fatalf("expected sample auth failure, got %v", err)
	}
}

func TestNormalizePerfSeriesCapsPoints(t *testing.T) {
	points := make([]model.PerfSampleEvent, 3001)
	for index := range points {
		points[index].Sequence = int64(index + 1)
	}
	raw, _ := json.Marshal(model.PerfSeries{Version: 1, Points: points})
	var series model.PerfSeries
	_ = json.Unmarshal(normalizePerfSeries(raw), &series)
	if len(series.Points) != 3000 || series.Points[0].Sequence != 1 || series.Points[len(series.Points)-1].Sequence != 3001 {
		t.Fatalf("series was not downsampled with endpoints: len=%d first=%d last=%d", len(series.Points), len(series.Points), series.Points[len(series.Points)-1].Sequence)
	}
}

func TestPerformanceServiceHandleCallbackInvalidToken(t *testing.T) {
	repo := &fakePerformanceRepo{
		runByTaskID: model.PerfTestRun{ID: 1, PlanID: 1, Status: "running", CallbackTokenHash: sha256Hex("correct")},
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	req := model.PerfCallbackRequest{Status: "running"}
	if err := svc.HandleCallback(context.Background(), "task-1", "wrong", req); err == nil || err.Error() != "回调凭据无效" {
		t.Fatalf("expected unauthorized error, got %v", err)
	}
}

func TestPerformanceServiceHandleCallbackGone(t *testing.T) {
	token := "secret-token"
	repo := &fakePerformanceRepo{
		runByTaskID: model.PerfTestRun{ID: 1, PlanID: 1, Status: "completed", CallbackTokenHash: sha256Hex(token)},
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	req := model.PerfCallbackRequest{Status: "completed"}
	if err := svc.HandleCallback(context.Background(), "task-1", token, req); err == nil || err.Error() != "执行已终结" {
		t.Fatalf("expected gone error, got %v", err)
	}
}

func TestPerformanceServiceHandleCallbackConflict(t *testing.T) {
	token := "secret-token"
	repo := &fakePerformanceRepo{
		runByTaskID:      model.PerfTestRun{ID: 1, PlanID: 1, Status: "dispatching", CallbackTokenHash: sha256Hex(token)},
		updateStatusRows: 0,
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	req := model.PerfCallbackRequest{Status: "running"}
	if err := svc.HandleCallback(context.Background(), "task-1", token, req); err == nil || err.Error() != "回调状态冲突" {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestPerformanceServiceHandleCallbackDuplicateRunningIsIdempotent(t *testing.T) {
	token := "secret-token"
	repo := &fakePerformanceRepo{runByTaskID: model.PerfTestRun{ID: 1, PlanID: 1, Status: model.PerfRunRunning, CallbackTokenHash: sha256Hex(token)}}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	if err := svc.HandleCallback(context.Background(), "task-1", token, model.PerfCallbackRequest{Status: "running"}); err != nil {
		t.Fatalf("duplicate running callback should be idempotent: %v", err)
	}
}

func TestPerformanceServiceStoppingCallbackKeepsCancelIntent(t *testing.T) {
	token := "secret-token"
	repo := &fakePerformanceRepo{
		runByTaskID:      model.PerfTestRun{ID: 1, PlanID: 1, Status: model.PerfRunStopping, CallbackTokenHash: sha256Hex(token)},
		updateResultRows: 1,
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	if err := svc.HandleCallback(context.Background(), "task-1", token, model.PerfCallbackRequest{Status: "completed"}); err == nil || err.Error() != "回调状态冲突" {
		t.Fatalf("stopping should reject completed callback, got %v", err)
	}
	if err := svc.HandleCallback(context.Background(), "task-1", token, model.PerfCallbackRequest{Status: "canceled"}); err != nil {
		t.Fatalf("stopping canceled callback should persist partial result: %v", err)
	}
	if repo.updateResultTo != model.PerfRunCanceled {
		t.Fatalf("expected canceled result, got %s", repo.updateResultTo)
	}
}

func TestPerformanceServiceHandleCallbackThresholdFailed(t *testing.T) {
	token := "secret-token"
	repo := &fakePerformanceRepo{
		runByTaskID:      model.PerfTestRun{ID: 1, PlanID: 1, Status: "running", CallbackTokenHash: sha256Hex(token)},
		updateResultRows: 1,
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	req := model.PerfCallbackRequest{
		Status:  "completed",
		Summary: json.RawMessage(`{"thresholds":{"http_req_duration":{"p(95)":{"ok":false}}}}`),
	}
	if err := svc.HandleCallback(context.Background(), "task-1", token, req); err != nil {
		t.Fatalf("HandleCallback returned error: %v", err)
	}
	if repo.updateResultTo != model.PerfRunThresholdFailed {
		t.Fatalf("expected threshold_failed, got %s", repo.updateResultTo)
	}
}

func TestPerformanceServiceHandleCallbackCompleted(t *testing.T) {
	token := "secret-token"
	repo := &fakePerformanceRepo{
		runByTaskID:      model.PerfTestRun{ID: 1, PlanID: 1, Status: "running", CallbackTokenHash: sha256Hex(token)},
		updateResultRows: 1,
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	req := model.PerfCallbackRequest{
		Status:  "completed",
		Summary: json.RawMessage(`{"thresholds":{"http_req_duration":{"p(95)":{"ok":true}}}}`),
	}
	if err := svc.HandleCallback(context.Background(), "task-1", token, req); err != nil {
		t.Fatalf("HandleCallback returned error: %v", err)
	}
	if repo.updateResultTo != model.PerfRunCompleted {
		t.Fatalf("expected completed, got %s", repo.updateResultTo)
	}
}

func TestPerformanceServiceHandleCallbackMapsPerfPayload(t *testing.T) {
	exitCode, durationMs := 0, 3456
	avg, p95, errorRate, rps := 3.0, 5.0, 0.0, 100.0
	summary := json.RawMessage(`{"generator_version":"perf-1.0.0","k6_version":"v1.0.0","thresholds":[{"ok":true}],"metrics":{"total_requests":3571,"avg_duration_ms":3,"p95_duration_ms":5,"error_rate":0,"rps":100}}`)
	repo := &fakePerformanceRepo{
		runByTaskID:      model.PerfTestRun{ID: 1, PlanID: 1, Status: model.PerfRunRunning, CallbackTokenHash: sha256Hex("secret-token")},
		updateResultRows: 1,
	}
	svc := NewPerformanceService(repo, nil, nil, "http://localhost")
	req := model.PerfCallbackRequest{
		Status: "completed", GeneratorVersion: "perf-1.0.0", K6Version: "v1.0.0",
		ExitCode: &exitCode, DurationMs: &durationMs, TotalRequests: 3571,
		AvgDurationMs: &avg, P95DurationMs: &p95, ErrorRate: &errorRate, RPS: &rps,
		Summary: summary, DiagnosticOutput: "ok",
	}
	if err := svc.HandleCallback(context.Background(), "task-1", "secret-token", req); err != nil {
		t.Fatalf("HandleCallback returned error: %v", err)
	}
	if repo.updateResultTo != model.PerfRunCompleted || repo.updateResult.TotalRequests != 3571 || repo.updateResult.ExitCode == nil || *repo.updateResult.ExitCode != 0 || repo.updateResult.K6Version != "v1.0.0" {
		t.Fatalf("perf payload was not mapped: status=%s result=%+v", repo.updateResultTo, repo.updateResult)
	}
	var gotSummary map[string]any
	if err := json.Unmarshal(repo.updateResult.Summary, &gotSummary); err != nil || gotSummary["k6_version"] != "v1.0.0" || repo.updateResult.DurationMs == nil || *repo.updateResult.DurationMs != 3456 {
		t.Fatalf("summary/duration were not preserved: summary=%s duration=%v", repo.updateResult.Summary, repo.updateResult.DurationMs)
	}
}

func TestPerformanceServiceHandleCallbackPreservesTerminalMapping(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		summary  string
		expected string
	}{
		{name: "completed", status: "completed", summary: `{"thresholds":[{"ok":true}]}`, expected: model.PerfRunCompleted},
		{name: "threshold_failed", status: "completed", summary: `{"thresholds":[{"ok":false}]}`, expected: model.PerfRunThresholdFailed},
		{name: "explicit_threshold_failed", status: "threshold_failed", summary: `{}`, expected: model.PerfRunThresholdFailed},
		{name: "execution_failed", status: "execution_failed", summary: `{}`, expected: model.PerfRunExecutionFailed},
		{name: "timed_out", status: "timed_out", summary: `{}`, expected: model.PerfRunTimedOut},
		{name: "canceled", status: "canceled", summary: `{}`, expected: model.PerfRunCanceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakePerformanceRepo{
				runByTaskID:      model.PerfTestRun{ID: 1, PlanID: 1, Status: model.PerfRunRunning, CallbackTokenHash: sha256Hex("secret-token")},
				updateResultRows: 1,
			}
			svc := NewPerformanceService(repo, nil, nil, "http://localhost")
			if err := svc.HandleCallback(context.Background(), "task-1", "secret-token", model.PerfCallbackRequest{Status: tc.status, Summary: json.RawMessage(tc.summary)}); err != nil {
				t.Fatalf("HandleCallback returned error: %v", err)
			}
			if repo.updateResultTo != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, repo.updateResultTo)
			}
		})
	}
}

func TestPerformanceServiceRepositoryErrors(t *testing.T) {
	req := validPerfPlanRequest()
	if _, err := NewPerformanceService(&fakePerformanceRepo{productExists: true, createErr: errors.New("duplicate")}, nil, nil, "http://localhost").CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected create repository error")
	}
	if err := NewPerformanceService(&fakePerformanceRepo{productExists: true, updateErr: errors.New("duplicate")}, nil, nil, "http://localhost").UpdatePlan(context.Background(), "admin", 1, req); err == nil {
		t.Fatal("expected update repository error")
	}
	if err := NewPerformanceService(&fakePerformanceRepo{deleteErr: errors.New("db")}, nil, nil, "http://localhost").DeletePlan(context.Background(), "admin", 1); err == nil {
		t.Fatal("expected delete repository error")
	}
}

func TestGenerateCallbackTokenFailsClosedWhenRandomSourceFails(t *testing.T) {
	original := perfRandRead
	perfRandRead = func([]byte) (int, error) { return 0, errors.New("random source unavailable") }
	defer func() { perfRandRead = original }()
	if token, err := generateCallbackToken(); err == nil || token != "" {
		t.Fatalf("expected callback token generation to fail closed, token=%q err=%v", token, err)
	}
}

func TestPerformanceServiceListAndGet(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil, nil, "http://localhost")
	result, err := svc.ListPlans(context.Background(), model.PerfTestPlanFilter{Name: " 压测 "}, 0, 500)
	if err != nil {
		t.Fatalf("ListPlans returned error: %v", err)
	}
	if result.Total != 1 || result.Page != 1 || result.PageSize != 100 {
		t.Fatalf("unexpected page result: %+v", result)
	}
	plan, err := svc.GetPlan(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPlan returned error: %v", err)
	}
	if plan.ID != 1 {
		t.Fatalf("expected plan id 1, got %d", plan.ID)
	}
	if _, err := svc.GetPlan(context.Background(), 0); err == nil {
		t.Fatal("expected invalid plan id error")
	}
	runResult, err := svc.ListRuns(context.Background(), model.PerfTestRunFilter{PlanID: "1"}, 1, 20)
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if runResult.Total != 1 {
		t.Fatalf("expected one run, got %d", runResult.Total)
	}
	run, err := svc.GetRun(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if run.ID != 1 {
		t.Fatalf("expected run id 1, got %d", run.ID)
	}
	if _, err := svc.GetRun(context.Background(), 0); err == nil {
		t.Fatal("expected invalid run id error")
	}
}

func TestPerfTotalDuration(t *testing.T) {
	if d := perfTotalDuration("baseline", json.RawMessage(`{"vus":3,"duration":"2m"}`)); d != 2*time.Minute {
		t.Fatalf("baseline expected 2m, got %v", d)
	}
	if d := perfTotalDuration("soak", json.RawMessage(`{"vus":100,"duration":"1h"}`)); d != time.Hour {
		t.Fatalf("soak expected 1h, got %v", d)
	}
	if d := perfTotalDuration("baseline", json.RawMessage(`{"vus":3,"duration":"10m30s"}`)); d != 630*time.Second {
		t.Fatalf("compound duration expected 630s, got %v", d)
	}
	if d := perfTotalDuration("ramp", json.RawMessage(`{"stages":[{"duration":"1m","target":10},{"duration":"30s","target":50}]}`)); d != 90*time.Second {
		t.Fatalf("ramp stages expected 90s, got %v", d)
	}
	if d := perfTotalDuration("peak", json.RawMessage(`{"peakVus":150,"rampDuration":"2m","holdDuration":"30m","rampDownDuration":"1m"}`)); d != 33*time.Minute {
		t.Fatalf("peak segments expected 33m, got %v", d)
	}
	if d := perfTotalDuration("baseline", json.RawMessage(`{}`)); d != perfDefaultDuration {
		t.Fatalf("unparseable expected default %v, got %v", perfDefaultDuration, d)
	}
}

func TestComputeExpectedFinishAt(t *testing.T) {
	plan := defaultPerfPlan(1) // baseline, duration=2m
	requested := time.Now()
	expected := computeExpectedFinishAt(plan, requested)
	want := requested.Add(2*time.Minute + perfTimeoutBuffer)
	if !expected.Equal(want) {
		t.Fatalf("expected %v, got %v", want, expected)
	}
}

func TestPerformanceServiceStartSmoke(t *testing.T) {
	var gotType, gotMode, gotTarget string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotType, _ = body["type"].(string)
		payload, _ := body["payload"].(map[string]any)
		gotMode, _ = payload["mode"].(string)
		gotTarget, _ = payload["target"].(string)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	execRepo := &fakeExecutorRepo{executors: []model.ExecutorView{{
		ExecutorID: "exec-1", Endpoint: server.URL, Status: "online", SupportedTypes: []string{"perf"},
	}}}
	svc := NewPerformanceService(&fakePerformanceRepo{}, execRepo, nil, server.URL)
	taskID, err := svc.StartSmoke(context.Background(), "admin", model.PerfSmokeRequest{
		ExecutorID: "exec-1", TargetURL: "https://example.com", Method: "GET",
	})
	if err != nil {
		t.Fatalf("StartSmoke returned error: %v", err)
	}
	if taskID == "" {
		t.Fatal("expected non-empty taskId")
	}
	if gotType != "perf" || gotMode != "smoke" {
		t.Fatalf("unexpected submit type=%s mode=%s", gotType, gotMode)
	}
	if gotTarget != "https://example.com" {
		t.Fatalf("expected payload.target, got %q", gotTarget)
	}
}

func TestPerformanceServiceStartSmokeRejectsUnsupportedExecutor(t *testing.T) {
	execRepo := &fakeExecutorRepo{executors: []model.ExecutorView{{
		ExecutorID: "exec-1", Endpoint: "http://localhost", Status: "online", SupportedTypes: []string{"ui"},
	}}}
	svc := NewPerformanceService(&fakePerformanceRepo{}, execRepo, nil, "http://localhost")
	if _, err := svc.StartSmoke(context.Background(), "admin", model.PerfSmokeRequest{
		ExecutorID: "exec-1", TargetURL: "https://example.com",
	}); err == nil {
		t.Fatal("expected unsupported executor error")
	}
}

func TestPerformanceServiceGetSmoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","result":{"exitCode":0}}`))
	}))
	defer server.Close()
	execRepo := &fakeExecutorRepo{executors: []model.ExecutorView{{
		ExecutorID: "exec-1", Endpoint: server.URL, Status: "online", SupportedTypes: []string{"perf"},
	}}}
	svc := NewPerformanceService(&fakePerformanceRepo{}, execRepo, nil, server.URL)
	svc.recordSmoke("task-1", "exec-1")
	result, err := svc.GetSmoke(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("GetSmoke returned error: %v", err)
	}
	if status, _ := result["status"].(string); status != "success" {
		t.Fatalf("expected success status, got %v", result)
	}
}

func TestPerformanceServiceRecoverDispatchingTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	deadline := time.Now().Add(-time.Minute)
	repo := &fakePerformanceRepo{
		updateResultRows: 1,
		listRunsReturn: []model.PerfTestRun{{
			ID: 1, PlanID: 1, Status: "dispatching", ExecutorID: "exec-1", TaskID: "task-1",
			DispatchDeadlineAt: &deadline,
		}},
	}
	execRepo := &fakeExecutorRepo{executors: []model.ExecutorView{{
		ExecutorID: "exec-1", Endpoint: server.URL, Status: "online", SupportedTypes: []string{"perf"},
	}}}
	svc := NewPerformanceService(repo, execRepo, nil, server.URL)
	if err := svc.RecoverStale(context.Background()); err != nil {
		t.Fatalf("RecoverStale returned error: %v", err)
	}
	if repo.updateResultTo != model.PerfRunExecutionFailed {
		t.Fatalf("expected execution_failed, got %s", repo.updateResultTo)
	}
}

func TestPerformanceServiceRecoveryPersistsExecutorTerminalResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		output, _ := json.Marshal(map[string]any{
			"terminal_status": "completed",
			"metrics":         map[string]any{"total_requests": 9, "avg_duration_ms": 2, "p95_duration_ms": 4, "error_rate": 0, "rps": 3},
			"summary":         map[string]any{"metrics": map[string]any{"http_reqs": map[string]any{"values": map[string]any{"count": 9}}}},
			"series":          map[string]any{"version": 1, "points": []any{}, "partial": false},
		})
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "result": map[string]any{"exitCode": 0, "output": string(output)}})
	}))
	defer server.Close()
	deadline := time.Now().Add(-time.Minute)
	repo := &fakePerformanceRepo{updateResultRows: 1, listRunsReturn: []model.PerfTestRun{{ID: 1, Status: model.PerfRunDispatched, ExecutorID: "exec-1", TaskID: "task-1", StartDeadlineAt: &deadline}}}
	execRepo := &fakeExecutorRepo{executors: []model.ExecutorView{{ExecutorID: "exec-1", Endpoint: server.URL}}}
	svc := NewPerformanceService(repo, execRepo, nil, server.URL)
	if err := svc.RecoverStale(context.Background()); err != nil {
		t.Fatalf("RecoverStale returned error: %v", err)
	}
	if repo.updateResultTo != model.PerfRunCompleted || repo.updateResult.TotalRequests != 9 {
		t.Fatalf("expected recovered completed result, status=%s result=%+v", repo.updateResultTo, repo.updateResult)
	}
}

func TestPerformanceServiceRecoverRunningTimeoutNeedsAttention(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"running"}`))
	}))
	defer server.Close()
	expected := time.Now().Add(-time.Minute)
	repo := &fakePerformanceRepo{
		updateResultRows: 1,
		listRunsReturn: []model.PerfTestRun{{
			ID: 1, PlanID: 1, Status: "running", ExecutorID: "exec-1", TaskID: "task-1",
			ExpectedFinishAt: &expected,
		}},
	}
	execRepo := &fakeExecutorRepo{executors: []model.ExecutorView{{
		ExecutorID: "exec-1", Endpoint: server.URL, Status: "online", SupportedTypes: []string{"perf"},
	}}}
	svc := NewPerformanceService(repo, execRepo, nil, server.URL)
	if err := svc.RecoverStale(context.Background()); err != nil {
		t.Fatalf("RecoverStale returned error: %v", err)
	}
	if repo.updateResultTo != "" {
		t.Fatalf("running 仍存在应仅标记 needs_attention，不应写终态，got %s", repo.updateResultTo)
	}
}

func TestPerformanceServiceRecoverStoppingPersistsPartialCanceledResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		output, _ := json.Marshal(map[string]any{
			"terminal_status": "canceled",
			"metrics":         map[string]any{"total_requests": 5, "avg_duration_ms": 2, "p95_duration_ms": 4, "error_rate": 0, "rps": 5},
			"summary":         map[string]any{"metrics": map[string]any{"http_reqs": map[string]any{"values": map[string]any{"count": 5}}}, "options": map[string]any{}, "state": map[string]any{}},
			"failure_stage":   "cancel",
			"diagnostic":      "partial",
		})
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "canceled", "result": map[string]any{"exitCode": 1, "error": "任务已取消", "output": string(output)}})
	}))
	defer server.Close()
	updatedAt := time.Now().Add(-time.Minute)
	repo := &fakePerformanceRepo{
		updateResultRows: 1,
		listRunsReturn: []model.PerfTestRun{{
			ID: 1, PlanID: 1, Status: model.PerfRunStopping, ExecutorID: "exec-1", TaskID: "task-1", UpdatedAt: updatedAt,
		}},
	}
	execRepo := &fakeExecutorRepo{executors: []model.ExecutorView{{ExecutorID: "exec-1", Endpoint: server.URL}}}
	svc := NewPerformanceService(repo, execRepo, nil, server.URL)
	if err := svc.RecoverStale(context.Background()); err != nil {
		t.Fatalf("RecoverStale returned error: %v", err)
	}
	if repo.updateResultTo != model.PerfRunCanceled || repo.updateResult.TotalRequests != 5 {
		t.Fatalf("expected canceled partial result, status=%s result=%+v", repo.updateResultTo, repo.updateResult)
	}
}

func TestBuildPerfPayload(t *testing.T) {
	snapshot := json.RawMessage(`{"planId":1,"name":"登录","targetUrl":"https://example.com/login","method":"GET","headers":{"Authorization":"{{secret.token}}"},"body":"","scenarioType":"baseline","loadConfig":{"vus":3,"duration":"2m"},"environment":"test","thresholds":[{"metric":"http_req_duration","aggregation":"p(95)","operator":"<","value":500}]}`)
	run := model.PerfTestRun{ID: 1, PlanID: 1, ScenarioType: "baseline", Environment: "test", PlanSnapshot: snapshot}
	payload, err := buildPerfPayload(run)
	if err != nil {
		t.Fatalf("buildPerfPayload returned error: %v", err)
	}
	for _, key := range []string{"scenario_type", "load_config", "target", "method", "headers", "body", "thresholds"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload 缺少字段 %s: %+v", key, payload)
		}
	}
	if _, ok := payload["snapshot"]; ok {
		t.Fatal("payload 不应包含 snapshot 字段")
	}
	if payload["target"] != "https://example.com/login" {
		t.Fatalf("expected target, got %v", payload["target"])
	}
	if payload["scenario_type"] != "baseline" {
		t.Fatalf("expected baseline, got %v", payload["scenario_type"])
	}
}
