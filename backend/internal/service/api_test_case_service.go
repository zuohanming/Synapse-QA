package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"synapseqa/backend/internal/model"
)

var apiStepKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (s *APIAutomationService) ListAPIGlobalVariables(ctx context.Context, userID int64, filter model.APIGlobalVariableFilter) (model.PageResult, error) {
	items, err := s.repo.ListAPIGlobalVariables(ctx, userID, filter)
	if err != nil {
		return model.PageResult{}, err
	}
	for index := range items {
		if items[index].Sensitive || items[index].ValueType == "secret" {
			items[index].Value = "******"
		}
	}
	return model.PageResult{Items: items, Total: int64(len(items)), Page: 1, PageSize: len(items)}, nil
}

func (s *APIAutomationService) CreateAPIGlobalVariable(ctx context.Context, userID int64, actor string, req model.APIGlobalVariableRequest) (int64, error) {
	req, err := s.normalizeAPIGlobalVariable(ctx, userID, req)
	if err != nil {
		return 0, err
	}
	value, err := s.protectAPIGlobalVariable(req.Value, req.Sensitive || req.ValueType == "secret")
	if err != nil {
		return 0, err
	}
	id, err := s.repo.CreateAPIGlobalVariable(ctx, req, value, actor)
	if err != nil {
		return 0, errors.New("新增全局变量失败，名称可能已存在")
	}
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, "新增接口全局变量", req.Name)
	}
	return id, nil
}

func (s *APIAutomationService) UpdateAPIGlobalVariable(ctx context.Context, userID int64, actor string, id int64, req model.APIGlobalVariableRequest) (int64, error) {
	current, err := s.repo.GetAPIGlobalVariable(ctx, id)
	if err != nil {
		return 0, errors.New("全局变量不存在")
	}
	if current.ProjectID != nil && !s.repo.CanAccessProject(ctx, userID, *current.ProjectID) {
		return 0, errors.New("无权修改该全局变量")
	}
	req, err = s.normalizeAPIGlobalVariable(ctx, userID, req)
	if err != nil {
		return 0, err
	}
	value := req.Value
	if value == "******" {
		value = current.Value
	} else {
		value, err = s.protectAPIGlobalVariable(value, req.Sensitive || req.ValueType == "secret")
		if err != nil {
			return 0, err
		}
	}
	revision, err := s.repo.UpdateAPIGlobalVariable(ctx, id, req, value, actor)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("变量已被其他用户修改，请刷新后重试")
	}
	if err != nil {
		return 0, errors.New("更新全局变量失败，名称可能已存在")
	}
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, "编辑接口全局变量", req.Name)
	}
	return revision, nil
}

func (s *APIAutomationService) DeleteAPIGlobalVariable(ctx context.Context, userID int64, actor string, id int64) error {
	current, err := s.repo.GetAPIGlobalVariable(ctx, id)
	if err != nil {
		return errors.New("全局变量不存在")
	}
	if current.ProjectID != nil && !s.repo.CanAccessProject(ctx, userID, *current.ProjectID) {
		return errors.New("无权删除该全局变量")
	}
	if err := s.repo.DeleteAPIGlobalVariable(ctx, id); err != nil {
		return errors.New("删除全局变量失败")
	}
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, "删除接口全局变量", current.Name)
	}
	return nil
}

func (s *APIAutomationService) normalizeAPIGlobalVariable(ctx context.Context, userID int64, req model.APIGlobalVariableRequest) (model.APIGlobalVariableRequest, error) {
	req.ScopeType = strings.ToLower(strings.TrimSpace(req.ScopeType))
	req.Name = strings.TrimSpace(req.Name)
	req.EnvName = strings.TrimSpace(req.EnvName)
	req.ValueType = strings.ToLower(strings.TrimSpace(req.ValueType))
	if !containsString([]string{"system", "project", "product"}, req.ScopeType) {
		return req, errors.New("变量作用域无效")
	}
	if req.Name == "" || !apiStepKeyPattern.MatchString(req.Name) {
		return req, errors.New("变量名仅支持字母、数字和下划线，且不能以数字开头")
	}
	if !containsString([]string{"string", "number", "boolean", "json", "secret"}, req.ValueType) {
		return req, errors.New("变量类型无效")
	}
	switch req.ScopeType {
	case "system":
		req.ProjectID, req.ProductID, req.EnvName = 0, 0, ""
	case "project":
		if req.ProjectID <= 0 || !s.repo.CanAccessProject(ctx, userID, req.ProjectID) {
			return req, errors.New("项目无效或无权访问")
		}
		req.ProductID = 0
	case "product":
		projectID, err := s.repo.ProductProject(ctx, req.ProductID)
		if err != nil || projectID != req.ProjectID || !s.repo.CanAccessProject(ctx, userID, projectID) {
			return req, errors.New("产品与项目不匹配或无权访问")
		}
	}
	if req.ValueType == "number" {
		var number json.Number
		if json.Unmarshal([]byte(req.Value), &number) != nil {
			return req, errors.New("Number 变量值格式无效")
		}
	}
	if req.ValueType == "boolean" && req.Value != "true" && req.Value != "false" {
		return req, errors.New("Boolean 变量值只能是 true 或 false")
	}
	if req.ValueType == "json" && !json.Valid([]byte(req.Value)) {
		return req, errors.New("JSON 变量值格式无效")
	}
	return req, nil
}

