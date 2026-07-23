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

type TestCaseRepository interface {
	List(ctx context.Context, filter model.TestCaseFilter, page, pageSize int) ([]model.TestCase, int64, error)
	Get(ctx context.Context, id int64) (model.TestCaseDetail, error)
	Create(ctx context.Context, req model.TestCaseRequest, actor string) (int64, error)
	Update(ctx context.Context, id int64, req model.TestCaseRequest) (int64, error)
	Delete(ctx context.Context, id int64) (int64, error)
	ExistsProduct(ctx context.Context, id int64) bool
	CountMissingSteps(ctx context.Context, ids []int64) (int64, error)
	ListDatasets(ctx context.Context, caseID int64) ([]model.TestCaseDataset, error)
	CreateDataset(ctx context.Context, caseID int64, req model.TestCaseDatasetRequest) error
	UpdateDataset(ctx context.Context, caseID, datasetID int64, req model.TestCaseDatasetRequest) (int64, error)
	DeleteDataset(ctx context.Context, caseID, datasetID int64) (int64, error)
}

type OperationLogger interface {
	LogOperation(ctx context.Context, actor, action, target string) error
}

type TestCaseService struct {
	testCaseRepo TestCaseRepository
	systemRepo   OperationLogger
}

func NewTestCaseService(testCaseRepo TestCaseRepository, systemRepo *repository.SystemRepository) *TestCaseService {
	svc := &TestCaseService{testCaseRepo: testCaseRepo}
	if systemRepo != nil {
		svc.systemRepo = systemRepo
	}
	return svc
}

func NewTestCaseServiceWithLogger(testCaseRepo TestCaseRepository, logger OperationLogger) *TestCaseService {
	return &TestCaseService{testCaseRepo: testCaseRepo, systemRepo: logger}
}

