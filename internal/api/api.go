// Package api HTTP 接口：会话、消息、AI 草稿。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cs-agent/internal/llm"
	"cs-agent/internal/store"
)

type Server struct {
	store *store.Store
	llm   llm.Completer
	embed llm.Embedder // 可为 nil：不启用对话记忆
}

func New(s *store.Store, l llm.Completer, e llm.Embedder) *Server {
	return &Server{store: s, llm: l, embed: e}
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/conversations", s.listConversations)
	mux.HandleFunc("POST /api/conversations", s.createConversation)
	mux.HandleFunc("GET /api/conversations/{id}/messages", s.listMessages)
	mux.HandleFunc("POST /api/conversations/{id}/messages", s.customerMessage)
	mux.HandleFunc("POST /api/conversations/{id}/draft", s.regenerateDraft)
	mux.HandleFunc("POST /api/messages/{id}/send", s.sendDraft)
	mux.HandleFunc("POST /api/messages/{id}/discard", s.discardDraft)
	mux.HandleFunc("POST /api/messages/{id}/polish", s.polishDraft)
	mux.HandleFunc("POST /api/conversations/{id}/remember", s.setRemember)
	mux.HandleFunc("GET /api/conversations/{id}/memories", s.listMemories)
	mux.HandleFunc("DELETE /api/memories/{id}", s.deleteMemory)
	mux.HandleFunc("GET /api/memories", s.listAllMemories)
	mux.HandleFunc("GET /api/kb/documents", s.listKBDocuments)
	mux.HandleFunc("POST /api/kb/documents", s.createKBDocument)
	mux.HandleFunc("GET /api/kb/documents/{id}", s.getKBDocument)
	mux.HandleFunc("DELETE /api/kb/documents/{id}", s.deleteKBDocument)
	mux.HandleFunc("POST /api/kb/search", s.kbSearchTest)
}

// ---------- 会话 ----------

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListConversations(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		fail(w, 400, errors.New("name is required"))
		return
	}
	if in.Language == "" {
		in.Language = "en"
	}
	c, err := s.store.CreateConversation(r.Context(), in.Name, in.Email, in.Language)
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 201, c)
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	msgs, err := s.store.ListMessages(r.Context(), id)
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, msgs)
}

// ---------- 客户来信 → 草稿 ----------

// customerMessage 模拟客户发来一条消息（后续由邮件/IM 渠道替换），并尝试生成草稿。
func (s *Server) customerMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Content) == "" {
		fail(w, 400, errors.New("content is required"))
		return
	}
	msg, err := s.store.AddMessage(r.Context(), id, "customer", in.Content, "sent")
	if err != nil {
		fail(w, 500, err)
		return
	}
	resp := map[string]any{"message": msg}
	res, derr := s.generateDraft(r.Context(), id)
	if derr != nil {
		log.Printf("draft failed (conv %d): %v", id, derr)
		resp["draft_error"] = derr.Error()
	} else {
		resp["draft"], resp["memories"], resp["kb"], resp["warnings"] = res.Draft, res.Memories, res.KB, res.Warnings
	}
	writeJSON(w, 201, resp)
}

func (s *Server) regenerateDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := s.generateDraft(r.Context(), id)
	if err != nil {
		fail(w, 502, err)
		return
	}
	writeJSON(w, 200, res)
}

// sendDraft 业务员审核（可改稿）后发送：草稿转为 agent 消息。
func (s *Server) sendDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Content) == "" {
		fail(w, 400, errors.New("content is required"))
		return
	}
	m, err := s.store.GetMessage(r.Context(), id)
	if err != nil {
		fail(w, 404, err)
		return
	}
	if m.Status != "draft" {
		fail(w, 409, errors.New("message is not a pending draft"))
		return
	}
	// 注意：这里是后续"改稿反馈"的采集点（原稿 m.Content vs 终稿 in.Content）。
	if err := s.store.UpdateMessage(r.Context(), id, "agent", in.Content, "sent"); err != nil {
		fail(w, 500, err)
		return
	}
	m.Sender, m.Content, m.Status = "agent", in.Content, "sent"
	// 会话打开了"记住这次对话"才入记忆；失败只记日志，不影响发送
	if _, err := s.remember(r.Context(), m.ConversationID, m.ID); err != nil {
		log.Printf("memory: conv %d reply %d: %v", m.ConversationID, m.ID, err)
	}
	writeJSON(w, 200, m)
}

func (s *Server) discardDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, err := s.store.GetMessage(r.Context(), id)
	if err != nil {
		fail(w, 404, err)
		return
	}
	if m.Status != "draft" {
		fail(w, 409, errors.New("message is not a pending draft"))
		return
	}
	if err := s.store.UpdateMessage(r.Context(), id, m.Sender, m.Content, "discarded"); err != nil {
		fail(w, 500, err)
		return
	}
	w.WriteHeader(204)
}

// ---------- 草稿生成 ----------

