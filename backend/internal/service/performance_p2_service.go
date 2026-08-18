package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"synapseqa/backend/internal/model"
)

type performanceP2Repository interface {
	ListPerfEnvironments(context.Context, int64) ([]model.PerfEnvironment, error)
	GetPerfEnvironment(context.Context, int64) (model.PerfEnvironment, error)
	CreatePlanWithEnvironment(context.Context, model.PerfTestPlanRequest, string) (int64, error)
	UpdatePlanWithEnvironment(context.Context, int64, model.PerfTestPlanRequest) (int64, error)
	GetPerfBaseline(context.Context, int64, string, string, *int64) (model.PerfBaseline, error)
	GetPerfBaselineByRun(context.Context, int64) (model.PerfBaseline, error)
	UpsertPerfBaseline(context.Context, model.PerfBaseline) (model.PerfBaseline, error)
	DeletePerfBaseline(context.Context, int64) error
	ListPerfTrendRuns(context.Context, int64, string, string, *int64, int) ([]model.PerfTestRun, error)
	GetActivePerfRun(context.Context, int64) (model.PerfTestRun, error)
	ListPerfDegradationCandidates(context.Context, int) ([]model.PerfTestRun, error)
	MarkPerfDegradationChecked(context.Context, int64) (int64, error)
	ListPerfSchedules(context.Context, model.PerfScheduleFilter, int, int) ([]model.PerfSchedule, int64, error)
	GetPerfSchedule(context.Context, int64) (model.PerfSchedule, error)
	CreatePerfSchedule(context.Context, model.PerfSchedule) (model.PerfSchedule, error)
	UpdatePerfSchedule(context.Context, model.PerfSchedule) (int64, error)
	SetPerfScheduleEnabled(context.Context, int64, bool, *time.Time, string) (int64, error)
	SoftDeletePerfSchedule(context.Context, int64, string) (int64, error)
	ClaimPerfSchedules(context.Context, string, time.Time, int) ([]model.PerfSchedule, error)
	FinishPerfSchedule(context.Context, int64, string, *time.Time, *time.Time, string, *int64, string) (int64, error)
	SetPerfScheduleError(context.Context, int64, string, string) (int64, error)
}

var ErrPerfComparisonNotFinal = errors.New("只有 completed 或 threshold_failed 执行支持比较")
var ErrPerfBaselineNotCurrent = errors.New("目标执行记录不是当前维度的基线")

func (s *PerformanceService) p2Repository() (performanceP2Repository, error) {
	repo, ok := s.performanceRepo.(performanceP2Repository)
	if !ok {
		return nil, errors.New("性能测试 P2 数据仓储未初始化")
	}
	return repo, nil
}

func (s *PerformanceService) ListPerfEnvironments(ctx context.Context, productID int64) ([]model.PerfEnvironment, error) {
	if productID <= 0 {
		return nil, errors.New("产品 ID 无效")
	}
	repo, err := s.p2Repository()
	if err != nil {
		return nil, err
	}
	items, err := repo.ListPerfEnvironments(ctx, productID)
	if err != nil {
		return nil, err
	}
	for index := range items {
		if err := validateEnvironmentBaseURL(items[index].BaseURL); err != nil {
			items[index].BaseURL = ""
		}
	}
	return items, nil
}

func (s *PerformanceService) resolvePerfEnvironment(ctx context.Context, productID int64, environmentID *int64) (model.PerfEnvironment, error) {
	if environmentID == nil || *environmentID <= 0 {
		return model.PerfEnvironment{}, errors.New("环境 ID 无效")
	}
	repo, err := s.p2Repository()
	if err != nil {
		return model.PerfEnvironment{}, err
	}
	item, err := repo.GetPerfEnvironment(ctx, *environmentID)
	if err != nil || item.ProductID != productID {
		return model.PerfEnvironment{}, errors.New("环境不属于当前产品")
	}
	if err := validateEnvironmentBaseURL(item.BaseURL); err != nil {
		return model.PerfEnvironment{}, err
	}
	return item, nil
}

func validateEnvironmentBaseURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("环境 Base URL 必须是无用户信息、查询参数和片段的绝对 HTTP(S) 地址")
	}
	return nil
}

func resolvePerfTarget(baseValue, targetValue string) (string, error) {
	base := strings.TrimSpace(baseValue)
	if err := validateEnvironmentBaseURL(base); err != nil {
		return "", err
	}
	baseURL, _ := url.Parse(base)
	target := strings.TrimSpace(targetValue)
	if target == "" {
		return baseURL.String(), nil
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Fragment != "" {
		return "", errors.New("目标 URL 无效")
	}
	if parsed.IsAbs() {
		if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return "", errors.New("目标 URL 必须是 HTTP(S) 地址")
		}
		parsed.Scheme = baseURL.Scheme
		parsed.Host = baseURL.Host
		return parsed.String(), nil
	}
	baseForDirectory := *baseURL
	if !strings.HasSuffix(baseForDirectory.Path, "/") {
		baseForDirectory.Path += "/"
	}
	return baseForDirectory.ResolveReference(parsed).String(), nil
}

