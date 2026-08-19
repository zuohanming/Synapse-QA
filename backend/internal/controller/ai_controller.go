package controller

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type AIController struct {
	service *service.AIService
}

func NewAIController(svc *service.AIService) *AIController {
	return &AIController{service: svc}
}

func (ctl *AIController) GetConfig(c *gin.Context) {
	claims, valid := claimsFromContext(c)
	if !valid {
		return
	}
	cfg, err := ctl.service.Load(c.Request.Context(), claims.UserID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "加载 AI 配置失败")
		return
	}
	ok(c, cfg)
}

func (ctl *AIController) SaveModel(c *gin.Context) {
	var m model.AIModel
	if err := c.ShouldBindJSON(&m); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	if m.ID == "" || m.Provider == "" || m.Name == "" {
		fail(c, http.StatusBadRequest, "模型 ID、服务商、模型名称不能为空")
		return
	}
	claims, valid := claimsFromContext(c)
	if !valid {
		return
	}
	cfg, err := ctl.service.SaveModel(c.Request.Context(), claims.UserID, m)
	if err != nil {
		fail(c, http.StatusInternalServerError, "保存模型失败")
		return
	}
	ok(c, cfg)
}

func (ctl *AIController) DeleteModel(c *gin.Context) {
	modelID := c.Param("id")
	if modelID == "" {
		fail(c, http.StatusBadRequest, "缺少模型 ID")
		return
	}
	claims, valid := claimsFromContext(c)
	if !valid {
		return
	}
	cfg, err := ctl.service.DeleteModel(c.Request.Context(), claims.UserID, modelID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "删除模型失败")
		return
	}
	ok(c, cfg)
}

func (ctl *AIController) TestConnection(c *gin.Context) {
	var req model.AIConnectionTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	result := ctl.service.TestConnection(c.Request.Context(), req)
	ok(c, result)
}

func (ctl *AIController) SavePreferences(c *gin.Context) {
	var prefs model.AIPreferences
	if err := c.ShouldBindJSON(&prefs); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	claims, valid := claimsFromContext(c)
	if !valid {
		return
	}
	cfg, err := ctl.service.SavePreferences(c.Request.Context(), claims.UserID, prefs)
	if err != nil {
		fail(c, http.StatusInternalServerError, "保存偏好失败")
		return
	}
	ok(c, cfg)
}

func (ctl *AIController) Chat(c *gin.Context) {
	var chatReq model.AIChatRequest
	if err := c.ShouldBindJSON(&chatReq); err != nil {
		fail(c, http.StatusBadRequest, "请求参数无效")
		return
	}
	if len(chatReq.Messages) == 0 {
		fail(c, http.StatusBadRequest, "消息不能为空")
		return
	}
	claims, valid := claimsFromContext(c)
	if !valid {
		return
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan service.SSEEvent, 32)
	go ctl.service.ChatWithTools(c.Request.Context(), claims, chatReq, ch)

	flusher, _ := c.Writer.(http.Flusher)
	for evt := range ch {
		data, _ := json.Marshal(evt)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}
}
