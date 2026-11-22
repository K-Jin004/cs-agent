# cs-agent（外贸客服助手 · 里程碑 1：对话链路）

客户来信 → 存库 → AI 草稿 → 业务员审核/改稿 → 发送。

## 运行
    cd cs-agent
    docker compose up -d                  # Postgres+pgvector，端口 5433
    go mod tidy                           # 首次：下载依赖(pgx)，生成 go.sum
    ollama list                           # 查看本地模型名
    export LLM_MODEL=<模型名>              # 例如 ollama list 里的第一列
    go run ./cmd/server                   # http://127.0.0.1:8090

    # 改用 Claude API：LLM_PROVIDER=claude ANTHROPIC_API_KEY=sk-ant-... go run ./cmd/server

环境变量：LLM_MODEL（Ollama 必填）、EMBED_MODEL（对话记忆用的向量模型，默认 bge-m3，需 `ollama pull bge-m3`）、OLLAMA_URL（默认 http://127.0.0.1:11434）、LLM_PROVIDER（ollama|claude，默认 ollama）、DATABASE_URL、ADDR。

## 试用
1. 新建会话（填客户名和语言）
2. 在底部"模拟客户来信"输入，例如：
   "Hi, we are interested in your LED panel lights. Could you send us the price for 2000 pcs and the lead time?"
3. 看 AI 草稿：它应该把价格、交期写成 [需业务员确认：…] 占位符，而不是编造数字。
   这是第一个要观察的点——如果它编了数字，说明提示词需要加强。
4. 改稿后"审核并发送"。

## 代码结构
    cmd/server        入口 + 内嵌的单页前端
    internal/store    Postgres 访问 + 建表 SQL
    internal/llm      标准库实现的 Claude 调用
    internal/api      HTTP 接口 + 草稿生成、记忆、知识库、润色与安全检查
    internal/rag      知识库分块 + RRF 融合

## 两个参考模块
- 记忆：选定会话中业务员发出的问答 → bge-m3 向量，按客户隔离，只给同一客户的草稿做参考。
- 知识库：公司事实资料（.txt/.md 或粘贴）→ 分块 → 向量 + 关键词（Postgres 全文）→ RRF 融合，所有客户共用。
草稿里产品事实可直接引用资料原文；价格/交期/条款一律占位符，资料原话只作括号提示。
