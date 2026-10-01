package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAI provider — chat completions OpenAI-compatible via HTTP murni
// (tanpa SDK). OPENAI_BASE_URL memungkinkan provider lain yang kompatibel
// tanpa refactor. Timeout ketat (8 s default) — gagal = pesan kecil, bukan
// error layar.
type OpenAI struct {
	baseURL string
	apiKey  string
	model   string
	timeout time.Duration
	client  *http.Client
}

func NewOpenAI(baseURL, apiKey, model string, timeout time.Duration) *OpenAI {
	return &OpenAI{
		baseURL: baseURL, apiKey: apiKey, model: model, timeout: timeout,
		client: &http.Client{Timeout: timeout},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (o *OpenAI) Complete(ctx context.Context, req Request) (*Response, error) {
	body := chatRequest{
		Model:       o.model,
		Messages:    []chatMessage{{Role: "system", Content: req.System}, {Role: "user", Content: req.User}},
		Temperature: 0.2, // operasi — jawaban konsisten, bukan kreativitas
	}
	if req.JSONMode {
		body.ResponseFormat = &struct {
			Type string `json:"type"`
		}{Type: "json_object"}
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("buat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	start := time.Now()
	httpResp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm unreachable: %w", err)
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("baca respons llm: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm status %d: %s", httpResp.StatusCode, truncate(string(raw), 200))
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return nil, fmt.Errorf("parse respons llm: %w", err)
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("llm error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 || cr.Choices[0].Message.Content == "" {
		return nil, fmt.Errorf("llm respons tanpa konten")
	}
	return &Response{Text: cr.Choices[0].Message.Content, Model: cr.Model, Duration: time.Since(start)}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
