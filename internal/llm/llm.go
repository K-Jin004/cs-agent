package llm

import "context"

// Completer 是 LLM 提供方的统一接口：Claude API、本地 Ollama 都实现它。
type Completer interface {
	Enabled() bool
	// Complete 发送对话并返回模型文本。turns 必须以 user 开头。
	Complete(ctx context.Context, system string, turns []Turn, maxTokens int) (string, error)
}
