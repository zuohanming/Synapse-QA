package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type APIAutomationController struct {
	service *service.APIAutomationService
}

func NewAPIAutomationController(service *service.APIAutomationService) *APIAutomationController {
	return &APIAutomationController{service: service}
}

func (ctl *APIAutomationController) ListInterfaces(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	page, pageSize := pageParams(c)
	result, err := ctl.service.ListInterfaces(c.Request.Context(), claims.UserID, model.APIInterfaceFilter{
		ProjectID: c.Query("projectId"), ProductID: c.Query("productId"), ModuleID: c.Query("moduleId"),
		Keyword: c.Query("keyword"), Method: c.Query("method"), LifecycleStatus: c.Query("lifecycleStatus"),
		IncludeDeleted: c.Query("includeDeleted") == "true", DeletedOnly: c.Query("deletedOnly") == "true",
	}, page, pageSize)
	if err != nil {
		fail(c, http.StatusBadRequest, "查询接口失败")
		return
	}
	ok(c, result)
}

func (ctl *APIAutomationController) RestoreInterface(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.service.RestoreInterface(c.Request.Context(), claims.UserID, claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "接口已恢复"})
}

func (ctl *APIAutomationController) BatchDeleteInterfaces(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.APIInterfaceBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		fail(c, http.StatusBadRequest, "请选择接口")
		return
	}
	ok(c, ctl.service.BatchDeleteInterfaces(c.Request.Context(), claims.UserID, claims.Username, req.IDs))
}

func (ctl *APIAutomationController) BatchUpdateInterfaceStatus(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.APIInterfaceBatchStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		fail(c, http.StatusBadRequest, "请选择接口")
		return
	}
	ok(c, ctl.service.BatchUpdateInterfaceStatus(c.Request.Context(), claims.UserID, claims.Username, req.IDs, req.Status))
}

func (ctl *APIAutomationController) BatchMoveInterfaces(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.APIInterfaceBatchMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		fail(c, http.StatusBadRequest, "请选择接口")
		return
	}
	ok(c, ctl.service.BatchMoveInterfaces(c.Request.Context(), claims.UserID, claims.Username, req.IDs, req.ProductID, req.ModuleID))
}

func (ctl *APIAutomationController) GetInterface(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := ctl.service.GetInterface(c.Request.Context(), claims.UserID, id)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *APIAutomationController) CreateInterface(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.APIInterfaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	id, err := ctl.service.CreateInterface(c.Request.Context(), claims.UserID, claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]any{"id": id, "message": "接口已创建"})
}

func (ctl *APIAutomationController) UpdateInterface(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.APIInterfaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if header := c.GetHeader("If-Match"); header != "" {
		req.Revision, _ = strconv.ParseInt(header, 10, 64)
	}
	if err := ctl.service.UpdateInterface(c.Request.Context(), claims.UserID, claims.Username, id, req); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "接口已被其他用户修改，请刷新后重试" {
			status = http.StatusConflict
		}
		fail(c, status, err.Error())
		return
	}
	ok(c, map[string]string{"message": "接口已更新"})
}

func (ctl *APIAutomationController) DeleteInterface(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.service.DeleteInterface(c.Request.Context(), claims.UserID, claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "接口已删除"})
}

func (ctl *APIAutomationController) PreviewRequest(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.APIRequestPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	result, err := ctl.service.PreviewRequest(c.Request.Context(), claims.UserID, id, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *APIAutomationController) ParseCurl(c *gin.Context) {
	var req model.APICurlParseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	result, err := service.ParseAPICurl(req.Curl)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *APIAutomationController) ExportCurl(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.APIRequestPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	preview, err := ctl.service.PreviewRequest(c.Request.Context(), claims.UserID, id, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, model.APICurlExportResult{Curl: service.BuildMaskedCurl(preview)})
}

func (ctl *APIAutomationController) UploadTempFile(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (10<<20)+(1<<20))
	projectID, err := strconv.ParseInt(c.PostForm("projectId"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "请选择项目")
		return
	}
	header, err := c.FormFile("file")
	if err != nil {
		fail(c, http.StatusBadRequest, "请选择上传文件")
		return
	}
	file, err := header.Open()
	if err != nil {
		fail(c, http.StatusBadRequest, "读取上传文件失败")
		return
	}
	defer file.Close()
	item, err := ctl.service.UploadTempFile(c.Request.Context(), claims.UserID, projectID, header.Filename, header.Header.Get("Content-Type"), header.Size, file)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, item)
}

func (ctl *APIAutomationController) DeleteTempFile(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	if err := ctl.service.DeleteTempFile(c.Request.Context(), claims.UserID, c.Param("id")); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "临时文件已删除"})
}

func (ctl *APIAutomationController) StartDebug(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.APIDebugStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	run, err := ctl.service.StartDebug(c.Request.Context(), claims.UserID, id, claims.Username, req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, run)
}

func (ctl *APIAutomationController) GetDebug(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	run, err := ctl.service.GetDebug(c.Request.Context(), claims.UserID, c.Param("taskId"))
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, run)
}

func (ctl *APIAutomationController) ListDebugEvents(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	after, _ := strconv.Atoi(c.Query("after"))
	events, err := ctl.service.ListDebugEvents(c.Request.Context(), claims.UserID, c.Param("taskId"), after)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, events)
}

