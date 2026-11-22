package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Ollama 调用本机 Ollama 的原生 /api/chat 接口（非流式）。
type Ollama struct {
	baseURL string
	model   string
	http    *http.Client
}

func NewOllama(baseURL, model string) *Ollama {
	return &Ollama{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		http:    &http.Client{Timeout: 5 * time.Minute}, // 本地模型首次加载可能较慢
	}
}

func (o *Ollama) Enabled() bool { return o.model != "" }

type ollamaRequest struct {
	Model    string         `json:"model"`
	Messages []Turn         `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Error string `json:"error"`
}

func (o *Ollama) Complete(ctx context.Context, system string, turns []Turn, maxTokens int) (string, error) {
	msgs := make([]Turn, 0, len(turns)+1)
	if system != "" {
		msgs = append(msgs, Turn{Role: "system", Content: system})
	}
	msgs = append(msgs, turns...)

	body, err := json.Marshal(ollamaRequest{
		Model:    o.model,
		Messages: msgs,
		Stream:   false,
		Options:  map[string]any{"num_predict": maxTokens, "temperature": 0.3},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")

	resp, err := o.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach Ollama at %s (is it running? try `ollama serve`): %w", o.baseURL, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var out ollamaResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode Ollama response (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || out.Error != "" {
		return "", fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, out.Error)
	}
	if strings.TrimSpace(out.Message.Content) == "" {
		return "", fmt.Errorf("ollama returned empty content")
	}
	return out.Message.Content, nil
}
