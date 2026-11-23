package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"cs-agent/internal/api"
	"cs-agent/internal/llm"
	"cs-agent/internal/store"
)

// 前端：web/（React + Vite）构建到 cmd/server/web/dist，再打包进二进制。
// 改前端后在 web/ 下运行 npm run build。
//
//go:embed all:web/dist
var webDist embed.FS

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
	dist, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("web: %v", err)
	}
	mux.Handle("GET /", http.FileServerFS(dist))

	addr := env("ADDR", "127.0.0.1:8090")
	log.Printf("listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
