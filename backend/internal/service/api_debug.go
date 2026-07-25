package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
)

func (s *APIAutomationService) StartDebug(ctx context.Context, userID, interfaceID int64, actor string, req model.APIDebugStartRequest) (model.APIDebugRun, error) {
	if s.executorRepo == nil {
		return model.APIDebugRun{}, errors.New("执行器调度尚未配置")
	}
	item, err := s.repo.GetInterface(ctx, userID, interfaceID)
	if err != nil || !s.repo.CanAccessProject(ctx, userID, item.ProjectID) {
		return model.APIDebugRun{}, errors.New("接口不存在或无权访问")
	}
	executor, err := s.pickAPIExecutor(ctx)
	if err != nil {
		return model.APIDebugRun{}, err
	}
	preview, err := s.buildAPIRequest(ctx, userID, interfaceID, model.APIRequestPreviewRequest{
		TestObjectID: req.TestObjectID, Snapshot: req.Snapshot,
		TemporaryVariables: req.TemporaryVariables, TemporaryHeaders: req.TemporaryHeaders,
	}, false)
	if err != nil {
		return model.APIDebugRun{}, err
	}
	taskID := fmt.Sprintf("api-debug-%d", time.Now().UnixNano())
	requestSnapshot, _ := json.Marshal(preview)
	run, err := s.repo.CreateDebugRun(ctx, taskID, interfaceID, item.ProjectID, executor.ExecutorID, actor, maskDebugRequest(requestSnapshot))
	if err != nil {
		return model.APIDebugRun{}, errors.New("创建调试记录失败")
	}
	_ = s.repo.CreateDebugEvent(ctx, model.APIDebugEvent{
		TaskID: taskID, Sequence: 1, Type: "task.queued", Stage: "queue",
		Status: "queued", Message: "调试任务已创建", Progress: 5, Data: json.RawMessage(`{}`),
	})
	payload := map[string]any{
		"method": preview.Method, "url": preview.URL, "headers": preview.Headers, "body": preview.Body,
		"timeoutSeconds": item.TimeoutSeconds, "maxResponseBytes": 20 << 20,
	}
	callbackURL := s.callbackBase + "/api/api-automation/debug/" + taskID + "/callback"
	eventURL := s.callbackBase + "/api/api-automation/debug/" + taskID + "/events/callback"
	if err := submitAPIDebugTask(ctx, executor.Endpoint, taskID, payload, callbackURL, eventURL); err != nil {
		_ = s.repo.UpdateDebugRun(ctx, taskID, "failed", json.RawMessage(`{}`), err.Error())
		_ = s.repo.CreateDebugEvent(ctx, model.APIDebugEvent{
			TaskID: taskID, Sequence: 2, Type: "task.failed", Stage: "dispatch",
			Status: "failed", Message: "下发任务到执行器失败", Progress: 100, Data: json.RawMessage(`{}`),
		})
		return run, errors.New("下发任务到执行器失败")
	}
	_ = s.repo.UpdateDebugRun(ctx, taskID, "running", json.RawMessage(`{}`), "")
	run.Status = "running"
	return run, nil
}

func (s *APIAutomationService) GetDebug(ctx context.Context, userID int64, taskID string) (model.APIDebugRun, error) {
	run, err := s.repo.GetDebugRun(ctx, taskID)
	if err != nil || !s.repo.CanAccessProject(ctx, userID, run.ProjectID) {
		return run, errors.New("调试任务不存在或无权访问")
	}
	return run, nil
}

func (s *APIAutomationService) ListDebugEvents(ctx context.Context, userID int64, taskID string, after int) ([]model.APIDebugEvent, error) {
	if _, err := s.GetDebug(ctx, userID, taskID); err != nil {
		return nil, err
	}
	return s.repo.ListDebugEvents(ctx, taskID, after)
}

