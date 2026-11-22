package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"os"
	"time"

	"cs-agent/internal/api"
	"cs-agent/internal/llm"
	"cs-agent/internal/store"
)

//go:embed web/index.html
var indexHTML []byte

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// newLLM 按 LLM_PROVIDER 选择模型提供方，默认本地 Ollama。
func newLLM() llm.Completer {
	switch env("LLM_PROVIDER", "ollama") {
	case "claude":
		c := llm.New(os.Getenv("ANTHROPIC_API_KEY"), env("LLM_MODEL", "claude-sonnet-5-5"))
		if !c.Enabled() {
			log.Println("WARN: LLM_PROVIDER=claude but ANTHROPIC_API_KEY not set, AI drafts disabled")
		}
		return c
	default:
		o := llm.NewOllama(env("OLLAMA_URL", "http://127.0.0.1:11434"), os.Getenv("LLM_MODEL"))
		if !o.Enabled() {
			log.Println("WARN: LLM_MODEL not set (run `ollama list` for a model name), AI drafts disabled; conversations still work")
		}
		return o
	}
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	st, err := store.New(ctx, env("DATABASE_URL", "postgres://cs:cs@localhost:5433/cs?sslmode=disable"))
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	mux := http.NewServeMux()
	emb := llm.NewOllamaEmbedder(env("OLLAMA_URL", "http://127.0.0.1:11434"), env("EMBED_MODEL", "bge-m3"))
	api.New(st, newLLM(), emb).Routes(mux)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})

	addr := env("ADDR", "127.0.0.1:8090")
	log.Printf("listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
