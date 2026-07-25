package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/repository"
)

type APIAutomationService struct {
	repo         *repository.APIAutomationRepository
	systemRepo   OperationLogger
	secretKey    []byte
	executorRepo *repository.ExecutorRepository
	callbackBase string
}

func NewAPIAutomationService(repo *repository.APIAutomationRepository, systemRepo OperationLogger, secretKey ...[]byte) *APIAutomationService {
	key := []byte("synapse-api-automation-local-secret")
	if len(secretKey) > 0 && len(secretKey[0]) > 0 {
		key = secretKey[0]
	}
	return &APIAutomationService{repo: repo, systemRepo: systemRepo, secretKey: key}
}

func (s *APIAutomationService) ConfigureDebug(executorRepo *repository.ExecutorRepository, callbackBase string) {
	s.executorRepo = executorRepo
	s.callbackBase = strings.TrimRight(callbackBase, "/")
}

func (s *APIAutomationService) ListInterfaces(ctx context.Context, userID int64, filter model.APIInterfaceFilter, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	items, total, err := s.repo.ListInterfaces(ctx, userID, filter, page, pageSize)
	for index := range items {
		items[index].Configuration = maskAPIConfiguration(items[index].Configuration)
	}
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *APIAutomationService) GetInterface(ctx context.Context, userID, id int64) (model.APIInterface, error) {
	item, err := s.repo.GetInterface(ctx, userID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return item, errors.New("接口不存在或无权访问")
	}
	item.Configuration = maskAPIConfiguration(item.Configuration)
	return item, err
}

func (s *APIAutomationService) CreateInterface(ctx context.Context, userID int64, actor string, req model.APIInterfaceRequest) (int64, error) {
	req, normalized, projectID, err := s.normalizeInterface(ctx, req)
	if err != nil {
		return 0, err
	}
	if !s.repo.CanAccessProject(ctx, userID, projectID) {
		return 0, errors.New("无权访问该项目")
	}
	id, err := s.repo.CreateInterface(ctx, req, normalized, actor)
	if err != nil {
		return 0, errors.New("新增接口失败，请检查方法和路径是否重复")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "新增接口", req.Name)
	return id, nil
}

func (s *APIAutomationService) UpdateInterface(ctx context.Context, userID int64, actor string, id int64, req model.APIInterfaceRequest) error {
	current, err := s.repo.GetInterface(ctx, userID, id)
	if err != nil {
		return errors.New("接口不存在或无权访问")
	}
	req.Configuration = preserveMaskedAPIAuth(req.Configuration, current.Configuration)
	req, normalized, projectID, err := s.normalizeInterface(ctx, req)
	if err != nil {
		return err
	}
	if !s.repo.CanAccessProject(ctx, userID, current.ProjectID) || !s.repo.CanAccessProject(ctx, userID, projectID) {
		return errors.New("无权访问该项目")
	}
	if req.Revision <= 0 {
		return errors.New("缺少接口版本号")
	}
	if _, err := s.repo.UpdateInterface(ctx, id, req, normalized, actor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("接口已被其他用户修改，请刷新后重试")
		}
		return errors.New("更新接口失败，请检查方法和路径是否重复")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "编辑接口", req.Name)
	return nil
}

func (s *APIAutomationService) DeleteInterface(ctx context.Context, userID int64, actor string, id int64) error {
	item, err := s.repo.GetInterface(ctx, userID, id)
	if err != nil || !s.repo.CanAccessProject(ctx, userID, item.ProjectID) {
		return errors.New("接口不存在或无权访问")
	}
	rows, err := s.repo.DeleteInterface(ctx, id)
	if err != nil || rows == 0 {
		return errors.New("删除接口失败")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "删除接口", item.Name)
	return nil
}

func (s *APIAutomationService) ListProjectHeaders(ctx context.Context, userID, projectID int64, keyword string) (model.PageResult, error) {
	if projectID <= 0 {
		return model.PageResult{}, errors.New("请选择项目")
	}
	if !s.repo.CanAccessProject(ctx, userID, projectID) {
		return model.PageResult{}, errors.New("无权访问该项目")
	}
	items, err := s.repo.ListProjectHeaders(ctx, projectID, strings.TrimSpace(keyword))
	return model.PageResult{Items: items, Total: int64(len(items)), Page: 1, PageSize: len(items)}, err
}