func resolvePerfPlanURLs(plan *model.PerfTestPlan) error {
	if plan.EnvironmentID == nil {
		return nil
	}
	resolvedTarget, err := resolvePerfTarget(plan.EnvironmentBaseURL, plan.TargetURL)
	if err != nil {
		return err
	}
	plan.TargetURL = resolvedTarget
	if plan.ScenarioType != "mixed" {
		return nil
	}
	var config map[string]any
	if err := json.Unmarshal(plan.LoadConfig, &config); err != nil {
		return errors.New("混合场景负载配置无效")
	}
	items, ok := config["scenarios"].([]any)
	if !ok {
		return errors.New("混合场景需要配置 scenarios")
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return errors.New("混合场景接口配置无效")
		}
		value, _ := item["url"].(string)
		if parsed, parseErr := url.Parse(strings.TrimSpace(value)); parseErr == nil && parsed.IsAbs() {
			if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
				return errors.New("mixed scenario URL 只允许 HTTP(S) 地址")
			}
			continue
		}
		resolved, resolveErr := resolvePerfTarget(resolvedTarget, value)
		if resolveErr != nil {
			return resolveErr
		}
		item["url"] = resolved
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	plan.LoadConfig = encoded
	return nil
}

func (s *PerformanceService) SetPlanEnvironment(ctx context.Context, req *model.PerfTestPlanRequest) error {
	if req.EnvironmentID == nil {
		return nil
	}
	item, err := s.resolvePerfEnvironment(ctx, req.ProductID, req.EnvironmentID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.Environment) == "" || req.Environment == "test" {
		req.Environment = item.EnvName
	}
	return nil
}

func (s *PerformanceService) GetPerfBaseline(ctx context.Context, runID int64) (model.PerfBaselineResponse, error) {
	run, err := s.performanceRepo.GetRun(ctx, runID)
	if err != nil {
		return model.PerfBaselineResponse{}, errors.New("执行记录不存在")
	}
	repo, err := s.p2Repository()
	if err != nil {
		return model.PerfBaselineResponse{}, err
	}
	baseline, err := repo.GetPerfBaseline(ctx, run.PlanID, run.ScenarioType, run.Environment, run.EnvironmentID)
	if err != nil {
		return model.PerfBaselineResponse{Exists: false, IsCurrentRun: false, Baseline: nil}, nil
	}
	return model.PerfBaselineResponse{Exists: true, IsCurrentRun: baseline.RunID == runID, Baseline: &baseline}, nil
}

func (s *PerformanceService) SetPerfBaseline(ctx context.Context, actor string, runID int64) (model.PerfBaselineResponse, error) {
	run, err := s.performanceRepo.GetRun(ctx, runID)
	if err != nil {
		return model.PerfBaselineResponse{}, errors.New("执行记录不存在")
	}
	if err := validateBaselineRun(run); err != nil {
		return model.PerfBaselineResponse{}, err
	}
	plan, err := s.performanceRepo.GetPlan(ctx, run.PlanID)
	if err != nil || plan.Status == "deleted" {
		return model.PerfBaselineResponse{}, errors.New("性能测试方案不存在或已删除")
	}
	repo, err := s.p2Repository()
	if err != nil {
		return model.PerfBaselineResponse{}, err
	}
	baseline, err := repo.UpsertPerfBaseline(ctx, model.PerfBaseline{PlanID: run.PlanID, ScenarioType: run.ScenarioType, EnvironmentID: run.EnvironmentID, Environment: run.Environment, RunID: run.ID, SetBy: actor})
	if err != nil {
		return model.PerfBaselineResponse{}, err
	}
	return model.PerfBaselineResponse{Exists: true, IsCurrentRun: true, Baseline: &baseline}, nil
}

func (s *PerformanceService) DeletePerfBaseline(ctx context.Context, runID int64) (model.PerfBaselineResponse, error) {
	run, err := s.performanceRepo.GetRun(ctx, runID)
	if err != nil {
		return model.PerfBaselineResponse{}, errors.New("执行记录不存在")
	}
	repo, err := s.p2Repository()
	if err != nil {
		return model.PerfBaselineResponse{}, err
	}
	baseline, err := repo.GetPerfBaselineByRun(ctx, runID)
	if err != nil || baseline.PlanID != run.PlanID || baseline.ScenarioType != run.ScenarioType || baseline.Environment != run.Environment {
		current, currentErr := s.GetPerfBaseline(ctx, runID)
		if currentErr != nil {
			return model.PerfBaselineResponse{}, currentErr
		}
		if current.Exists {
			return model.PerfBaselineResponse{}, ErrPerfBaselineNotCurrent
		}
		return current, nil
	}
	if err := repo.DeletePerfBaseline(ctx, baseline.ID); err != nil {
		return model.PerfBaselineResponse{}, err
	}
	return model.PerfBaselineResponse{Exists: false, IsCurrentRun: false, Baseline: nil}, nil
}