func (ctl *APIAutomationController) StreamDebugEvents(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	taskID := c.Param("taskId")
	after, _ := strconv.Atoi(c.Query("after"))
	if _, err := ctl.service.GetDebug(c.Request.Context(), claims.UserID, taskID); err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(c.Writer, ": heartbeat\n\n")
			c.Writer.Flush()
		case <-ticker.C:
			events, err := ctl.service.ListDebugEvents(c.Request.Context(), claims.UserID, taskID, after)
			if err != nil {
				return
			}
			terminal := false
			for _, event := range events {
				payload, _ := json.Marshal(event)
				_, _ = fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.Type, payload)
				after = event.Sequence
				terminal = event.Status == "success" || event.Status == "failed" || event.Status == "canceled"
			}
			if len(events) > 0 {
				c.Writer.Flush()
			}
			if terminal {
				return
			}
		}
	}
}

func (ctl *APIAutomationController) CancelDebug(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	if err := ctl.service.CancelDebug(c.Request.Context(), claims.UserID, c.Param("taskId")); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "调试任务已取消"})
}

func (ctl *APIAutomationController) ListInterfaceDebugRuns(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	filter := model.APIDebugRunFilter{Status: c.Query("status"), ExecutorID: c.Query("executorId")}
	filter.Limit, _ = strconv.Atoi(c.Query("limit"))
	if value := c.Query("dateFrom"); value != "" {
		parsed, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			fail(c, http.StatusBadRequest, "开始时间格式应为 RFC3339")
			return
		}
		filter.DateFrom = &parsed
	}
	if value := c.Query("dateTo"); value != "" {
		parsed, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			fail(c, http.StatusBadRequest, "结束时间格式应为 RFC3339")
			return
		}
		filter.DateTo = &parsed
	}
	items, err := ctl.service.ListInterfaceDebugRuns(c.Request.Context(), claims.UserID, id, filter)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, items)
}

func (ctl *APIAutomationController) GetDebugRunDetail(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := ctl.service.GetDebugRunDetail(c.Request.Context(), claims.UserID, id)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *APIAutomationController) ListInterfaceVersions(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	items, err := ctl.service.ListInterfaceVersions(c.Request.Context(), claims.UserID, id)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, items)
}

func (ctl *APIAutomationController) GetInterfaceVersion(c *gin.Context) {
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
	item, err := ctl.service.GetInterfaceVersion(c.Request.Context(), claims.UserID, id, version)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, item)
}

func (ctl *APIAutomationController) DiffInterfaceVersions(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	source, sourceErr := strconv.Atoi(c.Param("version"))
	target, targetErr := strconv.Atoi(c.Query("targetVersion"))
	if sourceErr != nil || targetErr != nil || source <= 0 || target <= 0 {
		fail(c, http.StatusBadRequest, "版本号无效")
		return
	}
	diff, err := ctl.service.DiffInterfaceVersions(c.Request.Context(), claims.UserID, id, source, target)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, diff)
}

func (ctl *APIAutomationController) RestoreInterfaceVersion(c *gin.Context) {
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
	newVersion, err := ctl.service.RestoreInterfaceVersion(c.Request.Context(), claims.UserID, claims.Username, id, version)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]any{"message": "版本已回滚", "version": newVersion})
}

func (ctl *APIAutomationController) DebugCallback(c *gin.Context) {
	var req model.APIDebugCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TaskID != c.Param("taskId") {
		fail(c, http.StatusBadRequest, "回调请求无效")
		return
	}
	if err := ctl.service.CompleteDebug(c.Request.Context(), req.TaskID, req.Status, req.Result); err != nil {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	ok(c, map[string]string{"message": "回调已接收"})
}

func (ctl *APIAutomationController) DebugEventCallback(c *gin.Context) {
	var event model.APIDebugEvent
	if err := c.ShouldBindJSON(&event); err != nil || event.TaskID != c.Param("taskId") {
		fail(c, http.StatusBadRequest, "事件请求无效")
		return
	}
	if err := ctl.service.AppendDebugEvent(c.Request.Context(), event); err != nil {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	ok(c, map[string]string{"message": "事件已接收"})
}

func (ctl *APIAutomationController) ListProjectHeaders(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	projectID, err := strconv.ParseInt(c.Query("projectId"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "请选择项目")
		return
	}
	result, err := ctl.service.ListProjectHeaders(c.Request.Context(), claims.UserID, projectID, c.Query("keyword"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, result)
}

func (ctl *APIAutomationController) CreateProjectHeader(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.APIProjectHeaderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.service.CreateProjectHeader(c.Request.Context(), claims.UserID, claims.Username, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, map[string]string{"message": "请求头已创建"})
}

func (ctl *APIAutomationController) UpdateProjectHeader(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req model.APIProjectHeaderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := ctl.service.UpdateProjectHeader(c.Request.Context(), claims.UserID, claims.Username, id, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "请求头已更新"})
}

func (ctl *APIAutomationController) DeleteProjectHeader(c *gin.Context) {
	claims, exists := claimsFromContext(c)
	if !exists {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.service.DeleteProjectHeader(c.Request.Context(), claims.UserID, claims.Username, id); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, map[string]string{"message": "请求头已删除"})
}
