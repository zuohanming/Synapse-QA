package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/repository"
)

type PerformanceRepository interface {
	ListPlans(ctx context.Context, filter model.PerfTestPlanFilter, page, pageSize int) ([]model.PerfTestPlan, int64, error)
	GetPlan(ctx context.Context, id int64) (model.PerfTestPlan, error)
	CreatePlan(ctx context.Context, req model.PerfTestPlanRequest, actor string) (int64, error)
	UpdatePlan(ctx context.Context, id int64, req model.PerfTestPlanRequest) (int64, error)
	DeletePlan(ctx context.Context, id int64) (int64, error)
	ExistsProduct(ctx context.Context, id int64) bool
	ListRuns(ctx context.Context, filter model.PerfTestRunFilter, page, pageSize int) ([]model.PerfTestRun, int64, error)
	GetRun(ctx context.Context, id int64) (model.PerfTestRun, error)
	CreateRun(ctx context.Context, planID int64, actor string) (int64, error)
}

type PerformanceService struct {
	performanceRepo PerformanceRepository
	systemRepo      OperationLogger
}

func NewPerformanceService(performanceRepo PerformanceRepository, systemRepo *repository.SystemRepository) *PerformanceService {
	svc := &PerformanceService{performanceRepo: performanceRepo}
	if systemRepo != nil {
		svc.systemRepo = systemRepo
	}
	return svc
}

func NewPerformanceServiceWithLogger(performanceRepo PerformanceRepository, logger OperationLogger) *PerformanceService {
	return &PerformanceService{performanceRepo: performanceRepo, systemRepo: logger}
}

