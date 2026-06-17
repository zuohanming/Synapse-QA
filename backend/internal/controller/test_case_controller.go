package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type TestCaseController struct {
	testCaseService *service.TestCaseService
}

func NewTestCaseController(testCaseService *service.TestCaseService) *TestCaseController {
	return &TestCaseController{testCaseService: testCaseService}
}

func (ctl *TestCaseController) List(c *gin.Context) {
	page, pageSize := pageParams(c)
	result, err := ctl.testCaseService.List(c.Request.Context(), testCaseFilter(c), page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询测试用例失败")
		return
	}
	ok(c, result)
}

func (ctl *TestCaseController) Get(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	result, err := ctl.testCaseService.Get(c.Request.Context(), id)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *TestCaseController) Create(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.TestCaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	id, err := ctl.testCaseService.Create(c.Request.Context(), claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]any{"id": id, "message": "测试用例已创建"})
}

func (ctl *TestCaseController) Update(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.TestCaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.testCaseService.Update(c.Request.Context(), claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "测试用例已更新"})
}

func (ctl *TestCaseController) Delete(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.testCaseService.Delete(c.Request.Context(), claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "测试用例已删除"})
}

func (ctl *TestCaseController) Import(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.TestCaseImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	count, err := ctl.testCaseService.Import(c.Request.Context(), claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]any{"count": count, "message": "测试用例已导入"})
}

func (ctl *TestCaseController) Export(c *gin.Context) {
	items, err := ctl.testCaseService.Export(c.Request.Context(), testCaseFilter(c))
	if err != nil {
		fail(c, http.StatusBadRequest, "导出测试用例失败")
		return
	}
	ok(c, map[string]any{"items": items, "total": len(items)})
}

func (ctl *TestCaseController) ListDatasets(c *gin.Context) {
	caseID, valid := idParam(c)
	if !valid {
		return
	}
	result, err := ctl.testCaseService.ListDatasets(c.Request.Context(), caseID)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *TestCaseController) CreateDataset(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	caseID, valid := idParam(c)
	if !valid {
		return
	}
	var req model.TestCaseDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.testCaseService.CreateDataset(c.Request.Context(), claims.Username, caseID, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "参数化数据已创建"})
}

func (ctl *TestCaseController) UpdateDataset(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	caseID, datasetID, valid := datasetParams(c)
	if !valid {
		return
	}
	var req model.TestCaseDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.testCaseService.UpdateDataset(c.Request.Context(), claims.Username, caseID, datasetID, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "参数化数据已更新"})
}

func (ctl *TestCaseController) DeleteDataset(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	caseID, datasetID, valid := datasetParams(c)
	if !valid {
		return
	}
	if err := ctl.testCaseService.DeleteDataset(c.Request.Context(), claims.Username, caseID, datasetID); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "参数化数据已删除"})
}

func datasetParams(c *gin.Context) (int64, int64, bool) {
	caseID, valid := idParam(c)
	if !valid {
		return 0, 0, false
	}
	datasetID, err := strconv.ParseInt(c.Param("datasetId"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "参数化数据 ID 无效")
		return 0, 0, false
	}
	return caseID, datasetID, true
}

func testCaseFilter(c *gin.Context) model.TestCaseFilter {
	return model.TestCaseFilter{
		ID:        c.Query("id"),
		Name:      c.Query("name"),
		ProductID: c.Query("productId"),
		ModuleID:  c.Query("moduleId"),
		CaseType:  c.Query("caseType"),
		Priority:  c.Query("priority"),
		Status:    c.Query("status"),
		Owner:     c.Query("owner"),
	}
}