func (s *APIAutomationService) CompleteDebug(ctx context.Context, taskID, status string, result json.RawMessage) error {
	run, err := s.repo.GetDebugRun(ctx, taskID)
	if err != nil || contains([]string{"success", "failed", "canceled"}, run.Status) {
		return errors.New("调试任务不存在或已结束")
	}
	if status != "success" && status != "canceled" {
		status = "failed"
	}
	var parsed struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(result, &parsed)
	if len(result) == 0 {
		result = json.RawMessage(`{}`)
	}
	if err := s.repo.UpdateDebugRun(ctx, taskID, status, result, parsed.Error); err != nil {
		return err
	}
	var taskResult struct {
		Output string `json:"output"`
	}
	var responseResult struct {
		DurationMS int64 `json:"durationMs"`
	}
	if json.Unmarshal(result, &taskResult) == nil {
		_ = json.Unmarshal([]byte(taskResult.Output), &responseResult)
	}
	_ = s.repo.UpdateInterfaceDebugSummary(ctx, run.InterfaceID, status, responseResult.DurationMS)
	events, _ := s.repo.ListDebugEvents(ctx, taskID, 0)
	sequence := len(events) + 1
	message := "调试执行完成"
	eventType := "task.completed"
	if status == "failed" {
		message, eventType = "调试执行失败", "task.failed"
	} else if status == "canceled" {
		message, eventType = "调试任务已取消", "task.canceled"
	}
	return s.repo.CreateDebugEvent(ctx, model.APIDebugEvent{
		TaskID: taskID, Sequence: sequence, Type: eventType, Stage: "terminal",
		Status: status, Message: message, Progress: 100, Data: json.RawMessage(`{}`),
	})
}

func (s *APIAutomationService) CancelDebug(ctx context.Context, userID int64, taskID string) error {
	run, err := s.GetDebug(ctx, userID, taskID)
	if err != nil {
		return err
	}
	if contains([]string{"success", "failed", "canceled"}, run.Status) {
		return errors.New("调试任务已结束")
	}
	executor, err := s.executorRepo.GetByID(ctx, run.ExecutorID)
	if err != nil {
		return errors.New("执行器不存在")
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(executor.Endpoint, "/")+"/tasks/"+taskID+"/cancel", nil)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return errors.New("取消执行器任务失败")
	}
	response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("执行器拒绝取消任务")
	}
	if err := s.repo.UpdateDebugRun(ctx, taskID, "canceled", json.RawMessage(`{}`), "用户取消"); err != nil {
		return err
	}
	events, _ := s.repo.ListDebugEvents(ctx, taskID, 0)
	return s.repo.CreateDebugEvent(ctx, model.APIDebugEvent{
		TaskID: taskID, Sequence: len(events) + 1, Type: "task.canceled", Stage: "terminal",
		Status: "canceled", Message: "调试任务已取消", Progress: 100, Data: json.RawMessage(`{}`),
	})
}

func (s *APIAutomationService) AppendDebugEvent(ctx context.Context, event model.APIDebugEvent) error {
	run, err := s.repo.GetDebugRun(ctx, event.TaskID)
	if err != nil || contains([]string{"success", "failed", "canceled"}, run.Status) {
		return errors.New("调试任务不存在或已结束")
	}
	if event.Sequence <= 1 {
		event.Sequence = int(time.Now().UnixNano() % 1000000000)
	}
	if len(event.Data) == 0 {
		event.Data = json.RawMessage(`{}`)
	}
	return s.repo.CreateDebugEvent(ctx, event)
}

func (s *APIAutomationService) pickAPIExecutor(ctx context.Context) (model.ExecutorView, error) {
	items, err := s.executorRepo.List(ctx)
	if err != nil {
		return model.ExecutorView{}, errors.New("读取执行器状态失败")
	}
	for _, item := range items {
		if item.Status != "online" || time.Since(item.LastHeartbeatAt) > 30*time.Second {
			continue
		}
		for _, supported := range item.SupportedTypes {
			if supported == "api" {
				return item, nil
			}
		}
	}
	return model.ExecutorView{}, errors.New("没有在线且支持 API 的执行器")
}

func submitAPIDebugTask(ctx context.Context, endpoint, taskID string, payload map[string]any, callbackURL, eventURL string) error {
	body, _ := json.Marshal(map[string]any{
		"taskId": taskID, "type": "api", "payload": payload,
		"callbackUrl": callbackURL, "eventUrl": eventURL,
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/tasks", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("执行器返回状态码 %d", response.StatusCode)
	}
	return nil
}

func maskDebugRequest(raw json.RawMessage) json.RawMessage {
	var request map[string]any
	if json.Unmarshal(raw, &request) != nil {
		return raw
	}
	if headers, ok := request["headers"].(map[string]any); ok {
		masked := map[string]string{}
		for key, value := range headers {
			masked[key] = fmt.Sprint(value)
		}
		request["headers"] = maskPreviewHeaders(masked)
	}
	result, _ := json.Marshal(request)
	return result
}