func validateBaselineRun(run model.PerfTestRun) error {
	if run.Status != model.PerfRunCompleted || run.TotalRequests <= 0 || run.P95DurationMs == nil || *run.P95DurationMs < 0 || run.P99DurationMs == nil || *run.P99DurationMs < 0 || run.ErrorRate == nil || *run.ErrorRate < 0 || *run.ErrorRate > 1 || run.RPS == nil || *run.RPS <= 0 {
		return errors.New("只有指标完整且通过的 completed 执行才能设置基线")
	}
	return nil
}

func (s *PerformanceService) ComparePerfRun(ctx context.Context, runID int64) (model.PerfComparison, error) {
	run, err := s.performanceRepo.GetRun(ctx, runID)
	if err != nil {
		return model.PerfComparison{}, errors.New("执行记录不存在")
	}
	if run.Status != model.PerfRunCompleted && run.Status != model.PerfRunThresholdFailed {
		return model.PerfComparison{}, ErrPerfComparisonNotFinal
	}
	repo, err := s.p2Repository()
	if err != nil {
		return model.PerfComparison{}, err
	}
	baseline, err := repo.GetPerfBaseline(ctx, run.PlanID, run.ScenarioType, run.Environment, run.EnvironmentID)
	if err != nil {
		return model.PerfComparison{CurrentRunID: run.ID, Threshold: 0.10, Comparable: false, DegradedMetrics: []string{}, Metrics: model.PerfComparisonMetrics{
			P95DurationMs: emptyComparisonMetric("lower", "无基线"), P99DurationMs: emptyComparisonMetric("lower", "无基线"), ErrorRate: emptyComparisonMetric("lower", "无基线"), RPS: emptyComparisonMetric("higher", "无基线"),
		}}, nil
	}
	baseRun, err := s.performanceRepo.GetRun(ctx, baseline.RunID)
	if err != nil {
		return model.PerfComparison{}, errors.New("基线执行记录不存在")
	}
	baselineID := baseRun.ID
	comparison := model.PerfComparison{CurrentRunID: run.ID, BaselineRunID: &baselineID, Threshold: 0.10, Comparable: true, DegradedMetrics: []string{}}
	comparison.Metrics.P95DurationMs = compareMetric("lower", run.P95DurationMs, baseRun.P95DurationMs, comparison.Threshold)
	comparison.Metrics.P99DurationMs = compareMetric("lower", run.P99DurationMs, baseRun.P99DurationMs, comparison.Threshold)
	comparison.Metrics.ErrorRate = compareMetric("lower", run.ErrorRate, baseRun.ErrorRate, comparison.Threshold)
	comparison.Metrics.RPS = compareMetric("higher", run.RPS, baseRun.RPS, comparison.Threshold)
	for name, metric := range map[string]model.PerfComparisonMetric{"p95DurationMs": comparison.Metrics.P95DurationMs, "p99DurationMs": comparison.Metrics.P99DurationMs, "errorRate": comparison.Metrics.ErrorRate, "rps": comparison.Metrics.RPS} {
		if metric.Degraded {
			comparison.DegradedMetrics = append(comparison.DegradedMetrics, name)
		}
	}
	sort.Strings(comparison.DegradedMetrics)
	comparison.Degraded = len(comparison.DegradedMetrics) > 0
	return comparison, nil
}

func compareMetric(direction string, current, baseline *float64, threshold float64) model.PerfComparisonMetric {
	result := model.PerfComparisonMetric{Direction: direction, Current: current, Baseline: baseline}
	if current == nil || baseline == nil {
		result.Reason = "指标缺失"
		return result
	}
	result.Comparable = true
	delta := *current - *baseline
	result.Delta = &delta
	if *baseline != 0 {
		change := delta / *baseline
		result.ChangeRate = &change
		result.RateAvailable = true
		if direction == "higher" {
			result.DegradationRate = floatPtr(-change)
			result.Degraded = change < -threshold
		} else {
			result.DegradationRate = floatPtr(change)
			result.Degraded = change > threshold
		}
		if result.Degraded {
			result.Reason = "超过劣化阈值"
		} else {
			result.Reason = "未超过劣化阈值"
		}
	} else if *current == 0 {
		zero := 0.0
		result.RateAvailable = true
		result.ChangeRate = &zero
		result.DegradationRate = &zero
		result.Reason = "基线与当前均为 0"
	} else if direction == "higher" {
		result.Comparable = false
		result.Reason = "RPS 基线为 0，无法比较"
	} else {
		result.Degraded = true
		result.Reason = "基线为 0 且当前值大于 0"
	}
	return result
}

func emptyComparisonMetric(direction, reason string) model.PerfComparisonMetric {
	return model.PerfComparisonMetric{Direction: direction, Reason: reason}
}

func floatPtr(value float64) *float64 { return &value }

func firstFloat(primary, fallback *float64) *float64 {
	if primary != nil {
		return primary
	}
	return fallback
}

