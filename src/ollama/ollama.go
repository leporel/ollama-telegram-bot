package ollama

import (
	"encoding/json"
	"time"
)

type ChatRequest struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Tools    []Tool         `json:"tools,omitempty"`
	AdvancedParams
}

type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	Images    []Image    `json:"images,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type Image struct {
	URL     string `json:"url"`
	Caption string `json:"caption,omitempty"`
}

type ToolCall struct {
	ID       string   `json:"id"`
	Function Function `json:"function"`
}

type Function struct {
	Name string `json:"name"`
	Args Args   `json:"arguments"`
}

type Args map[string]interface{}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type AdvancedParams struct {
	Format    string  `json:"format,omitempty"`
	Options   Options `json:"options,omitempty"`
	Stream    bool    `json:"stream"`
	KeepAlive string  `json:"keep_alive,omitempty"`
}

type Options struct {
	Mirostat       int     `json:"mirostat,omitempty"`
	MirostatEta    float64 `json:"mirostat_eta,omitempty"`
	MirostatTau    float64 `json:"mirostat_tau,omitempty"`
	NumCtx         int     `json:"num_ctx,omitempty"`
	RepeatLastN    int     `json:"repeat_last_n,omitempty"`
	RepeatPenalty  float64 `json:"repeat_penalty,omitempty"`
	Temperature    float64 `json:"temperature,omitempty"`
	Seed           int     `json:"seed,omitempty"`
	Stop           string  `json:"stop,omitempty"`
	TfsZ           float64 `json:"tfs_z,omitempty"`
	NumPredict     int     `json:"num_predict,omitempty"`
	TopK           int     `json:"top_k,omitempty"`
	TopP           float64 `json:"top_p,omitempty"`
	MinP           float64 `json:"min_p,omitempty"`
}

// --------------------------------------------

type ChatResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Message            Message   `json:"message"`
	Done               bool      `json:"done"`
	TotalDuration      int64     `json:"total_duration"`
	LoadDuration       int64     `json:"load_duration"`
	PromptEvalCount    int       `json:"prompt_eval_count"`
	PromptEvalDuration int64     `json:"prompt_eval_duration"`
	EvalCount          int       `json:"eval_count"`
	EvalDuration       int64     `json:"eval_duration"`
}


// --------------------------------------------


func MakeMessage(role, content string) Message {
	return Message{
		Role:    role,
		Content: content,
	}
}