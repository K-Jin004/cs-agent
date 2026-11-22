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

// Embedder 把文本转成向量。
type Embedder interface {
	Enabled() bool
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// OllamaEmbedder 调用本机 Ollama 的 /api/embed（默认 bge-m3，1024 维，多语言）。
type OllamaEmbedder struct {
	baseURL string
	model   string
	http    *http.Client
}

func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		http:    &http.Client{Timeout: 2 * time.Minute},
	}
}

func (o *OllamaEmbedder) Enabled() bool { return o.model != "" }

func (o *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"model": o.model, "input": texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Ollama at %s: %w", o.baseURL, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
		Error      string      `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode Ollama embed response (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || out.Error != "" {
		return nil, fmt.Errorf("ollama embed HTTP %d: %s", resp.StatusCode, out.Error)
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed: got %d vectors for %d texts", len(out.Embeddings), len(texts))
	}
	return out.Embeddings, nil
}