func perfP99FromSummary(raw json.RawMessage) *float64 {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	metrics, _ := value["metrics"].(map[string]any)
	duration, _ := metrics["http_req_duration"].(map[string]any)
	values, _ := duration["values"].(map[string]any)
	rawValue := values["p(99)"]
	if rawValue == nil {
		if legacyDuration, ok := duration["p99_duration_ms"].(float64); ok {
			rawValue = legacyDuration
		} else if legacyDuration, ok := duration["p99_duration_ms"].(json.Number); ok {
			rawValue = legacyDuration
		}
		if rawValue == nil {
			if legacyDuration, ok := value["http_req_duration"].(map[string]any); ok {
				rawValue = legacyDuration["p99_duration_ms"]
			}
		}
	}
	if rawValue == nil {
		rawValue = metrics["p99_duration_ms"]
	}
	switch item := rawValue.(type) {
	case float64:
		return &item
	case json.Number:
		value, err := item.Float64()
		if err == nil {
			return &value
		}
	}
	return nil
}

func (s *PerformanceService) TrendPerfRun(ctx context.Context, runID, limit int64) (model.PerfTrendResponse, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		return model.PerfTrendResponse{}, errors.New("trend limit 不能超过 100")
	}
	run, err := s.performanceRepo.GetRun(ctx, runID)
	if err != nil {
		return model.PerfTrendResponse{}, errors.New("执行记录不存在")
	}
	repo, err := s.p2Repository()
	if err != nil {
		return model.PerfTrendResponse{}, err
	}
	items, err := repo.ListPerfTrendRuns(ctx, run.PlanID, run.ScenarioType, run.Environment, run.EnvironmentID, int(limit))
	if err != nil {
		return model.PerfTrendResponse{}, err
	}
	baseline, _ := repo.GetPerfBaseline(ctx, run.PlanID, run.ScenarioType, run.Environment, run.EnvironmentID)
	points := make([]model.PerfTrendPoint, 0, len(items))
	for _, item := range items {
		hydrateRunEnvironmentFromSnapshot(&item)
		comparison, _ := s.ComparePerfRun(ctx, item.ID)
		degradedMetrics := comparison.DegradedMetrics
		points = append(points, model.PerfTrendPoint{RunID: item.ID, FinishedAt: item.FinishedAt, Status: item.Status, TotalRequests: item.TotalRequests, P95DurationMs: item.P95DurationMs, P99DurationMs: item.P99DurationMs, ErrorRate: item.ErrorRate, RPS: item.RPS, IsBaseline: item.ID == baseline.RunID, Degraded: comparison.Degraded, DegradedMetrics: degradedMetrics})
	}
	sort.SliceStable(points, func(i, j int) bool {
		if points[i].FinishedAt == nil {
			return false
		}
		if points[j].FinishedAt == nil {
			return true
		}
		if points[i].FinishedAt.Equal(*points[j].FinishedAt) {
			return points[i].RunID < points[j].RunID
		}
		return points[i].FinishedAt.Before(*points[j].FinishedAt)
	})
	var baselineID *int64
	if baseline.ID != 0 {
		baselineID = &baseline.RunID
	}
	hydrateRunEnvironmentFromSnapshot(&run)
	return model.PerfTrendResponse{PlanID: run.PlanID, ScenarioType: run.ScenarioType, EnvironmentID: run.EnvironmentID, Environment: run.Environment, EnvironmentName: run.EnvironmentName, EnvironmentBaseURL: run.EnvironmentBaseURL, EnvironmentDeployEnv: run.EnvironmentDeployEnv, BaselineRunID: baselineID, Limit: int(limit), Items: points}, nil
}

func (s *PerformanceService) ExportPerfRun(ctx context.Context, runID int64, format string) ([]byte, string, string, error) {
	run, err := s.performanceRepo.GetRun(ctx, runID)
	if err != nil {
		return nil, "", "", errors.New("执行记录不存在")
	}
	hydrateRunEnvironmentFromSnapshot(&run)
	if !isFinalRunStatus(run.Status) {
		return nil, "", "", errors.New("非终态执行不能导出")
	}
	plan, _ := s.performanceRepo.GetPlan(ctx, run.PlanID)
	comparison, _ := s.ComparePerfRun(ctx, runID)
	snapshot := sanitizePerfJSON(run.PlanSnapshot)
	resolvedTargetURL := snapshotString(run.PlanSnapshot, "resolvedTargetUrl")
	if strings.EqualFold(format, "json") {
		runRaw, _ := json.Marshal(run)
		var runData any
		_ = json.Unmarshal(sanitizePerfJSON(runRaw), &runData)
		payload := map[string]any{"schemaVersion": "perf-report.v1", "exportedAt": time.Now().UTC().Format(time.RFC3339Nano), "run": runData, "environment": map[string]any{"environmentId": run.EnvironmentID, "name": run.EnvironmentName, "baseUrl": sanitizePerfText(run.EnvironmentBaseURL), "resolvedTargetUrl": sanitizePerfText(resolvedTargetURL)}, "planSnapshot": snapshot, "metrics": map[string]any{"totalRequests": run.TotalRequests, "avgDurationMs": run.AvgDurationMs, "p95DurationMs": run.P95DurationMs, "p99DurationMs": run.P99DurationMs, "errorRate": run.ErrorRate, "rps": run.RPS}, "thresholds": extractSummaryThresholds(run.Summary), "series": sanitizePerfJSON(run.Series), "comparison": comparison, "diagnostic": map[string]any{"failureStage": run.FailureStage, "errorMessage": sanitizePerfTextOrJSON(run.ErrorMessage), "output": sanitizePerfTextOrJSON(run.DiagnosticOutput), "needsAttention": run.NeedsAttention}, "plan": map[string]any{"id": plan.ID, "name": plan.Name, "scenarioType": plan.ScenarioType}}
		payload = stripPerfExportSecrets(payload).(map[string]any)
		data, err := json.Marshal(payload)
		return data, "application/json; charset=utf-8", fmt.Sprintf("synapse-perf-run-%d.json", runID), err
	}
	if !strings.EqualFold(format, "csv") {
		return nil, "", "", errors.New("format 必须是 csv 或 json")
	}
	data, err := exportPerfCSV(run, plan, comparison)
	return data, "text/csv; charset=utf-8", fmt.Sprintf("synapse-perf-run-%d.csv", runID), err
}