func (s *PerformanceService) ListPlans(ctx context.Context, filter model.PerfTestPlanFilter, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	filter.ID = strings.TrimSpace(filter.ID)
	filter.Name = strings.TrimSpace(filter.Name)
	filter.ProductID = strings.TrimSpace(filter.ProductID)
	filter.LoadMode = strings.TrimSpace(filter.LoadMode)
	filter.Priority = strings.TrimSpace(filter.Priority)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Owner = strings.TrimSpace(filter.Owner)
	items, total, err := s.performanceRepo.ListPlans(ctx, filter, page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *PerformanceService) GetPlan(ctx context.Context, id int64) (model.PerfTestPlan, error) {
	if id <= 0 {
		return model.PerfTestPlan{}, errors.New("性能测试方案 ID 无效")
	}
	item, err := s.performanceRepo.GetPlan(ctx, id)
	if err != nil {
		return model.PerfTestPlan{}, errors.New("性能测试方案不存在")
	}
	return item, nil
}

func (s *PerformanceService) CreatePlan(ctx context.Context, actor string, req model.PerfTestPlanRequest) (int64, error) {
	req, err := s.normalizeRequest(ctx, req)
	if err != nil {
		return 0, err
	}
	id, err := s.performanceRepo.CreatePlan(ctx, req, actor)
	if err != nil {
		return 0, errors.New("新增性能测试方案失败，名称可能已存在")
	}
	s.log(ctx, actor, "新增性能测试方案", req.Name)
	return id, nil
}

func (s *PerformanceService) UpdatePlan(ctx context.Context, actor string, id int64, req model.PerfTestPlanRequest) error {
	if id <= 0 {
		return errors.New("性能测试方案 ID 无效")
	}
	req, err := s.normalizeRequest(ctx, req)
	if err != nil {
		return err
	}
	rows, err := s.performanceRepo.UpdatePlan(ctx, id, req)
	if err != nil {
		return errors.New("更新性能测试方案失败，名称可能已存在")
	}
	if rows == 0 {
		return errors.New("性能测试方案不存在")
	}
	s.log(ctx, actor, "更新性能测试方案", fmt.Sprintf("%d:%s", id, req.Name))
	return nil
}

func (s *PerformanceService) DeletePlan(ctx context.Context, actor string, id int64) error {
	if id <= 0 {
		return errors.New("性能测试方案 ID 无效")
	}
	rows, err := s.performanceRepo.DeletePlan(ctx, id)
	if err != nil {
		return errors.New("删除性能测试方案失败")
	}
	if rows == 0 {
		return errors.New("性能测试方案不存在")
	}
	s.log(ctx, actor, "删除性能测试方案", strconv.FormatInt(id, 10))
	return nil
}

func (s *PerformanceService) RunPlan(ctx context.Context, actor string, planID int64) (int64, error) {
	if planID <= 0 {
		return 0, errors.New("性能测试方案 ID 无效")
	}
	if _, err := s.performanceRepo.GetPlan(ctx, planID); err != nil {
		return 0, errors.New("性能测试方案不存在")
	}
	runID, err := s.performanceRepo.CreateRun(ctx, planID, actor)
	if err != nil {
		return 0, errors.New("触发性能测试失败")
	}
	s.log(ctx, actor, "触发性能测试", strconv.FormatInt(planID, 10))
	return runID, nil
}

func (s *PerformanceService) ListRuns(ctx context.Context, filter model.PerfTestRunFilter, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	filter.ID = strings.TrimSpace(filter.ID)
	filter.PlanID = strings.TrimSpace(filter.PlanID)
	filter.Status = strings.TrimSpace(filter.Status)
	items, total, err := s.performanceRepo.ListRuns(ctx, filter, page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *PerformanceService) GetRun(ctx context.Context, id int64) (model.PerfTestRun, error) {
	if id <= 0 {
		return model.PerfTestRun{}, errors.New("执行记录 ID 无效")
	}
	item, err := s.performanceRepo.GetRun(ctx, id)
	if err != nil {
		return model.PerfTestRun{}, errors.New("执行记录不存在")
	}
	return item, nil
}

func (s *PerformanceService) normalizeRequest(ctx context.Context, req model.PerfTestPlanRequest) (model.PerfTestPlanRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.TargetURL = strings.TrimSpace(req.TargetURL)
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	req.LoadMode = strings.TrimSpace(req.LoadMode)
	req.Priority = strings.TrimSpace(req.Priority)
	req.Status = strings.TrimSpace(req.Status)
	req.Owner = strings.TrimSpace(req.Owner)
	req.Tags = strings.TrimSpace(req.Tags)
	req.Description = strings.TrimSpace(req.Description)
	req.Duration = strings.TrimSpace(req.Duration)
	if req.Method == "" {
		req.Method = "GET"
	}
	if req.LoadMode == "" {
		req.LoadMode = "constant"
	}
	if req.Priority == "" {
		req.Priority = "P2"
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.ProductID <= 0 || req.Name == "" {
		return req, errors.New("产品名称和方案名称不能为空")
	}
	if len([]rune(req.Name)) > 120 {
		return req, errors.New("方案名称不能超过 120 个字符")
	}
	if !allowed(req.Method, "GET", "POST", "PUT", "DELETE", "PATCH") {
		return req, errors.New("请求方法无效")
	}
	if !allowed(req.LoadMode, "constant", "ramping") {
		return req, errors.New("负载模式无效")
	}
	if !allowed(req.Priority, "P0", "P1", "P2", "P3") {
		return req, errors.New("优先级无效")
	}
	if !allowed(req.Status, "draft", "active", "disabled") {
		return req, errors.New("状态无效")
	}
	if !s.performanceRepo.ExistsProduct(ctx, req.ProductID) {
		return req, errors.New("产品不存在")
	}
	if len(req.Headers) == 0 {
		req.Headers = json.RawMessage(`{}`)
	}
	if !isJSONObject(req.Headers) {
		return req, errors.New("请求头必须是 JSON 对象")
	}
	if len(req.Thresholds) == 0 {
		req.Thresholds = json.RawMessage(`{}`)
	}
	if !isJSONObject(req.Thresholds) {
		return req, errors.New("阈值必须是 JSON 对象")
	}
	if len(req.Stages) == 0 {
		req.Stages = json.RawMessage(`[]`)
	}
	if req.LoadMode == "constant" {
		if req.VUs <= 0 || req.Duration == "" {
			return req, errors.New("固定并发模式需要设置并发数和时长")
		}
	} else {
		var stages []any
		if err := json.Unmarshal(req.Stages, &stages); err != nil || len(stages) == 0 {
			return req, errors.New("爬坡模式需要配置爬坡阶段")
		}
	}
	return req, nil
}

func isJSONObject(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var obj map[string]any
	return json.Unmarshal(raw, &obj) == nil
}

func (s *PerformanceService) log(ctx context.Context, actor, action, target string) {
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, action, target)
	}
}
