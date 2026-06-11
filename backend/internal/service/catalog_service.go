package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/repository"
)

type CatalogService struct {
	catalogRepo *repository.CatalogRepository
	systemRepo  *repository.SystemRepository
}

const executorTokenSettingKey = "executor_shared_token"

func NewCatalogService(catalogRepo *repository.CatalogRepository, systemRepo *repository.SystemRepository) *CatalogService {
	return &CatalogService{catalogRepo: catalogRepo, systemRepo: systemRepo}
}

func (s *CatalogService) ListMenus(ctx context.Context) ([]model.Menu, error) {
	return s.catalogRepo.ListMenus(ctx)
}

func (s *CatalogService) ListDictionaries(ctx context.Context) ([]model.Dictionary, error) {
	return s.catalogRepo.ListDictionaries(ctx)
}

func (s *CatalogService) ListOperationLogs(ctx context.Context) ([]model.OperationLog, error) {
	return s.catalogRepo.ListOperationLogs(ctx)
}

func (s *CatalogService) GetExecutorToken(ctx context.Context) (model.ExecutorTokenConfig, error) {
	value, updatedAt, err := s.catalogRepo.GetSettingWithUpdatedAt(ctx, executorTokenSettingKey)
	if err != nil {
		return model.ExecutorTokenConfig{}, errors.New("查询执行器 Token 失败")
	}
	return model.ExecutorTokenConfig{MaskedToken: maskToken(value), UpdatedAt: updatedAt}, nil
}

func (s *CatalogService) GenerateExecutorToken(ctx context.Context, actor string) (model.ExecutorTokenGenerateResult, error) {
	token, err := generateExecutorToken()
	if err != nil {
		return model.ExecutorTokenGenerateResult{}, errors.New("生成执行器 Token 失败")
	}
	updatedAt, err := s.catalogRepo.UpsertSetting(ctx, executorTokenSettingKey, token)
	if err != nil {
		return model.ExecutorTokenGenerateResult{}, errors.New("保存执行器 Token 失败")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "生成执行器 Token", "executor_shared_token")
	return model.ExecutorTokenGenerateResult{Token: token, MaskedToken: maskToken(token), UpdatedAt: updatedAt}, nil
}

func (s *CatalogService) ListProjects(ctx context.Context, id, name string, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	items, total, err := s.catalogRepo.ListProjects(ctx, strings.TrimSpace(id), strings.TrimSpace(name), page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *CatalogService) CreateProject(ctx context.Context, actor string, req model.ProjectRequest) error {
	req, err := normalizeProject(req)
	if err != nil {
		return err
	}
	if err := s.catalogRepo.CreateProject(ctx, req); err != nil {
		return errors.New("新增项目失败，项目名称可能已存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "新增项目", req.Name)
	return nil
}

func (s *CatalogService) UpdateProject(ctx context.Context, actor string, id int64, req model.ProjectRequest) error {
	req, err := normalizeProject(req)
	if err != nil {
		return err
	}
	rows, err := s.catalogRepo.UpdateProject(ctx, id, req)
	if err != nil {
		return errors.New("更新项目失败，项目名称可能已存在")
	}
	if rows == 0 {
		return errors.New("项目不存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "编辑项目", fmt.Sprintf("%d:%s", id, req.Name))
	return nil
}

func (s *CatalogService) DeleteProject(ctx context.Context, actor string, id int64) error {
	rows, err := s.catalogRepo.DeleteProject(ctx, id)
	if err != nil {
		return errors.New("删除项目失败")
	}
	if rows == 0 {
		return errors.New("项目不存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "删除项目", strconv.FormatInt(id, 10))
	return nil
}

func (s *CatalogService) ListProducts(ctx context.Context, id, name string, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	items, total, err := s.catalogRepo.ListProducts(ctx, strings.TrimSpace(id), strings.TrimSpace(name), page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *CatalogService) CreateProduct(ctx context.Context, actor string, req model.ProductRequest) error {
	req, err := normalizeProduct(req)
	if err != nil {
		return err
	}
	if err := s.catalogRepo.CreateProduct(ctx, req); err != nil {
		return errors.New("新增产品失败，产品名称可能已存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "新增产品", req.Name)
	return nil
}

func (s *CatalogService) UpdateProduct(ctx context.Context, actor string, id int64, req model.ProductRequest) error {
	req, err := normalizeProduct(req)
	if err != nil {
		return err
	}
	rows, err := s.catalogRepo.UpdateProduct(ctx, id, req)
	if err != nil {
		return errors.New("更新产品失败，产品名称可能已存在")
	}
	if rows == 0 {
		return errors.New("产品不存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "编辑产品", fmt.Sprintf("%d:%s", id, req.Name))
	return nil
}

func (s *CatalogService) DeleteProduct(ctx context.Context, actor string, id int64) error {
	rows, err := s.catalogRepo.DeleteProduct(ctx, id)
	if err != nil {
		return errors.New("删除产品失败")
	}
	if rows == 0 {
		return errors.New("产品不存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "删除产品", strconv.FormatInt(id, 10))
	return nil
}

func normalizeProject(req model.ProjectRequest) (model.ProjectRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Status = strings.TrimSpace(req.Status)
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Name == "" {
		return req, errors.New("项目名称不能为空")
	}
	if req.Status != "active" && req.Status != "disabled" {
		return req, errors.New("项目状态无效")
	}
	return req, nil
}

func generateExecutorToken() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "syn_exec_" + hex.EncodeToString(bytes), nil
}

func maskToken(value string) string {
	if len(value) <= 14 {
		return "******"
	}
	return value[:10] + "..." + value[len(value)-6:]
}

func normalizeProduct(req model.ProductRequest) (model.ProductRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.UIType = strings.ToUpper(strings.TrimSpace(req.UIType))
	req.APIType = strings.ToUpper(strings.TrimSpace(req.APIType))
	if req.UIType == "" {
		req.UIType = "WEB"
	}
	if req.APIType == "" {
		req.APIType = "WEB"
	}
	if req.ProjectID <= 0 {
		return req, errors.New("项目名称不能为空")
	}
	if req.Name == "" {
		return req, errors.New("产品名称不能为空")
	}
	if !validEndpointType(req.UIType) || !validEndpointType(req.APIType) {
		return req, errors.New("产品端类型无效")
	}
	return req, nil
}

func validEndpointType(value string) bool {
	return value == "WEB" || value == "APP" || value == "安卓" || value == "IOS" || value == "NONE"
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