func snapshotString(raw json.RawMessage, key string) string {
	var snapshot map[string]any
	if json.Unmarshal(raw, &snapshot) != nil {
		return ""
	}
	value, _ := snapshot[key].(string)
	return value
}

func sanitizePerfTextOrJSON(value string) string {
	safeText := sanitizePerfText(value)
	trimmed := strings.TrimSpace(safeText)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return safeText
	}
	var parsed any
	if json.Unmarshal([]byte(trimmed), &parsed) != nil {
		return safeText
	}
	encoded, err := json.Marshal(sanitizePerfValue(parsed))
	if err != nil {
		return safeText
	}
	return string(encoded)
}

func hydrateRunEnvironmentFromSnapshot(run *model.PerfTestRun) {
	if run == nil || len(run.PlanSnapshot) == 0 {
		return
	}
	var snapshot map[string]any
	if json.Unmarshal(run.PlanSnapshot, &snapshot) != nil {
		return
	}
	if value, ok := snapshot["environmentName"].(string); ok && strings.TrimSpace(value) != "" {
		run.EnvironmentName = value
		run.Environment = value
	}
	if value, ok := snapshot["environmentBaseUrl"].(string); ok && strings.TrimSpace(value) != "" {
		run.EnvironmentBaseURL = value
	}
	if value, ok := snapshot["environmentDeployEnv"].(string); ok {
		run.EnvironmentDeployEnv = value
	}
	if value, ok := snapshot["environmentId"].(float64); ok && value > 0 {
		id := int64(value)
		run.EnvironmentID = &id
	}
}

