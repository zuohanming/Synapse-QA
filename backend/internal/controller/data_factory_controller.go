package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type DataFactoryController struct {
	service *service.DataFactoryService
}

func NewDataFactoryController(service *service.DataFactoryService) *DataFactoryController {
	return &DataFactoryController{service: service}
}

func (ctl *DataFactoryController) ListGenerators(c *gin.Context) {
	_, exists := claimsFromContext(c)
	if !exists {
		return
	}
	generators := ctl.service.ListGenerators()
	ok(c, map[string]any{"generators": generators})
}

func (ctl *DataFactoryController) Preview(c *gin.Context) {
	_, exists := claimsFromContext(c)
	if !exists {
		return
	}
	var req model.MockPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	if req.Placeholder == "" {
		fail(c, http.StatusBadRequest, "placeholder 不能为空")
		return
	}
	if req.Count <= 0 {
		req.Count = 5
	}
	if req.Count > 100 {
		req.Count = 100
	}
	result := ctl.service.Preview(req.Placeholder, req.Count)
	ok(c, result)
}
