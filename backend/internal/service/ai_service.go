package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/repository"
)

const maxToolCallRounds = 5

type AIService struct {
	repo   *repository.AIRepository
	tools  *AIToolExecutor
	client *http.Client
}

func NewAIService(repo *repository.AIRepository, tools *AIToolExecutor) *AIService {
	return &AIService{
		repo:  repo,
		tools: tools,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
			},
		},
	}
}

// Load reads the current config for a specific user.
func (s *AIService) Load(ctx context.Context, userID int64) (*model.AIConfig, error) {
	cfg, err := s.repo.GetAIConfig(ctx, userID)
	if err != nil {
		return model.DefaultAIConfig(), nil
	}
	if cfg.Security.MaxTokensPerRequest == 0 {
		cfg.Security = model.DefaultAIConfig().Security
	}
	return cfg, nil
}

// SaveModel upserts a model configuration by ID.
func (s *AIService) SaveModel(ctx context.Context, userID int64, m model.AIModel) (*model.AIConfig, error) {
	cfg, err := s.repo.GetAIConfig(ctx, userID)
	if err != nil {
		return nil, err
	}
	found := false
	for i := range cfg.Models {
		if cfg.Models[i].ID == m.ID {
			cfg.Models[i] = m
			found = true
			break
		}
	}
	if !found {
		cfg.Models = append(cfg.Models, m)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveAIConfig(ctx, userID, string(b)); err != nil {
		return nil, err
	}
	return cfg, nil
}

// DeleteModel removes a model by ID.
func (s *AIService) DeleteModel(ctx context.Context, userID int64, modelID string) (*model.AIConfig, error) {
	cfg, err := s.repo.GetAIConfig(ctx, userID)
	if err != nil {
		return nil, err
	}
	filtered := make([]model.AIModel, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		if m.ID != modelID {
			filtered = append(filtered, m)
		}
	}
	cfg.Models = filtered
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveAIConfig(ctx, userID, string(b)); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SavePreferences updates the AI preferences for a user.
func (s *AIService) SavePreferences(ctx context.Context, userID int64, prefs model.AIPreferences) (*model.AIConfig, error) {
	cfg, err := s.repo.GetAIConfig(ctx, userID)
	if err != nil {
		return nil, err
	}
	cfg.Preferences = prefs
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveAIConfig(ctx, userID, string(b)); err != nil {
		return nil, err
	}
	return cfg, nil
}

// TestConnection sends a lightweight request to validate provider credentials.
func (s *AIService) TestConnection(ctx context.Context, req model.AIConnectionTestRequest) *model.AIConnectionTestResult {
	start := time.Now()
	baseURL := req.BaseURL
	if baseURL == "" {
		switch req.Provider {
		case "openai":
			baseURL = "https://api.openai.com/v1"
		case "anthropic":
			baseURL = "https://api.anthropic.com"
		case "gemini":
			baseURL = "https://generativelanguage.googleapis.com"
		case "openrouter":
			baseURL = "https://openrouter.ai/api/v1"
		case "deepseek":
			baseURL = "https://api.deepseek.com"
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")

	var testURL string
	var headers map[string]string

	switch req.Provider {
	case "openai", "openrouter", "deepseek":
		testURL = baseURL + "/models"
		headers = map[string]string{
			"Authorization": "Bearer " + req.APIKey,
		}
	case "anthropic":
		testURL = baseURL + "/v1/messages"
		headers = map[string]string{
			"x-api-key":         req.APIKey,
			"anthropic-version": "2023-06-01",
		}
	case "gemini":
		testURL = baseURL + "/v1beta/models?key=" + req.APIKey
		headers = map[string]string{}
	default:
		return &model.AIConnectionTestResult{Success: false, Message: fmt.Sprintf("不支持的 Provider: %s", req.Provider)}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return &model.AIConnectionTestResult{Success: false, Message: fmt.Sprintf("请求创建失败: %v", err)}
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := s.client.Do(httpReq)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &model.AIConnectionTestResult{Success: false, Message: fmt.Sprintf("连接失败: %v", err), Latency: latency}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &model.AIConnectionTestResult{Success: true, Message: "连接成功", Latency: latency}
	}

	body := make([]byte, 200)
	resp.Body.Read(body)
	return &model.AIConnectionTestResult{
		Success: false,
		Message: fmt.Sprintf("HTTP %d — %s", resp.StatusCode, string(body)),
		Latency: latency,
	}
}

// SSEEvent represents an SSE event sent to the frontend.
// Type can be: "model", "tool_call", "tool_result", "content", "done", "error".
type SSEEvent struct {
	Type     string `json:"type,omitempty"`
	Content  string `json:"content,omitempty"`
	Done     bool   `json:"done,omitempty"`
	Error    string `json:"error,omitempty"`
	Model    string `json:"model,omitempty"`
	Tool     string `json:"tool,omitempty"`
	ToolArgs string `json:"toolArgs,omitempty"`
	Result   string `json:"result,omitempty"`
	Success  *bool  `json:"success,omitempty"`
}

// chatMessage is used internally for the OpenAI-compatible messages array.
type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // LLM returns this as a JSON-encoded string
}

// ChatWithTools handles the full tool-calling chat loop and emits events to the channel.
// The caller should close the channel after this function returns.
func (s *AIService) ChatWithTools(ctx context.Context, userID int64, req model.AIChatRequest, ch chan<- SSEEvent) {
	defer close(ch)

	cfg, modelCfg, err := s.prepareChat(ctx, userID)
	if err != nil {
		ch <- SSEEvent{Type: "error", Error: err.Error()}
		return
	}

	// Send model info
	ch <- SSEEvent{Type: "model", Model: modelCfg.Label}

	messages := s.buildMessages(req.Messages)
	tools := s.tools.ToolDefinitions()
	baseURL := s.resolveBaseURL(modelCfg)

	// Tool calling loop
	for round := 0; round < maxToolCallRounds; round++ {
		resp, err := s.callLLMNonStream(ctx, baseURL, modelCfg, messages, tools, cfg.Security.MaxTokensPerRequest)
		if err != nil {
			ch <- SSEEvent{Type: "error", Error: err.Error()}
			return
		}

		choice := resp.Choices[0]

		// Check for tool calls
		if len(choice.Message.ToolCalls) > 0 {
			// Add assistant message with tool calls
			messages = append(messages, chatMessage{
				Role:      "assistant",
				ToolCalls: choice.Message.ToolCalls,
			})

			for _, tc := range choice.Message.ToolCalls {
				toolName := tc.Function.Name
				toolArgs := tc.Function.Arguments

				// Notify frontend
				ch <- SSEEvent{
					Type:     "tool_call",
					Tool:     toolName,
					ToolArgs: toolArgs,
				}

				// Execute the tool
				result, execErr := s.tools.Execute(ctx, userID, toolName, json.RawMessage(toolArgs))
				success := execErr == nil
				resultText := result
				if execErr != nil {
					resultText = execErr.Error()
				}

				// Notify result
				ch <- SSEEvent{
					Type:    "tool_result",
					Tool:    toolName,
					Result:  resultText,
					Success: &success,
				}

				// Add tool result to messages
				messages = append(messages, chatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    resultText,
				})
			}
			continue
		}

		// No tool calls — stream the final text response
		if choice.Message.Content != "" {
			// LLM already returned full content (non-streaming)
			ch <- SSEEvent{Type: "content", Content: choice.Message.Content}
			ch <- SSEEvent{Type: "done", Done: true}
			return
		}

		// Empty response — stream a follow-up call
		s.streamFinalResponse(ctx, baseURL, modelCfg, messages, cfg.Security.MaxTokensPerRequest, ch)
		return
	}

	// Max rounds exceeded — do final streaming
	s.streamFinalResponse(ctx, baseURL, modelCfg, messages, cfg.Security.MaxTokensPerRequest, ch)
}

// prepareChat loads config and finds the active copilot model.
func (s *AIService) prepareChat(ctx context.Context, userID int64) (*model.AIConfig, *model.AIModel, error) {
	cfg, err := s.Load(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	modelID := cfg.Preferences.Copilot
	if modelID == "" {
		return nil, nil, fmt.Errorf("未配置默认 AI 模型，请先在 AI 设置中新建模型并设为默认")
	}

	for i := range cfg.Models {
		if cfg.Models[i].ID == modelID && cfg.Models[i].Enabled && cfg.Models[i].APIKey != "" {
			return cfg, &cfg.Models[i], nil
		}
	}

	return nil, nil, fmt.Errorf("模型 %s 未启用或未配置 API Key", modelID)
}

// resolveBaseURL returns the base URL for the model, falling back to defaults.
func (s *AIService) resolveBaseURL(m *model.AIModel) string {
	baseURL := strings.TrimRight(m.BaseURL, "/")
	if baseURL == "" {
		switch m.Provider {
		case "openai":
			baseURL = "https://api.openai.com/v1"
		case "deepseek":
			baseURL = "https://api.deepseek.com"
		case "openrouter":
			baseURL = "https://openrouter.ai/api/v1"
		default:
			baseURL = "https://api.openai.com/v1"
		}
	}
	return baseURL
}

// buildMessages converts the request messages to internal format with a system prompt.
func (s *AIService) buildMessages(reqMsgs []model.AIChatMessage) []chatMessage {
	messages := []chatMessage{
		{
			Role:    "system",
			Content: "你是 Synapse QA 测试平台的 AI 助手。你可以帮助用户管理接口、分析测试数据、解答问题。当用户要求操作系统时（如创建接口、查询接口等），请调用对应的工具函数完成操作，然后向用户总结执行结果。",
		},
	}
	for _, msg := range reqMsgs {
		messages = append(messages, chatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	return messages
}

// callLLMNonStream makes a non-streaming request to detect tool calls.
func (s *AIService) callLLMNonStream(ctx context.Context, baseURL string, m *model.AIModel, messages []chatMessage, tools []map[string]any, maxTokens int) (*chatCompletionResponse, error) {
	bodyMap := map[string]any{
		"model":       m.Name,
		"messages":    messages,
		"stream":      false,
		"max_tokens":  maxTokens,
		"temperature": 0.7,
	}
	if len(tools) > 0 {
		bodyMap["tools"] = tools
	}

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}

	chatURL := baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", chatURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求 AI 服务失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf("AI 服务返回错误 HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	var result chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 AI 响应失败: %v", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("AI 返回了空响应")
	}
	return &result, nil
}

// streamFinalResponse makes a streaming request and emits content/done events.
func (s *AIService) streamFinalResponse(ctx context.Context, baseURL string, m *model.AIModel, messages []chatMessage, maxTokens int, ch chan<- SSEEvent) {
	bodyMap := map[string]any{
		"model":       m.Name,
		"messages":    messages,
		"stream":      true,
		"max_tokens":  maxTokens,
		"temperature": 0.7,
	}

	body, err := json.Marshal(bodyMap)
	if err != nil {
		ch <- SSEEvent{Type: "error", Error: err.Error()}
		return
	}

	chatURL := baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", chatURL, bytes.NewReader(body))
	if err != nil {
		ch <- SSEEvent{Type: "error", Error: err.Error()}
		return
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	streamClient := &http.Client{Timeout: 0, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: false}}}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		ch <- SSEEvent{Type: "error", Error: fmt.Sprintf("请求 AI 服务失败: %v", err)}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		ch <- SSEEvent{Type: "error", Error: fmt.Sprintf("AI 服务返回错误 HTTP %d: %s", resp.StatusCode, string(errBody))}
		return
	}

	parseOpenAIStream(resp.Body, ch)
}

// chatCompletionResponse is the OpenAI-compatible non-streaming response.
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []toolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

// parseOpenAIStream reads SSE lines from the LLM stream and sends parsed events.
func parseOpenAIStream(body io.ReadCloser, ch chan<- SSEEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), 32*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- SSEEvent{Type: "done", Done: true}
			return
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				ch <- SSEEvent{Type: "content", Content: c.Delta.Content}
			}
			if c.FinishReason != nil {
				ch <- SSEEvent{Type: "done", Done: true}
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- SSEEvent{Type: "error", Error: fmt.Sprintf("流读取错误: %v", err)}
	}
}