func exportPerfCSV(run model.PerfTestRun, plan model.PerfTestPlan, comparison model.PerfComparison) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&buffer)
	writer.UseCRLF = true
	row := []string{"run_id", "plan_id", "plan_name", "scenario_type", "status", "triggered_by", "environment_id", "environment", "environment_name", "resolved_target_url", "requested_at", "started_at", "finished_at", "duration_ms", "total_requests", "avg_duration_ms", "p95_duration_ms", "p99_duration_ms", "error_rate", "rps", "is_baseline", "baseline_run_id", "degraded", "degraded_metrics", "failure_stage", "error_message", "needs_attention", "config_hash", "generator_version", "k6_version"}
	if err := writer.Write(row); err != nil {
		return nil, err
	}
	environmentID, environment, environmentName, resolvedTargetURL := csvSnapshotEnvironment(run.PlanSnapshot)
	baselineID := ""
	if comparison.BaselineRunID != nil {
		baselineID = strconv.FormatInt(*comparison.BaselineRunID, 10)
	}
	isBaseline := comparison.BaselineRunID != nil && *comparison.BaselineRunID == run.ID
	values := []string{
		strconv.FormatInt(run.ID, 10), strconv.FormatInt(run.PlanID, 10), plan.Name, run.ScenarioType, run.Status, run.TriggeredBy,
		environmentID, sanitizePerfText(environment), sanitizePerfText(environmentName), sanitizePerfText(resolvedTargetURL),
		formatTime(run.RequestedAt), formatTime(run.StartedAt), formatTime(run.FinishedAt), fmt.Sprint(valueOrNil(run.DurationMs)),
		strconv.Itoa(run.TotalRequests), fmt.Sprint(valueOrNil(run.AvgDurationMs)), fmt.Sprint(valueOrNil(run.P95DurationMs)),
		fmt.Sprint(valueOrNil(run.P99DurationMs)), fmt.Sprint(valueOrNil(run.ErrorRate)), fmt.Sprint(valueOrNil(run.RPS)),
		strconv.FormatBool(isBaseline), baselineID, strconv.FormatBool(comparison.Degraded), strings.Join(comparison.DegradedMetrics, ";"),
		sanitizePerfTextOrJSON(run.FailureStage), sanitizePerfTextOrJSON(run.ErrorMessage), strconv.FormatBool(run.NeedsAttention),
		run.ConfigHash, run.GeneratorVersion, run.K6Version,
	}
	for i := range values {
		values[i] = csvSafe(values[i])
	}
	if err := writer.Write(values); err != nil {
		return nil, err
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func valueOrNil(value any) any {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case *int:
		if v == nil {
			return ""
		}
		return *v
	case *float64:
		if v == nil {
			return ""
		}
		return *v
	default:
		return value
	}
}
func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func csvSnapshotEnvironment(raw json.RawMessage) (environmentID, environment, environmentName, resolvedTargetURL string) {
	var snapshot struct {
		EnvironmentID     *int64 `json:"environmentId"`
		Environment       string `json:"environment"`
		EnvironmentName   string `json:"environmentName"`
		ResolvedTargetURL string `json:"resolvedTargetUrl"`
	}
	if json.Unmarshal(raw, &snapshot) != nil {
		return "", "", "", ""
	}
	if snapshot.EnvironmentID != nil {
		environmentID = strconv.FormatInt(*snapshot.EnvironmentID, 10)
	}
	return environmentID, snapshot.Environment, snapshot.EnvironmentName, snapshot.ResolvedTargetURL
}
func csvSafe(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

func extractSummaryThresholds(raw json.RawMessage) any {
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	return sanitizePerfValue(value["thresholds"])
}
func stripPerfExportSecrets(value any) any {
	switch typed := value.(type) {
	case json.RawMessage:
		var decoded any
		if json.Unmarshal(typed, &decoded) != nil {
			return nil
		}
		return stripPerfExportSecrets(decoded)
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			lower := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if lower == "taskid" || lower == "callbacktoken" || lower == "callbackurl" || lower == "idempotencykey" || lower == "claimtoken" || lower == "claimowner" || lower == "claimuntil" {
				continue
			}
			result[key] = stripPerfExportSecrets(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = stripPerfExportSecrets(item)
		}
		return result
	default:
		return value
	}
}

func (s *PerformanceService) notifyPerfDegradation(ctx context.Context, runID int64) error {
	run, err := s.performanceRepo.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	repo, err := s.p2Repository()
	if err != nil {
		return err
	}
	baseline, err := repo.GetPerfBaseline(ctx, run.PlanID, run.ScenarioType, run.Environment, run.EnvironmentID)
	if err != nil || baseline.RunID == run.ID {
		_, markErr := repo.MarkPerfDegradationChecked(ctx, run.ID)
		return markErr
	}
	comparison, err := s.ComparePerfRun(ctx, run.ID)
	if err != nil {
		return err
	}
	if !comparison.Degraded {
		_, markErr := repo.MarkPerfDegradationChecked(ctx, run.ID)
		return markErr
	}
	if s.notifier == nil {
		return errors.New("通知服务未初始化")
	}
	if err := s.notifier.Create(ctx, model.NotificationCreate{Username: run.TriggeredBy, Type: "perf.degradation", Level: "warning", Title: "性能测试出现劣化", Content: fmt.Sprintf("性能测试执行 #%d 相对基线 #%d 出现劣化：%s", run.ID, baseline.RunID, strings.Join(comparison.DegradedMetrics, ",")), TargetType: "perf_run", TargetID: strconv.FormatInt(run.ID, 10), TargetURL: "#/performance/runs/" + strconv.FormatInt(run.ID, 10)}); err != nil {
		return err
	}
	_, err = repo.MarkPerfDegradationChecked(ctx, run.ID)
	return err
}

func (s *PerformanceService) RecoverPerfDegradation(ctx context.Context) {
	repo, err := s.p2Repository()
	if err != nil {
		return
	}
	items, err := repo.ListPerfDegradationCandidates(ctx, 20)
	if err != nil {
		return
	}
	for _, item := range items {
		_ = s.notifyPerfDegradation(ctx, item.ID)
	}
}

var perfScheduleCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func parsePerfSchedule(expression, timezone string) (cron.Schedule, *time.Location, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" || len(expression) > 256 || strings.IndexFunc(expression, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return nil, nil, errors.New("cron 表达式无效")
	}
	location, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		return nil, nil, errors.New("timezone 必须是有效 IANA 时区")
	}
	schedule, err := perfScheduleCronParser.Parse(expression)
	if err != nil {
		return nil, nil, errors.New("cron 必须是严格五字段表达式")
	}
	return schedule, location, nil
}

func nextPerfScheduleAt(expression, timezone string, from time.Time) (*time.Time, error) {
	schedule, location, err := parsePerfSchedule(expression, timezone)
	if err != nil {
		return nil, err
	}
	next := schedule.Next(from.In(location)).UTC()
	return &next, nil
}

