package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type AutomationController struct {
	automationService *service.AutomationService
}

func NewAutomationController(automationService *service.AutomationService) *AutomationController {
	return &AutomationController{automationService: automationService}
}

func (ctl *AutomationController) ListTestObjects(c *gin.Context) {
	page, pageSize := pageParams(c)
	result, err := ctl.automationService.ListTestObjects(c.Request.Context(), c.Query("id"), c.Query("envName"), c.Query("productId"), page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询测试对象失败")
		return
	}
	ok(c, result)
}

func (ctl *AutomationController) CreateTestObject(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.TestObjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.automationService.CreateTestObject(c.Request.Context(), claims.Username, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "测试对象已创建"})
}

func (ctl *AutomationController) UpdateTestObject(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.TestObjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.automationService.UpdateTestObject(c.Request.Context(), claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "测试对象已更新"})
}

func (ctl *AutomationController) DeleteTestObject(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.automationService.DeleteTestObject(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "测试对象已删除"})
}

func (ctl *AutomationController) ListUIAssets(assetType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, pageSize := pageParams(c)
		result, err := ctl.automationService.ListUIAssets(c.Request.Context(), assetType, uiAssetFilter(c), page, pageSize)
		if err != nil {
			fail(c, http.StatusBadRequest, "查询数据失败")
			return
		}
		ok(c, result)
	}
}

func (ctl *AutomationController) CreateUIAsset(assetType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := claimsFromContext(c)
		if !exists {
			return
		}
		var req model.UIAssetRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, "请求体格式错误")
			return
		}
		if err := ctl.automationService.CreateUIAsset(c.Request.Context(), claims.Username, assetType, req); err != nil {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
		created(c, map[string]string{"message": "创建成功"})
	}
}

func (ctl *AutomationController) UpdateUIAsset(assetType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := claimsFromContext(c)
		if !exists {
			return
		}
		id, valid := idParam(c)
		if !valid {
			return
		}
		var req model.UIAssetRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, "请求体格式错误")
			return
		}
		if err := ctl.automationService.UpdateUIAsset(c.Request.Context(), claims.Username, assetType, id, req); err != nil {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
		ok(c, map[string]string{"message": "更新成功"})
	}
}

func (ctl *AutomationController) DeleteUIAsset(assetType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := claimsFromContext(c)
		if !exists {
			return
		}
		id, valid := idParam(c)
		if !valid {
			return
		}
		if err := ctl.automationService.DeleteUIAsset(c.Request.Context(), claims.Username, assetType, id); err != nil {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
		ok(c, map[string]string{"message": "删除成功"})
	}
}

func (ctl *AutomationController) ListPageElements(c *gin.Context) {
	pageID, err := strconv.ParseInt(c.Query("pageId"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "页面 ID 无效")
		return
	}
	page, pageSize := pageParams(c)
	result, err := ctl.automationService.ListPageElements(c.Request.Context(), pageID, page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *AutomationController) CreatePageElement(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.PageElementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.automationService.CreatePageElement(c.Request.Context(), claims.Username, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "元素已创建"})
}

func (ctl *AutomationController) UpdatePageElement(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.PageElementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.automationService.UpdatePageElement(c.Request.Context(), claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "元素已更新"})
}

func (ctl *AutomationController) DeletePageElement(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.automationService.DeletePageElement(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "元素已删除"})
}

func uiAssetFilter(c *gin.Context) model.UIAssetFilter {
	return model.UIAssetFilter{
		Keyword:  c.Query("keyword"),
		ID:       c.Query("id"),
		PageName: c.Query("pageName"),
		PageURL:  c.Query("pageUrl"),
		Product:  c.Query("product"),
		Module:   c.Query("module"),
	}
}
