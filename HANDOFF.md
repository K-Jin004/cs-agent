# 项目交接说明（给 Claude Code）

## 项目目标
外贸客服助手：帮外贸业务员更高效、更准确地回复客户。学习/实验性质，Go 实现。

## 已定的设计决策
- 第一版只服务外贸业务员，工作方式为"半自动"：简单问题可自动回，复杂问题转人工。
  当前阶段先做**辅助模式**：AI 出草稿，业务员审核/改稿后发送。
- 渠道：先做独立 Web 界面，邮件/IM 以后再接。
- 技术：Go（标准库为主）+ PostgreSQL/pgvector（不用 Milvus，保持精简）。
- LLM：用户使用**本地 Ollama 模型**，不用 API。Claude API 仅作可选 (LLM_PROVIDER=claude)。
- 参考项目（已 clone 在上级目录，仅供读思路）：
  - ../nexus-agent  借鉴：短期记忆(窗口+摘要)、长期记忆(抽取→去重→召回)、RAG(稠密+关键词→RRF)
  - ../chatwoot     借鉴：联系人档案、会话状态、标签、预设回复、转人工分配

## 当前进度（里程碑 1：对话链路）
代码已写好但**从未编译或运行过**（写代码的环境没有 Go/Docker）。
链路：客户来信 → 存库 → AI 草稿 → 业务员改稿发送。

## 你现在要做
1. `docker compose up -d`（Postgres 在 5433 端口）
2. `go mod tidy && go vet ./...`，修复所有编译错误
3. `ollama list` 看本机模型，`export LLM_MODEL=<模型名>`，`go run ./cmd/server`
4. 访问 http://127.0.0.1:8090 ，新建会话，模拟客户询价：
   "Hi, we are interested in LED panel lights, can you send the price for 2000 pcs and the lead time?"
5. 验收：草稿必须把价格/交期写成 [需业务员确认：…] 占位符，不能编数字。
   若本地模型会编造数字，请加强 internal/api/api.go 里的 systemPrompt，并如实告诉用户模型不够可靠。

## 后续里程碑
2. RAG：文档上传→分块→pgvector+全文检索→RRF 融合，草稿带可核对的引用
3. 上下文管理：滑动窗口+摘要，客户档案固定注入
4. 长期记忆：按"客户"（不是员工）存储；检索阶段就要按 customer_id 过滤；
   记忆需记录来源消息，价格/数量/交期等关键字段须业务员确认后才入库
5. 风险路由：涉及价格/交期/条款/投诉/新客户的一律转人工
6. 评估：30–50 条带标准答案的测试询盘（改稿率、违规自动回复数）

## 注意
- 服务仅绑定 127.0.0.1，无登录鉴权，不要暴露到公网。
- 不要把 chatwoot/、nexus-agent/ 提交进 git；.env 含密钥也不要提交。
