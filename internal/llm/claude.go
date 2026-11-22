// Package llm 最简 LLM 客户端：只依赖标准库，直接调用 Anthropic Messages API。
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

type Client struct {
	apiKey string
	model  string
	http   *http.Client
}

type Turn struct {
	Role    string `json:"role"` // user | assistant
	Content string `json:"content"`
}

func New(apiKey, model string) *Client {
	return &Client{apiKey: apiKey, model: model, http: &http.Client{Timeout: 90 * time.Second}}
}

func (c *Client) Enabled() bool { return c.apiKey != "" }

type request struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	System    string `json:"system,omitempty"`
	Messages  []Turn `json:"messages"`
}

type response struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete 发送对话并返回模型文本。turns 必须以 user 开头。
func (c *Client) Complete(ctx context.Context, system string, turns []Turn, maxTokens int) (string, error) {
	body, err := json.Marshal(request{Model: c.model, MaxTokens: maxTokens, System: system, Messages: turns})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var out response
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode response (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := string(raw)
		if out.Error != nil {
			msg = out.Error.Message
		}
		return "", fmt.Errorf("llm HTTP %d: %s", resp.StatusCode, msg)
	}
	for _, blk := range out.Content {
		if blk.Type == "text" {
			return blk.Text, nil
		}
	}
	return "", fmt.Errorf("llm returned no text")
}