func (s *APIAutomationService) CreateProjectHeader(ctx context.Context, userID int64, actor string, req model.APIProjectHeaderRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.ProjectID <= 0 || req.Name == "" {
		return errors.New("项目和请求头名称不能为空")
	}
	if !s.repo.CanAccessProject(ctx, userID, req.ProjectID) {
		return errors.New("无权访问该项目")
	}
	if err := s.repo.CreateProjectHeader(ctx, req, actor); err != nil {
		return errors.New("新增请求头失败，该项目可能已存在同名请求头")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "新增项目请求头", req.Name)
	return nil
}

func (s *APIAutomationService) UpdateProjectHeader(ctx context.Context, userID int64, actor string, id int64, req model.APIProjectHeaderRequest) error {
	oldProjectID, err := s.repo.HeaderProject(ctx, id)
	if err != nil || !s.repo.CanAccessProject(ctx, userID, oldProjectID) || !s.repo.CanAccessProject(ctx, userID, req.ProjectID) {
		return errors.New("请求头不存在或无权访问")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.ProjectID <= 0 || req.Name == "" {
		return errors.New("项目和请求头名称不能为空")
	}
	rows, err := s.repo.UpdateProjectHeader(ctx, id, req, actor)
	if err != nil || rows == 0 {
		return errors.New("更新请求头失败，该项目可能已存在同名请求头")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "编辑项目请求头", req.Name)
	return nil
}

func (s *APIAutomationService) DeleteProjectHeader(ctx context.Context, userID int64, actor string, id int64) error {
	projectID, err := s.repo.HeaderProject(ctx, id)
	if err != nil || !s.repo.CanAccessProject(ctx, userID, projectID) {
		return errors.New("请求头不存在或无权访问")
	}
	rows, err := s.repo.DeleteProjectHeader(ctx, id)
	if err != nil || rows == 0 {
		return errors.New("删除请求头失败")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "删除项目请求头", strconv.FormatInt(id, 10))
	return nil
}

func (s *APIAutomationService) normalizeInterface(ctx context.Context, req model.APIInterfaceRequest) (model.APIInterfaceRequest, string, int64, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	req.Path = strings.TrimSpace(req.Path)
	req.Protocol = strings.ToUpper(strings.TrimSpace(req.Protocol))
	req.EndpointType = strings.ToUpper(strings.TrimSpace(req.EndpointType))
	req.LifecycleStatus = strings.ToLower(strings.TrimSpace(req.LifecycleStatus))
	if req.Protocol == "" {
		req.Protocol = "HTTP"
	}
	if req.EndpointType == "" {
		req.EndpointType = "WEB"
	}
	if req.LifecycleStatus == "" {
		req.LifecycleStatus = "draft"
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 30
	}
	if len(req.Configuration) == 0 {
		req.Configuration = json.RawMessage(`{}`)
	}
	if !json.Valid(req.Configuration) {
		return req, "", 0, errors.New("接口配置格式无效")
	}
	protected, err := s.protectAPIConfiguration(req.Configuration)
	if err != nil {
		return req, "", 0, errors.New("认证密钥加密失败")
	}
	req.Configuration = protected
	if req.ProductID <= 0 || req.Name == "" || req.Path == "" {
		return req, "", 0, errors.New("项目/产品、接口名称和路径不能为空")
	}
	if !contains([]string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}, req.Method) {
		return req, "", 0, errors.New("请求方法无效")
	}
	if !contains([]string{"draft", "active", "disabled", "deprecated"}, req.LifecycleStatus) {
		return req, "", 0, errors.New("接口状态无效")
	}
	if req.TimeoutSeconds < 1 || req.TimeoutSeconds > 300 {
		return req, "", 0, errors.New("超时时间必须在 1 到 300 秒之间")
	}
	projectID, err := s.repo.ProductProject(ctx, req.ProductID)
	if err != nil {
		return req, "", 0, errors.New("项目/产品不存在")
	}
	return req, normalizeAPIPath(req.Path), projectID, nil
}

func normalizeAPIPath(value string) string {
	if parsed, err := url.Parse(value); err == nil && parsed.IsAbs() {
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.Path = cleanAPIPath(parsed.Path)
		return parsed.String()
	}
	return cleanAPIPath(value)
}

func cleanAPIPath(value string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(strings.Split(value, "?")[0]))
	if cleaned != "/" {
		cleaned = strings.TrimSuffix(cleaned, "/")
	}
	return cleaned
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