const systemPrompt = `你是外贸业务员的回复助手。为业务员起草对客户最新来信的回复。
用户消息里可能有三部分：
- 【知识库】公司的产品与公司资料，是最权威的依据；
- 【历史往来】业务员过去实际发给该客户的回复；
- 【客户最新来信】要回复的内容。

客户问到的信息分两类，处理方式不同：

A. 商务条款：价格、折扣、最小起订量(MOQ)、交期、付款条款、运费、报价有效期、保修承诺。
   这些会变，必须由业务员每次确认。一律写成占位符：
   [需业务员确认：<该项内容>]
   如果【知识库】或【历史往来】里出现过，把原话和出处写进占位符括号里作提示：
   [需业务员确认：<该项内容>（资料《<文档名>》：<原话>）] 或 [需业务员确认：<该项内容>（历史 <日期>：<原话>）]
   不能在占位符之外写出这些数字。

B. 产品事实：型号、规格、瓦数、尺寸、光效、发光角度、防护等级、材质、认证、功能、公司情况等。
   - 如果【知识库】或【历史往来】里明确写了，直接照原话写进回复，数字一个字都不要改，也不要补充原文没有的内容。
   - 两者说法冲突时，以【知识库】为准。
   - 都没有的，写成占位符 [需业务员确认：<该项内容>]，绝对不要猜。
   - 资料里没提到不等于没有：除非资料明确写了"没有/不支持"，绝不能说"我们没有某产品/不能做某事"。
     客户的说法和资料对不上时（例如单位不同、型号写法不同），写成占位符让业务员判断。

其他规则：
1. 使用客户最新来信的语言回复，语气专业、简洁、礼貌。
2. 只回答客户明确问到的项目，没问的不要列。
3. 占位符保持中文，不要翻译；不要用"我们会确认后回复"之类的笼统说法代替占位符。
4. 只输出回复正文，不要加解释、标题、出处标注或签名。

示例一（有资料）：
【知识库】
[资料1]《Model X 规格书》
Model X: available in black and white. Rated power 18W. Reference price USD 3.2/pc (2026 Q3).
【历史往来】
[历史1]
客户（2026-09-10）：Do you ship to Brazil?
业务员（2026-09-10）：Yes, we have shipped to Brazil many times.
【客户最新来信】
Which colors and power does model X have, and what's the price now?
回复：
Thank you for your message.
- Colors: model X is available in black and white.
- Power: rated power 18W.
- Price: [需业务员确认：model X 当前单价（资料《Model X 规格书》：USD 3.2/pc (2026 Q3)）]
Please let us know if you need any other details.

示例二（没有资料）：
【知识库】无
【历史往来】无
【客户最新来信】
Can you quote 500 pcs of model X and tell me its power?
回复：
Thank you for your inquiry about model X.
- Unit price for 500 pcs: [需业务员确认：500 pcs 单价]
- Power: [需业务员确认：model X 功率]
Please let us know if you need any other details.`

const maxHistory = 20

var errNoCustomerMsg = errors.New("no customer message to reply to")

// draftResult 草稿及其参考来源（供业务员核对）。
type draftResult struct {
	Draft    *store.Message `json:"draft"`
	Memories []store.Memory `json:"memories"`
	KB       []store.KBHit  `json:"kb"`
	Warnings []string       `json:"warnings"` // 需业务员特别核对的句子（如否认产品/能力）
}

func (s *Server) generateDraft(ctx context.Context, convID int64) (*draftResult, error) {
	if !s.llm.Enabled() {
		return nil, errors.New("LLM is not configured (set LLM_MODEL for Ollama, or LLM_PROVIDER=claude with ANTHROPIC_API_KEY)")
	}
	msgs, err := s.store.ListMessages(ctx, convID)
	if err != nil {
		return nil, err
	}
	var turns []llm.Turn
	for _, m := range msgs {
		if m.Status != "sent" {
			continue
		}
		role := "user"
		if m.Sender == "agent" {
			role = "assistant"
		}
		turns = append(turns, llm.Turn{Role: role, Content: m.Content})
	}
	if len(turns) > maxHistory {
		turns = turns[len(turns)-maxHistory:]
	}
	for len(turns) > 0 && turns[0].Role != "user" { // API 要求以 user 开头
		turns = turns[1:]
	}
	if len(turns) == 0 || turns[len(turns)-1].Role != "user" {
		return nil, errNoCustomerMsg
	}

	// 召回查询 = 上一条业务员回复之后客户发来的全部消息（客户可能连发多条）
	var pending []string
	for i := len(turns) - 1; i >= 0 && turns[i].Role == "user"; i-- {
		pending = append([]string{turns[i].Content}, pending...)
	}
	query := strings.Join(pending, "\n")
	mems := s.recall(ctx, convID, query)
	kb, err := s.searchKB(ctx, query)
	if err != nil { // 知识库是增强不是依赖：检索失败照常出草稿
		log.Printf("kb search (conv %d): %v", convID, err)
		kb = nil
	}

	cctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	// 参考资料紧挨着客户来信放在最后一条 user 消息里（小模型对 system 末尾的长文本关注度低）
	last := &turns[len(turns)-1]
	last.Content = referenceBlock(kb, mems) + "【客户最新来信】\n" + last.Content
	text, err := s.llm.Complete(cctx, systemPrompt, turns, 1024)
	if err != nil {
		return nil, err
	}
	if err := s.store.DiscardDrafts(ctx, convID); err != nil {
		return nil, err
	}
	draft, err := s.store.AddMessage(ctx, convID, "ai_draft", stripCitations(strings.TrimSpace(text)), "draft")
	if err != nil {
		return nil, err
	}
	if mems == nil {
		mems = []store.Memory{}
	}
	if kb == nil {
		kb = []store.KBHit{}
	}
	warnings := draftWarnings(draft.Content, kb)
	if warnings == nil {
		warnings = []string{}
	}
	return &draftResult{Draft: draft, Memories: mems, KB: kb, Warnings: warnings}, nil
}

// ---------- 工具函数 ----------

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, 400, errors.New("invalid id"))
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
