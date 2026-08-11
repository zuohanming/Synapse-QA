package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type CatalogController struct {
	catalogService *service.CatalogService
}

func NewCatalogController(catalogService *service.CatalogService) *CatalogController {
	return &CatalogController{catalogService: catalogService}
}

func (ctl *CatalogController) ListMenus(c *gin.Context) {
	items, err := ctl.catalogService.ListMenus(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询菜单失败")
		return
	}
	ok(c, items)
}

func (ctl *CatalogController) ListDictionaries(c *gin.Context) {
	items, err := ctl.catalogService.ListDictionaries(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询字典失败")
		return
	}
	ok(c, items)
}

func (ctl *CatalogController) ListLogs(c *gin.Context) {
	items, err := ctl.catalogService.ListOperationLogs(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "查询日志失败")
		return
	}
	ok(c, items)
}

func (ctl *CatalogController) GetExecutorToken(c *gin.Context) {
	item, err := ctl.catalogService.GetExecutorToken(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *CatalogController) GenerateExecutorToken(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	item, err := ctl.catalogService.GenerateExecutorToken(c.Request.Context(), claims.Username)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *CatalogController) ListProjects(c *gin.Context) {
	page, pageSize := pageParams(c)
	result, err := ctl.catalogService.ListProjects(c.Request.Context(), c.Query("id"), c.Query("name"), page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询项目失败")
		return
	}
	ok(c, result)
}

func (ctl *CatalogController) CreateProject(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.ProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.catalogService.CreateProject(c.Request.Context(), claims.Username, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "项目已创建"})
}

func (ctl *CatalogController) UpdateProject(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.ProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.catalogService.UpdateProject(c.Request.Context(), claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "项目已更新"})
}

func (ctl *CatalogController) DeleteProject(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.catalogService.DeleteProject(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "项目已删除"})
}

func (ctl *CatalogController) ListProducts(c *gin.Context) {
	page, pageSize := pageParams(c)
	result, err := ctl.catalogService.ListProducts(c.Request.Context(), c.Query("id"), c.Query("name"), page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询产品失败")
		return
	}
	ok(c, result)
}

func (ctl *CatalogController) CreateProduct(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.ProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.catalogService.CreateProduct(c.Request.Context(), claims.Username, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "产品已创建"})
}

func (ctl *CatalogController) UpdateProduct(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.ProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.catalogService.UpdateProduct(c.Request.Context(), claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "产品已更新"})
}

func (ctl *CatalogController) DeleteProduct(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.catalogService.DeleteProduct(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "产品已删除"})
}

func (ctl *CatalogController) ProductStats(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := ctl.catalogService.ProductStats(c.Request.Context(), id)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询产品统计失败")
		return
	}
	ok(c, item)
}

func (ctl *CatalogController) CopyProduct(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.ProductCopyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.catalogService.CopyProduct(c.Request.Context(), claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "产品已复制"})
}

func (ctl *CatalogController) ListProductModules(c *gin.Context) {
	productID, err := strconv.ParseInt(c.Query("productId"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "产品ID无效")
		return
	}
	page, pageSize := pageParams(c)
	result, err := ctl.catalogService.ListProductModules(c.Request.Context(), productID, c.Query("level1"), c.Query("level2"), c.Query("name"), page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询产品模块失败")
		return
	}
	ok(c, result)
}

func (ctl *CatalogController) CreateProductModule(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.ProductModuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.catalogService.CreateProductModule(c.Request.Context(), claims.Username, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "产品模块已创建"})
}

func (ctl *CatalogController) UpdateProductModule(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.ProductModuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.catalogService.UpdateProductModule(c.Request.Context(), claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "产品模块已更新"})
}

func (ctl *CatalogController) DeleteProductModule(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.catalogService.DeleteProductModule(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "产品模块已删除"})
}

func pageParams(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	return page, pageSize
}