func (s *TestCaseService) List(ctx context.Context, filter model.TestCaseFilter, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	filter.ID = strings.TrimSpace(filter.ID)
	filter.Name = strings.TrimSpace(filter.Name)
	filter.ProductID = strings.TrimSpace(filter.ProductID)
	filter.ModuleID = strings.TrimSpace(filter.ModuleID)
	filter.CaseType = strings.TrimSpace(filter.CaseType)
	filter.Priority = strings.TrimSpace(filter.Priority)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Owner = strings.TrimSpace(filter.Owner)
	items, total, err := s.testCaseRepo.List(ctx, filter, page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *TestCaseService) Get(ctx context.Context, id int64) (model.TestCaseDetail, error) {
	if id <= 0 {
		return model.TestCaseDetail{}, errors.New("测试用例 ID 无效")
	}
	item, err := s.testCaseRepo.Get(ctx, id)
	if err != nil {
		return model.TestCaseDetail{}, errors.New("测试用例不存在")
	}
	return item, nil
}

func (s *TestCaseService) Create(ctx context.Context, actor string, req model.TestCaseRequest) (int64, error) {
	req, err := s.normalizeRequest(ctx, req)
	if err != nil {
		return 0, err
	}
	id, err := s.testCaseRepo.Create(ctx, req, actor)
	if err != nil {
		return 0, errors.New("新增测试用例失败，名称可能已存在")
	}
	s.log(ctx, actor, "新增测试用例", req.Name)
	return id, nil
}

func (s *TestCaseService) Update(ctx context.Context, actor string, id int64, req model.TestCaseRequest) error {
	if id <= 0 {
		return errors.New("测试用例 ID 无效")
	}
	req, err := s.normalizeRequest(ctx, req)
	if err != nil {
		return err
	}
	rows, err := s.testCaseRepo.Update(ctx, id, req)
	if err != nil {
		return errors.New("更新测试用例失败，名称可能已存在")
	}
	if rows == 0 {
		return errors.New("测试用例不存在")
	}
	s.log(ctx, actor, "编辑测试用例", fmt.Sprintf("%d:%s", id, req.Name))
	return nil
}

func (s *TestCaseService) Delete(ctx context.Context, actor string, id int64) error {
	if id <= 0 {
		return errors.New("测试用例 ID 无效")
	}
	rows, err := s.testCaseRepo.Delete(ctx, id)
	if err != nil {
		return errors.New("删除测试用例失败")
	}
	if rows == 0 {
		return errors.New("测试用例不存在")
	}
	s.log(ctx, actor, "删除测试用例", strconv.FormatInt(id, 10))
	return nil
}

func (s *TestCaseService) Import(ctx context.Context, actor string, req model.TestCaseImportRequest) (int, error) {
	if len(req.Items) == 0 {
		return 0, errors.New("导入数据不能为空")
	}
	if len(req.Items) > 200 {
		return 0, errors.New("单次最多导入 200 条测试用例")
	}
	count := 0
	for index, item := range req.Items {
		if _, err := s.Create(ctx, actor, item); err != nil {
			return count, fmt.Errorf("第 %d 条导入失败：%w", index+1, err)
		}
		count++
	}
	s.log(ctx, actor, "导入测试用例", strconv.Itoa(count))
	return count, nil
}

func (s *TestCaseService) Export(ctx context.Context, filter model.TestCaseFilter) ([]model.TestCase, error) {
	result, err := s.List(ctx, filter, 1, 10000)
	if err != nil {
		return nil, err
	}
	items, ok := result.Items.([]model.TestCase)
	if !ok {
		return nil, errors.New("导出测试用例失败")
	}
	return items, nil
}

func (s *TestCaseService) ListDatasets(ctx context.Context, caseID int64) ([]model.TestCaseDataset, error) {
	if caseID <= 0 {
		return nil, errors.New("测试用例 ID 无效")
	}
	return s.testCaseRepo.ListDatasets(ctx, caseID)
}

func (s *TestCaseService) CreateDataset(ctx context.Context, actor string, caseID int64, req model.TestCaseDatasetRequest) error {
	req, err := normalizeDataset(req)
	if err != nil {
		return err
	}
	if err := s.CreateDatasetRaw(ctx, caseID, req); err != nil {
		return err
	}
	s.log(ctx, actor, "新增测试用例参数化数据", fmt.Sprintf("%d:%s", caseID, req.Name))
	return nil
}

func (s *TestCaseService) CreateDatasetRaw(ctx context.Context, caseID int64, req model.TestCaseDatasetRequest) error {
	if caseID <= 0 {
		return errors.New("测试用例 ID 无效")
	}
	if err := s.testCaseRepo.CreateDataset(ctx, caseID, req); err != nil {
		return errors.New("新增参数化数据失败，名称可能已存在")
	}
	return nil
}

func (s *TestCaseService) UpdateDataset(ctx context.Context, actor string, caseID, datasetID int64, req model.TestCaseDatasetRequest) error {
	req, err := normalizeDataset(req)
	if err != nil {
		return err
	}
	if caseID <= 0 || datasetID <= 0 {
		return errors.New("参数化数据 ID 无效")
	}
	rows, err := s.testCaseRepo.UpdateDataset(ctx, caseID, datasetID, req)
	if err != nil {
		return errors.New("更新参数化数据失败")
	}
	if rows == 0 {
		return errors.New("参数化数据不存在")
	}
	s.log(ctx, actor, "编辑测试用例参数化数据", fmt.Sprintf("%d:%d", caseID, datasetID))
	return nil
}

func (s *TestCaseService) DeleteDataset(ctx context.Context, actor string, caseID, datasetID int64) error {
	if caseID <= 0 || datasetID <= 0 {
		return errors.New("参数化数据 ID 无效")
	}
	rows, err := s.testCaseRepo.DeleteDataset(ctx, caseID, datasetID)
	if err != nil {
		return errors.New("删除参数化数据失败")
	}
	if rows == 0 {
		return errors.New("参数化数据不存在")
	}
	s.log(ctx, actor, "删除测试用例参数化数据", fmt.Sprintf("%d:%d", caseID, datasetID))
	return nil
}

func (s *TestCaseService) normalizeRequest(ctx context.Context, req model.TestCaseRequest) (model.TestCaseRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.CaseType = strings.TrimSpace(req.CaseType)
	req.Priority = strings.TrimSpace(req.Priority)
	req.Status = strings.TrimSpace(req.Status)
	req.Owner = strings.TrimSpace(req.Owner)
	req.Tags = strings.TrimSpace(req.Tags)
	req.Description = strings.TrimSpace(req.Description)
	req.Preconditions = strings.TrimSpace(req.Preconditions)
	req.ExpectedResult = strings.TrimSpace(req.ExpectedResult)
	if req.CaseType == "" {
		req.CaseType = "ui"
	}
	if req.Priority == "" {
		req.Priority = "P2"
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.ProductID <= 0 || req.Name == "" {
		return req, errors.New("项目/产品和用例名称不能为空")
	}
	if len([]rune(req.Name)) > 120 {
		return req, errors.New("用例名称不能超过 120 个字符")
	}
	if !allowed(req.CaseType, "ui", "api", "unit", "mixed") {
		return req, errors.New("用例类型无效")
	}
	if !allowed(req.Priority, "P0", "P1", "P2", "P3") {
		return req, errors.New("优先级无效")
	}
	if !allowed(req.Status, "draft", "active", "disabled") {
		return req, errors.New("状态无效")
	}
	if !s.testCaseRepo.ExistsProduct(ctx, req.ProductID) {
		return req, errors.New("项目/产品不存在")
	}
	missing, err := s.testCaseRepo.CountMissingSteps(ctx, uniqueIDs(req.StepIDs))
	if err != nil {
		return req, errors.New("校验步骤失败")
	}
	if missing > 0 {
		return req, errors.New("存在无效页面步骤")
	}
	req.StepIDs = uniqueIDs(req.StepIDs)
	return req, nil
}

func normalizeDataset(req model.TestCaseDatasetRequest) (model.TestCaseDatasetRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return req, errors.New("数据集名称不能为空")
	}
	if len(req.Variables) == 0 {
		req.Variables = json.RawMessage(`{}`)
	}
	var variables map[string]any
	if err := json.Unmarshal(req.Variables, &variables); err != nil {
		return req, errors.New("参数化变量必须是 JSON 对象")
	}
	return req, nil
}

func allowed(value string, values ...string) bool {
	for _, item := range values {
		if value == item {
			return true
		}
	}
	return false
}

func uniqueIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	result := []int64{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

func (s *TestCaseService) log(ctx context.Context, actor, action, target string) {
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, action, target)
	}
}
