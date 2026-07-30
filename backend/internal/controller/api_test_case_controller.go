package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
)

func (ctl *APIAutomationController) ListGlobalVariables(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	result, err := ctl.service.ListAPIGlobalVariables(c.Request.Context(), claims.UserID, model.APIGlobalVariableFilter{
		ScopeType: c.Query("scopeType"), ProjectID: c.Query("projectId"), ProductID: c.Query("productId"),
		EnvName: c.Query("envName"), Keyword: c.Query("keyword"),
	})
	if err != nil {
		fail(c, http.StatusBadRequest, "查询接口全局变量失败")
		return
	}
	ok(c, result)
}

func (ctl *APIAutomationController) CreateGlobalVariable(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.APIGlobalVariableRequest
	if c.ShouldBindJSON(&req) != nil {
		fail(c, http.StatusBadRequest, "全局变量参数无效")
		return
	}
	id, err := ctl.service.CreateAPIGlobalVariable(c.Request.Context(), claims.UserID, claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, gin.H{"id": id, "message": "全局变量已创建"})
}

func (ctl *APIAutomationController) UpdateGlobalVariable(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.APIGlobalVariableRequest
	if c.ShouldBindJSON(&req) != nil {
		fail(c, http.StatusBadRequest, "全局变量参数无效")
		return
	}
	revision, err := ctl.service.UpdateAPIGlobalVariable(c.Request.Context(), claims.UserID, claims.Username, id, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"revision": revision, "message": "全局变量已更新"})
}

func (ctl *APIAutomationController) DeleteGlobalVariable(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.service.DeleteAPIGlobalVariable(c.Request.Context(), claims.UserID, claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"message": "全局变量已删除"})
}

func (ctl *APIAutomationController) ListTestCases(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	page, pageSize := pageParams(c)
	result, err := ctl.service.ListAPITestCases(c.Request.Context(), claims.UserID, model.APITestCaseFilter{
		ProjectID: c.Query("projectId"), ProductID: c.Query("productId"), ModuleID: c.Query("moduleId"),
		Keyword: c.Query("keyword"), Status: c.Query("status"), Priority: c.Query("priority"), Owner: c.Query("owner"),
	}, page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询接口测试用例失败")
		return
	}
	ok(c, result)
}

func (ctl *APIAutomationController) GetTestCase(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := ctl.service.GetAPITestCase(c.Request.Context(), claims.UserID, id)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *APIAutomationController) CreateTestCase(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.APITestCaseRequest
	if c.ShouldBindJSON(&req) != nil {
		fail(c, http.StatusBadRequest, "接口测试用例参数无效")
		return
	}
	id, err := ctl.service.CreateAPITestCase(c.Request.Context(), claims.UserID, claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, gin.H{"id": id, "message": "接口测试用例已创建"})
}

func (ctl *APIAutomationController) UpdateTestCase(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.APITestCaseRequest
	if c.ShouldBindJSON(&req) != nil {
		fail(c, http.StatusBadRequest, "接口测试用例参数无效")
		return
	}
	revision, err := ctl.service.UpdateAPITestCase(c.Request.Context(), claims.UserID, claims.Username, id, req)
	if err != nil {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	ok(c, gin.H{"revision": revision, "message": "草稿已保存"})
}

func (ctl *APIAutomationController) DeleteTestCase(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.service.DeleteAPITestCase(c.Request.Context(), claims.UserID, claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"message": "接口测试用例已删除"})
}

func (ctl *APIAutomationController) ValidateTestCase(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	result, err := ctl.service.ValidateAPITestCase(c.Request.Context(), claims.UserID, id)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *APIAutomationController) PublishTestCase(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.APITestCasePublishRequest
	if c.ShouldBindJSON(&req) != nil {
		fail(c, http.StatusBadRequest, "发布参数无效")
		return
	}
	version, err := ctl.service.PublishAPITestCase(c.Request.Context(), claims.UserID, claims.Username, id, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"version": version, "message": "接口测试用例已发布"})
}

func (ctl *APIAutomationController) ListTestCaseVersions(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	items, err := ctl.service.ListAPITestCaseVersions(c.Request.Context(), claims.UserID, id)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, items)
}

func (ctl *APIAutomationController) GetTestCaseVersion(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil || version <= 0 {
		fail(c, http.StatusBadRequest, "版本号无效")
		return
	}
	item, err := ctl.service.GetAPITestCaseVersion(c.Request.Context(), claims.UserID, id, version)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *APIAutomationController) StartTestRun(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.APITestRunStartRequest
	if c.ShouldBindJSON(&req) != nil {
		fail(c, http.StatusBadRequest, "执行参数无效")
		return
	}
	batch, err := ctl.service.StartAPITestRun(c.Request.Context(), claims.UserID, claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, batch)
}

func (ctl *APIAutomationController) GetTestRun(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	result, err := ctl.service.GetAPITestRun(c.Request.Context(), claims.UserID, c.Param("batchId"))
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *APIAutomationController) TestRunCallback(c *gin.Context) {
	var req struct {
		TaskID string `json:"taskId"`
		Status string `json:"status"`
		Result *struct {
			Output string `json:"output"`
			Error  string `json:"error"`
		} `json:"result"`
	}
	if c.ShouldBindJSON(&req) != nil {
		fail(c, http.StatusBadRequest, "执行回调参数无效")
		return
	}
	if req.TaskID == "" {
		req.TaskID = c.Param("taskId")
	}
	result := json.RawMessage(`{}`)
	errorMessage := ""
	if req.Result != nil {
		errorMessage = req.Result.Error
		if json.Valid([]byte(req.Result.Output)) {
			result = json.RawMessage(req.Result.Output)
		} else if req.Result.Output != "" {
			encoded, _ := json.Marshal(gin.H{"output": req.Result.Output})
			result = encoded
		}
	}
	if err := ctl.service.CompleteAPITestRun(c.Request.Context(), req.TaskID, req.Status, result, errorMessage); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"message": "执行结果已接收"})
}
