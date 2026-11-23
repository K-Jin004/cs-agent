# cs-agent — Foreign-Trade Customer Service Draft Assistant

A local assistant that drafts email replies for foreign-trade sales reps.

Customer message → stored → AI draft → sales rep reviews / edits → sent.

The core rule: **the AI never invents commercial numbers.** Prices, MOQ, lead time, payment terms, warranty and other commercial terms always appear as placeholders the rep must fill in:

    [需业务员确认：unit price for 2000 pcs (history 2026-09-10: USD 8.5/pc)]

Product facts (wattage, beam angle, certifications, …) found in the knowledge base or past replies are stated directly, word for word.

> Runs locally only. The server binds to `127.0.0.1` and has no login — do not expose it to the internet.

## Running

    cd cs-agent
    docker compose up -d                  # Postgres + pgvector on port 5433
    go mod tidy                           # first run: fetch dependencies
    ollama pull bge-m3                    # embedding model (memory + knowledge base)
    ollama list                           # find your chat model name
    export LLM_MODEL=<model name>         # e.g. qwen3:4b-instruct-2507-q4_K_M
    go run ./cmd/server                   # http://127.0.0.1:8090

To use the Claude API instead of Ollama:

    LLM_PROVIDER=claude ANTHROPIC_API_KEY=sk-ant-... go run ./cmd/server

### Environment variables

| Variable | Default | Notes |
|---|---|---|
| `LLM_PROVIDER` | `ollama` | `ollama` or `claude` |
| `LLM_MODEL` | — (required for Ollama); `claude-sonnet-5-5` for Claude | chat model |
| `EMBED_MODEL` | `bge-m3` | Ollama embedding model (1024-dim) |
| `OLLAMA_URL` | `http://127.0.0.1:11434` | |
| `ANTHROPIC_API_KEY` | — | required when `LLM_PROVIDER=claude` |
| `DATABASE_URL` | `postgres://cs:cs@localhost:5433/cs?sslmode=disable` | |
| `ADDR` | `127.0.0.1:8090` | |

Database migrations in `internal/store/migrations/` run automatically on startup.

## Quick tour

1. Create a conversation (customer name, email, language).
2. Use **模拟客户来信** (simulate customer message) at the bottom, e.g.
   *"Hi, we are interested in your LED panel lights. Could you send us the price for 2000 pcs and the lead time?"*
3. The AI draft should turn price and lead time into `[需业务员确认：…]` placeholders rather than making up numbers.
4. Fill in the placeholders, optionally click **按填好的内容润色** (polish with filled-in values), then **审核并发送** (review and send).
5. No new customer message? Use **AI 帮我写** (AI write for me): type an instruction such as `follow up`, `yes`, or a rough draft, and the AI writes the email from the conversation context in your tone.

## Features

**Reply drafts** — Built from the last 20 messages of the conversation plus retrieved references, in this priority order:
1. **History with this customer** — past replies the rep actually sent to this customer.
2. **Similar Q&A (global memory)** — how we answered the same kind of question for *other* customers. Their prices, percentages and names are masked in code before the model sees them, and their commercial terms may only appear inside placeholders.
3. **Knowledge base** — company reference material, used as a supporting check.

When past replies and the knowledge base disagree, the draft writes a placeholder listing both versions.

**Memory** — Turn on **记住这次对话** (remember this conversation) and every reply the rep sends is stored as a customer-question + reply pair, embedded with bge-m3 into pgvector. The Memory page lets you remember past conversations, filter by customer, and delete entries.

**Knowledge base** — Upload `.txt` / `.md` or paste text. Documents are split heading-aware (800 chars, 100 overlap) and retrieved with hybrid search: dense vectors + Postgres full-text, fused with RRF. Includes a search-test panel.

**Polish** — After the rep fills in the placeholders, the model rewrites the draft into fluent text. Code verifies the set of numbers is unchanged (up to 2 attempts), and polish is refused while placeholders remain. One click undoes it.

**AI compose** — Drafts from a rep instruction even without a new customer message. The instruction overrides everything else; short answers like "yes" are treated as the answer. Follow-ups do not repeat the previous email. Tone examples come only from replies the rep substantially edited.

**Safety checks (code, not prompt)** — Small models don't always follow instructions, so drafts are checked deterministically and the rep sees warnings for:
- prices or discounts not from the rep's own instruction or sent messages (internal KB price lists are masked in compose);
- numbers with no source;
- commitments made on the rep's behalf ("price remains valid", "guarantee");
- denials ("we don't have …") not backed by the knowledge base;
- drafts that contradict an affirmative instruction (retried once automatically);
- commercial terms copied from other customers (rewritten into placeholders).

Citation markers, signature placeholders and template tokens are stripped automatically.

**Edit tracking** — On send, the original AI draft and the rep's edit ratio (word-level LCS) are stored for tone examples and future evaluation.

## Code layout

    cmd/server          entry point + embedded single-page frontend
    internal/store      Postgres access + migration SQL
    internal/llm        Ollama / Claude chat clients, Ollama embeddings (stdlib only)
    internal/api        HTTP API: drafts, compose, memory, knowledge base, polish, safety checks
    internal/rag        knowledge base chunking + RRF fusion

## Tests

    go test ./internal/...

## Known limitations

- Chinese keyword search is weak (Postgres `simple` parser); dense retrieval still works.
- Safety checks cover English and Chinese only.
- Only `.txt` / `.md` uploads; no PDF yet.
- Non-price terms like warranty in the knowledge base can still be stated directly by AI compose.