func (s *APIAutomationService) protectAPIGlobalVariable(value string, sensitive bool) (string, error) {
	if !sensitive || value == "" || strings.HasPrefix(value, encryptedAPISecretPrefix) {
		return value, nil
	}
	return encryptAPISecret(s.secretKey, value)
}

func (s *APIAutomationService) ListAPITestCases(ctx context.Context, userID int64, filter model.APITestCaseFilter, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	items, total, err := s.repo.ListAPITestCases(ctx, userID, filter, page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *APIAutomationService) GetAPITestCase(ctx context.Context, userID, id int64) (model.APITestCase, error) {
	item, err := s.repo.GetAPITestCase(ctx, userID, id)
	if err != nil {
		return item, errors.New("接口测试用例不存在")
	}
	return item, nil
}

func (s *APIAutomationService) CreateAPITestCase(ctx context.Context, userID int64, actor string, req model.APITestCaseRequest) (int64, error) {
	req, err := s.normalizeAPITestCase(ctx, userID, req, false)
	if err != nil {
		return 0, err
	}
	id, err := s.repo.CreateAPITestCase(ctx, req, actor)
	if err != nil {
		return 0, errors.New("新增接口测试用例失败，名称可能已存在")
	}
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, "新增接口测试用例", req.Name)
	}
	return id, nil
}

func (s *APIAutomationService) UpdateAPITestCase(ctx context.Context, userID int64, actor string, id int64, req model.APITestCaseRequest) (int64, error) {
	current, err := s.repo.GetAPITestCase(ctx, userID, id)
	if err != nil {
		return 0, errors.New("接口测试用例不存在")
	}
	req, err = s.normalizeAPITestCase(ctx, userID, req, false)
	if err != nil {
		return 0, err
	}
	if req.Revision != current.Revision {
		return 0, errors.New("草稿已被其他用户修改，请刷新后比较差异")
	}
	revision, err := s.repo.UpdateAPITestCase(ctx, id, req, actor)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("草稿已被其他用户修改，请刷新后比较差异")
	}
	if err != nil {
		return 0, errors.New("保存接口测试用例失败，名称可能已存在")
	}
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, "保存接口测试用例草稿", req.Name)
	}
	return revision, nil
}

func (s *APIAutomationService) DeleteAPITestCase(ctx context.Context, userID int64, actor string, id int64) error {
	item, err := s.repo.GetAPITestCase(ctx, userID, id)
	if err != nil {
		return errors.New("接口测试用例不存在")
	}
	if err := s.repo.DeleteAPITestCase(ctx, id); err != nil {
		return errors.New("删除接口测试用例失败")
	}
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, "删除接口测试用例", item.Name)
	}
	return nil
}

func (s *APIAutomationService) ValidateAPITestCase(ctx context.Context, userID, id int64) (model.APITestCaseValidation, error) {
	item, err := s.repo.GetAPITestCase(ctx, userID, id)
	if err != nil {
		return model.APITestCaseValidation{}, errors.New("接口测试用例不存在")
	}
	return s.validateAPITestCaseDraft(ctx, item.ProjectID, item.Draft), nil
}

func (s *APIAutomationService) PublishAPITestCase(ctx context.Context, userID int64, actor string, id int64, req model.APITestCasePublishRequest) (int, error) {
	item, err := s.repo.GetAPITestCase(ctx, userID, id)
	if err != nil {
		return 0, errors.New("接口测试用例不存在")
	}
	if req.Revision != item.Revision {
		return 0, errors.New("草稿版本已变化，请刷新后重试")
	}
	validation := s.validateAPITestCaseDraft(ctx, item.ProjectID, item.Draft)
	if !validation.Valid {
		return 0, fmt.Errorf("发布检查失败：%s", strings.Join(validation.Errors, "；"))
	}
	version, err := s.repo.PublishAPITestCase(ctx, id, req.Revision, actor, strings.TrimSpace(req.ChangeSummary))
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("草稿版本已变化，请刷新后重试")
	}
	if err != nil {
		return 0, errors.New("发布接口测试用例失败")
	}
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, "发布接口测试用例", fmt.Sprintf("%s V%d", item.Name, version))
	}
	return version, nil
}

