package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/repository"
)

type AutomationService struct {
	automationRepo *repository.AutomationRepository
	systemRepo     *repository.SystemRepository
}

func NewAutomationService(automationRepo *repository.AutomationRepository, systemRepo *repository.SystemRepository) *AutomationService {
	return &AutomationService{automationRepo: automationRepo, systemRepo: systemRepo}
}

func (s *AutomationService) ListTestObjects(ctx context.Context, id, envName, productID string, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	items, total, err := s.automationRepo.ListTestObjects(ctx, strings.TrimSpace(id), strings.TrimSpace(envName), strings.TrimSpace(productID), page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *AutomationService) CreateTestObject(ctx context.Context, actor string, req model.TestObjectRequest) error {
	req, err := normalizeTestObject(req)
	if err != nil {
		return err
	}
	if err := s.automationRepo.CreateTestObject(ctx, req); err != nil {
		return errors.New("新增测试对象失败，环境名称可能已存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "新增测试对象", req.EnvName)
	return nil
}

func (s *AutomationService) UpdateTestObject(ctx context.Context, actor string, id int64, req model.TestObjectRequest) error {
	req, err := normalizeTestObject(req)
	if err != nil {
		return err
	}
	rows, err := s.automationRepo.UpdateTestObject(ctx, id, req)
	if err != nil {
		return errors.New("更新测试对象失败，环境名称可能已存在")
	}
	if rows == 0 {
		return errors.New("测试对象不存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "编辑测试对象", fmt.Sprintf("%d:%s", id, req.EnvName))
	return nil
}

func (s *AutomationService) DeleteTestObject(ctx context.Context, actor string, id int64) error {
	rows, err := s.automationRepo.DeleteTestObject(ctx, id)
	if err != nil {
		return errors.New("删除测试对象失败")
	}
	if rows == 0 {
		return errors.New("测试对象不存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "删除测试对象", strconv.FormatInt(id, 10))
	return nil
}

func (s *AutomationService) ListUIAssets(ctx context.Context, assetType string, filter model.UIAssetFilter, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	filter.ID = strings.TrimSpace(filter.ID)
	filter.PageName = strings.TrimSpace(filter.PageName)
	filter.PageURL = strings.TrimSpace(filter.PageURL)
	filter.Product = strings.TrimSpace(filter.Product)
	filter.Module = strings.TrimSpace(filter.Module)
	items, total, err := s.automationRepo.ListUIAssets(ctx, assetType, filter, page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *AutomationService) CreateUIAsset(ctx context.Context, actor, assetType string, req model.UIAssetRequest) error {
	req, err := normalizeUIAsset(req)
	if err != nil {
		return err
	}
	if err := s.automationRepo.CreateUIAsset(ctx, assetType, req, actor); err != nil {
		return errors.New("新增失败")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "新增界面自动化资源", assetType+":"+req.Name)
	return nil
}

func (s *AutomationService) UpdateUIAsset(ctx context.Context, actor, assetType string, id int64, req model.UIAssetRequest) error {
	req, err := normalizeUIAsset(req)
	if err != nil {
		return err
	}
	rows, err := s.automationRepo.UpdateUIAsset(ctx, id, assetType, req)
	if err != nil {
		return errors.New("更新失败")
	}
	if rows == 0 {
		return errors.New("数据不存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "编辑界面自动化资源", fmt.Sprintf("%s:%d", assetType, id))
	return nil
}

func (s *AutomationService) DeleteUIAsset(ctx context.Context, actor, assetType string, id int64) error {
	rows, err := s.automationRepo.DeleteUIAsset(ctx, id, assetType)
	if err != nil {
		return errors.New("删除失败")
	}
	if rows == 0 {
		return errors.New("数据不存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "删除界面自动化资源", fmt.Sprintf("%s:%d", assetType, id))
	return nil
}

func (s *AutomationService) ListPageElements(ctx context.Context, pageID int64, page, pageSize int) (model.PageResult, error) {
	if pageID <= 0 {
		return model.PageResult{}, errors.New("页面 ID 无效")
	}
	page, pageSize = normalizePage(page, pageSize)
	items, total, err := s.automationRepo.ListPageElements(ctx, pageID, page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *AutomationService) CreatePageElement(ctx context.Context, actor string, req model.PageElementRequest) error {
	req, err := normalizePageElement(req)
	if err != nil {
		return err
	}
	if err := s.automationRepo.CreatePageElement(ctx, req); err != nil {
		return errors.New("新增元素失败")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "新增页面元素", req.Name)
	return nil
}

func (s *AutomationService) UpdatePageElement(ctx context.Context, actor string, id int64, req model.PageElementRequest) error {
	req, err := normalizePageElement(req)
	if err != nil {
		return err
	}
	rows, err := s.automationRepo.UpdatePageElement(ctx, id, req)
	if err != nil {
		return errors.New("更新元素失败")
	}
	if rows == 0 {
		return errors.New("元素不存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "编辑页面元素", fmt.Sprintf("%d:%s", id, req.Name))
	return nil
}

func (s *AutomationService) DeletePageElement(ctx context.Context, actor string, id int64) error {
	rows, err := s.automationRepo.DeletePageElement(ctx, id)
	if err != nil {
		return errors.New("删除元素失败")
	}
	if rows == 0 {
		return errors.New("元素不存在")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "删除页面元素", strconv.FormatInt(id, 10))
	return nil
}

func normalizeTestObject(req model.TestObjectRequest) (model.TestObjectRequest, error) {
	req.EnvName = strings.TrimSpace(req.EnvName)
	req.Target = strings.TrimSpace(req.Target)
	req.DeployEnv = strings.TrimSpace(req.DeployEnv)
	req.AutoType = strings.TrimSpace(req.AutoType)
	req.Owner = strings.TrimSpace(req.Owner)
	if req.DeployEnv == "" {
		req.DeployEnv = "生产环境"
	}
	if req.AutoType == "" {
		req.AutoType = "界面自动化"
	}
	if req.ProductID <= 0 || req.EnvName == "" || req.Target == "" || req.Owner == "" {
		return req, errors.New("项目/产品、环境名称、测试对象和负责人不能为空")
	}
	return req, nil
}

func normalizeUIAsset(req model.UIAssetRequest) (model.UIAssetRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.TrimSpace(req.Category)
	req.Method = strings.TrimSpace(req.Method)
	req.Locator = strings.TrimSpace(req.Locator)
	req.Action = strings.TrimSpace(req.Action)
	req.Value = strings.TrimSpace(req.Value)
	req.Description = strings.TrimSpace(req.Description)
	req.Status = strings.TrimSpace(req.Status)
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Name == "" {
		return req, errors.New("名称不能为空")
	}
	if req.Status != "active" && req.Status != "disabled" {
		return req, errors.New("状态无效")
	}
	return req, nil
}

func normalizePageElement(req model.PageElementRequest) (model.PageElementRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Type1 = strings.TrimSpace(req.Type1)
	req.Locator1 = strings.TrimSpace(req.Locator1)
	req.Index1 = strings.TrimSpace(req.Index1)
	req.Type2 = strings.TrimSpace(req.Type2)
	req.Locator2 = strings.TrimSpace(req.Locator2)
	req.Index2 = strings.TrimSpace(req.Index2)
	req.Type3 = strings.TrimSpace(req.Type3)
	req.Locator3 = strings.TrimSpace(req.Locator3)
	req.Index3 = strings.TrimSpace(req.Index3)
	req.AIPrompt = strings.TrimSpace(req.AIPrompt)
	req.WaitTime = strings.TrimSpace(req.WaitTime)
	if req.PageID <= 0 || req.Name == "" || req.Type1 == "" || req.Locator1 == "" {
		return req, errors.New("页面、元素名称、类型-1和定位-1不能为空")
	}
	return req, nil
}
