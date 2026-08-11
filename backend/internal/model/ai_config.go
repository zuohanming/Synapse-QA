package model

import "encoding/json"

// AIModel represents a user-configured LLM model entry.
type AIModel struct {
	ID       string `json:"id"`       // unique user-facing ID, e.g. "deepseek-v4-pro"
	Provider string `json:"provider"` // "openai" | "deepseek" | "anthropic" | "gemini" | "openrouter"
	Name     string `json:"name"`     // API model name
	Label    string `json:"label"`    // display label, optional
	APIKey   string `json:"apiKey"`
	BaseURL  string `json:"baseUrl"`
	Enabled  bool   `json:"enabled"`
}

// AIPreferences maps use-cases to model IDs.
type AIPreferences struct {
	Copilot    string `json:"copilot"`
	Assertions string `json:"assertions"`
	Analysis   string `json:"analysis"`
	Generation string `json:"generation"`
}

// AISecurity holds safety / cost controls.
type AISecurity struct {
	DataMasking         bool `json:"dataMasking"`
	MaxTokensPerRequest int  `json:"maxTokensPerRequest"`
	RateLimitPerMinute  int  `json:"rateLimitPerMinute"`
}

// AIConfig is the top-level AI configuration stored in platform_settings.
type AIConfig struct {
	Models      []AIModel     `json:"models"`
	Preferences AIPreferences `json:"preferences"`
	Security    AISecurity    `json:"security"`
}

// DefaultAIConfig returns an empty config with sensible security defaults.
func DefaultAIConfig() *AIConfig {
	return &AIConfig{
		Models:      []AIModel{},
		Preferences: AIPreferences{},
		Security: AISecurity{
			DataMasking:         true,
			MaxTokensPerRequest: 4096,
			RateLimitPerMinute:  30,
		},
	}
}

// AIConnectionTestRequest is used to test a single provider connection.
type AIConnectionTestRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey"`
	BaseURL  string `json:"baseUrl,omitempty"`
}

// AIConnectionTestResult is returned after testing a provider.
type AIConnectionTestResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Latency int64  `json:"latency,omitempty"` // ms
}

// AIChatRequest is the payload for the AI chat endpoint.
type AIChatRequest struct {
	Messages []AIChatMessage `json:"messages" binding:"required"`
}

// AIChatMessage represents a single turn in the conversation.
type AIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Marshal returns the JSON-encoded config.
func (c *AIConfig) Marshal() (json.RawMessage, error) {
	b, err := json.Marshal(c)
	return json.RawMessage(b), err
}
