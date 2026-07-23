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

type NotificationController struct{ service *service.NotificationService }

func NewNotificationController(s *service.NotificationService) *NotificationController {
	return &NotificationController{service: s}
}
func (ctl *NotificationController) List(c *gin.Context) {
	claims, okc := claimsFromContext(c)
	if !okc {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	result, err := ctl.service.List(c.Request.Context(), claims.UserID, c.Query("unread") == "true", c.Query("category"), page, size)
	if err != nil {
		fail(c, 500, "查询通知失败")
		return
	}
	ok(c, result)
}
func (ctl *NotificationController) Count(c *gin.Context) {
	claims, okc := claimsFromContext(c)
	if !okc {
		return
	}
	n, err := ctl.service.UnreadCount(c.Request.Context(), claims.UserID)
	if err != nil {
		fail(c, 500, "查询未读通知失败")
		return
	}
	ok(c, gin.H{"count": n})
}
func (ctl *NotificationController) Read(c *gin.Context) {
	claims, okc := claimsFromContext(c)
	if !okc {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.service.MarkRead(c.Request.Context(), claims.UserID, id); err != nil {
		fail(c, 500, "标记已读失败")
		return
	}
	ok(c, gin.H{"message": "已读"})
}
func (ctl *NotificationController) ReadAll(c *gin.Context) {
	claims, okc := claimsFromContext(c)
	if !okc {
		return
	}
	if err := ctl.service.MarkAllRead(c.Request.Context(), claims.UserID); err != nil {
		fail(c, 500, "全部已读失败")
		return
	}
	ok(c, gin.H{"message": "已全部读"})
}
func (ctl *NotificationController) Delete(c *gin.Context) {
	claims, okc := claimsFromContext(c)
	if !okc {
		return
	}
	id, valid := idParam(c)
	if !valid {
		return
	}
	if err := ctl.service.Delete(c.Request.Context(), claims.UserID, id); err != nil {
		fail(c, 500, "删除通知失败")
		return
	}
	ok(c, gin.H{"message": "已删除"})
}
func (ctl *NotificationController) Preferences(c *gin.Context) {
	claims, okc := claimsFromContext(c)
	if !okc {
		return
	}
	p, err := ctl.service.Preferences(c.Request.Context(), claims.UserID)
	if err != nil {
		fail(c, 500, "查询通知偏好失败")
		return
	}
	ok(c, p)
}
func (ctl *NotificationController) UpdatePreferences(c *gin.Context) {
	claims, okc := claimsFromContext(c)
	if !okc {
		return
	}
	var p model.NotificationPreference
	if c.ShouldBindJSON(&p) != nil {
		fail(c, 400, "请求参数无效")
		return
	}
	if err := ctl.service.UpdatePreferences(c.Request.Context(), claims.UserID, p); err != nil {
		fail(c, 500, "保存通知偏好失败")
		return
	}
	ok(c, p)
}
func (ctl *NotificationController) System(c *gin.Context) {
	claims, okc := claimsFromContext(c)
	if !okc {
		return
	}
	_ = claims
	var req model.SystemNotificationRequest
	if c.ShouldBindJSON(&req) != nil || req.Title == "" {
		fail(c, 400, "通知标题不能为空")
		return
	}
	if req.Level == "" {
		req.Level = "info"
	}
	err := ctl.service.Broadcast(c.Request.Context(), model.NotificationCreate{Type: "system", Level: req.Level, Title: req.Title, Content: req.Content, TargetType: "system"})
	if err != nil {
		fail(c, 500, "发布系统通知失败")
		return
	}
	ok(c, gin.H{"message": "通知已发布"})
}
func (ctl *NotificationController) Stream(c *gin.Context) {
	claims, okc := claimsFromContext(c)
	if !okc {
		return
	}
	flusher, okf := c.Writer.(http.Flusher)
	if !okf {
		fail(c, 500, "浏览器不支持实时通知")
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	ch, cancel := ctl.service.Subscribe(claims.UserID)
	defer cancel()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case n := <-ch:
			data, _ := json.Marshal(n)
			fmt.Fprintf(c.Writer, "event: notification\ndata: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(c.Writer, ": ping\n\n")
			flusher.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}