func validatePerfScheduleRequest(req model.PerfScheduleRequest) error {
	if strings.TrimSpace(req.Name) == "" || req.PlanID <= 0 {
		return errors.New("计划名称和方案不能为空")
	}
	_, _, err := parsePerfSchedule(req.CronExpression, req.Timezone)
	return err
}

func (s *PerformanceService) validateSchedulePlan(ctx context.Context, planID int64, environmentID *int64) (model.PerfTestPlan, error) {
	plan, err := s.performanceRepo.GetPlan(ctx, planID)
	if err != nil || plan.ID == 0 {
		return model.PerfTestPlan{}, errors.New("性能测试方案不存在")
	}
	if environmentID != nil {
		item, err := s.resolvePerfEnvironment(ctx, plan.ProductID, environmentID)
		if err != nil {
			return model.PerfTestPlan{}, err
		}
		plan.EnvironmentID = environmentID
		plan.EnvironmentName = item.EnvName
	}
	return plan, nil
}

func (s *PerformanceService) ListPerfSchedules(ctx context.Context, filter model.PerfScheduleFilter, page, pageSize int) (model.PageResult, error) {
	repo, err := s.p2Repository()
	if err != nil {
		return model.PageResult{}, err
	}
	page, pageSize = normalizePage(page, pageSize)
	items, total, err := repo.ListPerfSchedules(ctx, filter, page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *PerformanceService) CreatePerfSchedule(ctx context.Context, actor string, req model.PerfScheduleRequest) (model.PerfSchedule, error) {
	if err := validatePerfScheduleRequest(req); err != nil {
		return model.PerfSchedule{}, err
	}
	plan, err := s.validateSchedulePlan(ctx, req.PlanID, req.EnvironmentID)
	if err != nil {
		return model.PerfSchedule{}, err
	}
	repo, err := s.p2Repository()
	if err != nil {
		return model.PerfSchedule{}, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item := model.PerfSchedule{Name: strings.TrimSpace(req.Name), PlanID: req.PlanID, EnvironmentID: req.EnvironmentID, CronExpression: strings.TrimSpace(req.CronExpression), Timezone: strings.TrimSpace(req.Timezone), CreatedBy: actor, UpdatedBy: actor, Enabled: enabled}
	if plan.ID == 0 {
		return model.PerfSchedule{}, errors.New("性能测试方案不存在")
	}
	if enabled {
		next, err := nextPerfScheduleAt(item.CronExpression, item.Timezone, time.Now().UTC())
		if err != nil {
			return model.PerfSchedule{}, err
		}
		item.NextRunAt = next
	}
	return repo.CreatePerfSchedule(ctx, item)
}

func (s *PerformanceService) UpdatePerfSchedule(ctx context.Context, actor string, id int64, patch model.PerfSchedulePatch) (model.PerfSchedule, error) {
	repo, err := s.p2Repository()
	if err != nil {
		return model.PerfSchedule{}, err
	}
	item, err := repo.GetPerfSchedule(ctx, id)
	if err != nil {
		return model.PerfSchedule{}, errors.New("定时任务不存在")
	}
	cronChanged := false
	if patch.Name != nil {
		item.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.PlanID != nil {
		item.PlanID = *patch.PlanID
	}
	if patch.EnvironmentID != nil {
		item.EnvironmentID = *patch.EnvironmentID
	}
	if patch.CronExpression != nil {
		item.CronExpression = strings.TrimSpace(*patch.CronExpression)
		cronChanged = true
	}
	if patch.Timezone != nil {
		item.Timezone = strings.TrimSpace(*patch.Timezone)
		cronChanged = true
	}
	if item.Name == "" {
		return model.PerfSchedule{}, errors.New("计划名称不能为空")
	}
	if _, err := s.validateSchedulePlan(ctx, item.PlanID, item.EnvironmentID); err != nil {
		return model.PerfSchedule{}, err
	}
	if cronChanged {
		if item.Enabled {
			next, err := nextPerfScheduleAt(item.CronExpression, item.Timezone, time.Now().UTC())
			if err != nil {
				return model.PerfSchedule{}, err
			}
			item.NextRunAt = next
		} else {
			item.NextRunAt = nil
		}
	}
	item.UpdatedBy = actor
	if rows, err := repo.UpdatePerfSchedule(ctx, item); err != nil {
		return model.PerfSchedule{}, err
	} else if rows == 0 {
		return model.PerfSchedule{}, errors.New("定时任务不存在或仍被其他实例占用")
	}
	return repo.GetPerfSchedule(ctx, id)
}

func (s *PerformanceService) EnablePerfSchedule(ctx context.Context, actor string, id int64, enabled bool) (model.PerfSchedule, error) {
	repo, err := s.p2Repository()
	if err != nil {
		return model.PerfSchedule{}, err
	}
	item, err := repo.GetPerfSchedule(ctx, id)
	if err != nil {
		return model.PerfSchedule{}, errors.New("定时任务不存在")
	}
	var next *time.Time
	if enabled {
		next, err = nextPerfScheduleAt(item.CronExpression, item.Timezone, time.Now().UTC())
		if err != nil {
			return model.PerfSchedule{}, err
		}
	}
	if rows, err := repo.SetPerfScheduleEnabled(ctx, id, enabled, next, actor); err != nil {
		return model.PerfSchedule{}, err
	} else if rows == 0 {
		return model.PerfSchedule{}, errors.New("定时任务不存在或仍被其他实例占用")
	}
	return repo.GetPerfSchedule(ctx, id)
}

func (s *PerformanceService) DeletePerfSchedule(ctx context.Context, actor string, id int64) error {
	repo, err := s.p2Repository()
	if err != nil {
		return err
	}
	rows, err := repo.SoftDeletePerfSchedule(ctx, id, actor)
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("定时任务不存在")
	}
	return nil
}

func (s *PerformanceService) ProcessPerfSchedules(ctx context.Context) {
	repo, err := s.p2Repository()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	items, err := repo.ClaimPerfSchedules(ctx, "api-perf-scheduler", now, 20)
	if err != nil {
		return
	}
	for _, item := range items {
		if item.NextRunAt == nil {
			_, _ = repo.SetPerfScheduleError(ctx, item.ID, item.ClaimToken, "缺少 next_run_at")
			continue
		}
		scheduledFor := item.NextRunAt.UTC()
		next, nextErr := nextPerfScheduleAt(item.CronExpression, item.Timezone, now)
		if nextErr != nil {
			_, _ = repo.FinishPerfSchedule(ctx, item.ID, item.ClaimToken, &scheduledFor, nil, "blocked", nil, nextErr.Error())
			continue
		}
		plan, planErr := s.performanceRepo.GetPlan(ctx, item.PlanID)
		if planErr != nil {
			if errors.Is(planErr, sql.ErrNoRows) {
				_, _ = repo.FinishPerfSchedule(ctx, item.ID, item.ClaimToken, &scheduledFor, nil, "blocked", nil, "方案不存在")
			} else {
				_, _ = repo.SetPerfScheduleError(ctx, item.ID, item.ClaimToken, planErr.Error())
			}
			continue
		}
		if item.EnvironmentID != nil {
			_, envErr := s.resolvePerfEnvironment(ctx, plan.ProductID, item.EnvironmentID)
			if envErr != nil {
				_, _ = repo.FinishPerfSchedule(ctx, item.ID, item.ClaimToken, &scheduledFor, nil, "blocked", nil, envErr.Error())
				continue
			}
		}
		key := "perf-schedule:" + strconv.FormatInt(item.ID, 10) + ":" + scheduledFor.Format(time.RFC3339)
		active, activeErr := repo.GetActivePerfRun(ctx, item.PlanID)
		if activeErr != nil && !errors.Is(activeErr, sql.ErrNoRows) {
			_, _ = repo.SetPerfScheduleError(ctx, item.ID, item.ClaimToken, activeErr.Error())
			continue
		}
		if activeErr == nil && active.ID > 0 {
			if active.IdempotencyKey == key && perfScheduleEnvironmentMatches(active, plan, item.EnvironmentID) {
				run, _, runErr := s.RunPlanWithEnvironment(ctx, item.CreatedBy, key, item.PlanID, item.EnvironmentID)
				if runErr != nil {
					_, _ = repo.SetPerfScheduleError(ctx, item.ID, item.ClaimToken, runErr.Error())
					continue
				}
				_, _ = repo.FinishPerfSchedule(ctx, item.ID, item.ClaimToken, &scheduledFor, next, "triggered", &run.ID, "")
			} else {
				_, _ = repo.FinishPerfSchedule(ctx, item.ID, item.ClaimToken, &scheduledFor, next, "skipped_active_run", nil, "已有活动执行")
			}
			continue
		}
		run, _, runErr := s.RunPlanWithEnvironment(ctx, item.CreatedBy, key, item.PlanID, item.EnvironmentID)
		if runErr != nil {
			if isPerfSchedulePermanentError(runErr) {
				_, _ = repo.FinishPerfSchedule(ctx, item.ID, item.ClaimToken, &scheduledFor, nil, "blocked", nil, runErr.Error())
			} else {
				_, _ = repo.SetPerfScheduleError(ctx, item.ID, item.ClaimToken, runErr.Error())
			}
			continue
		}
		_, _ = repo.FinishPerfSchedule(ctx, item.ID, item.ClaimToken, &scheduledFor, next, "triggered", &run.ID, "")
	}
}

func isPerfSchedulePermanentError(err error) bool {
	message := err.Error()
	for _, fragment := range []string{"方案不存在", "方案必须处于 active", "环境不属于", "环境 Base URL", "目标 URL", "场景类型无效", "负载配置"} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func perfScheduleEnvironmentMatches(active model.PerfTestRun, plan model.PerfTestPlan, override *int64) bool {
	environmentID := override
	if environmentID == nil {
		environmentID = plan.EnvironmentID
	}
	if environmentID != nil {
		return active.EnvironmentID != nil && *active.EnvironmentID == *environmentID
	}
	return active.EnvironmentID == nil && active.Environment == plan.Environment
}