func (s *APIAutomationService) ListAPITestCaseVersions(ctx context.Context, userID, id int64) ([]model.APITestCaseVersion, error) {
	if _, err := s.repo.GetAPITestCase(ctx, userID, id); err != nil {
		return nil, errors.New("接口测试用例不存在")
	}
	return s.repo.ListAPITestCaseVersions(ctx, id)
}

func (s *APIAutomationService) GetAPITestCaseVersion(ctx context.Context, userID, id int64, version int) (model.APITestCaseVersion, error) {
	if _, err := s.repo.GetAPITestCase(ctx, userID, id); err != nil {
		return model.APITestCaseVersion{}, errors.New("接口测试用例不存在")
	}
	item, err := s.repo.GetAPITestCaseVersion(ctx, id, version)
	if err != nil {
		return item, errors.New("用例版本不存在")
	}
	return item, nil
}

func (s *APIAutomationService) normalizeAPITestCase(ctx context.Context, userID int64, req model.APITestCaseRequest, strict bool) (model.APITestCaseRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.ProjectID <= 0 || req.ProductID <= 0 {
		return req, errors.New("项目、产品和用例名称必填")
	}
	projectID, err := s.repo.ProductProject(ctx, req.ProductID)
	if err != nil || projectID != req.ProjectID || !s.repo.CanAccessProject(ctx, userID, req.ProjectID) {
		return req, errors.New("产品与项目不匹配或无权访问")
	}
	if req.Priority == "" {
		req.Priority = "P2"
	}
	if !containsString([]string{"P0", "P1", "P2", "P3"}, req.Priority) {
		return req, errors.New("用例优先级无效")
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if !containsString([]string{"draft", "active", "disabled", "deprecated"}, req.Status) {
		return req, errors.New("用例状态无效")
	}
	if len(req.Draft) == 0 {
		req.Draft = json.RawMessage(`{"steps":[],"datasets":[],"variables":[],"dataSchema":[]}`)
	}
	if !json.Valid(req.Draft) {
		return req, errors.New("用例草稿格式无效")
	}
	var draft model.APITestCaseDraft
	if err := json.Unmarshal(req.Draft, &draft); err != nil {
		return req, errors.New("用例草稿结构无效")
	}
	if len(draft.Steps) > 100 || len(draft.Datasets) > 10000 {
		return req, errors.New("用例最多 100 个步骤和 10000 个数据实例")
	}
	if strict {
		validation := s.validateAPITestCaseDraft(ctx, req.ProjectID, req.Draft)
		if !validation.Valid {
			return req, errors.New(strings.Join(validation.Errors, "；"))
		}
	}
	return req, nil
}

func (s *APIAutomationService) validateAPITestCaseDraft(ctx context.Context, projectID int64, raw json.RawMessage) model.APITestCaseValidation {
	result := model.APITestCaseValidation{Valid: true, Errors: []string{}, Warnings: []string{}}
	var draft model.APITestCaseDraft
	if err := json.Unmarshal(raw, &draft); err != nil {
		result.Errors = append(result.Errors, "草稿结构无效")
		result.Valid = false
		return result
	}
	enabled := 0
	keys := map[string]bool{}
	for index, step := range draft.Steps {
		if !step.Enabled {
			continue
		}
		enabled++
		label := fmt.Sprintf("步骤 %d", index+1)
		if step.Name != "" {
			label = step.Name
		}
		if !apiStepKeyPattern.MatchString(step.Key) || keys[strings.ToLower(step.Key)] {
			result.Errors = append(result.Errors, label+"的步骤标识无效或重复")
		}
		keys[strings.ToLower(step.Key)] = true
		if step.InterfaceID <= 0 || step.InterfaceVersion <= 0 {
			result.Errors = append(result.Errors, label+"未固定接口版本")
			continue
		}
		version, err := s.repo.GetInterfaceVersion(ctx, step.InterfaceID, step.InterfaceVersion)
		if err != nil {
			result.Errors = append(result.Errors, label+"引用的接口版本不存在")
			continue
		}
		var snapshot model.APIInterfaceRequest
		if json.Unmarshal(version.Snapshot, &snapshot) != nil {
			result.Errors = append(result.Errors, label+"的接口版本快照无效")
			continue
		}
		stepProjectID, err := s.repo.ProductProject(ctx, snapshot.ProductID)
		if err != nil || stepProjectID != projectID {
			result.Errors = append(result.Errors, label+"引用了其他项目的接口")
		}
		if step.FailurePolicy != "" && !containsString([]string{"stop", "continue", "always"}, step.FailurePolicy) {
			result.Errors = append(result.Errors, label+"的失败策略无效")
		}
	}
	if enabled == 0 {
		result.Errors = append(result.Errors, "至少需要一个启用的接口步骤")
	}
	result.Valid = len(result.Errors) == 0
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
