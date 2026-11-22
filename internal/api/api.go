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
	mux.HandleFunc("POST /api/conversations/{id}/remember", s.setRemember)
	mux.HandleFunc("GET /api/conversations/{id}/memories", s.listMemories)
	mux.HandleFunc("DELETE /api/memories/{id}", s.deleteMemory)
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
	draft, mems, derr := s.generateDraft(r.Context(), id)
	if derr != nil {
		log.Printf("draft failed (conv %d): %v", id, derr)
		resp["draft_error"] = derr.Error()
	} else {
		resp["draft"] = draft
		resp["memories"] = mems
	}
	writeJSON(w, 201, resp)
}

func (s *Server) regenerateDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	draft, mems, err := s.generateDraft(r.Context(), id)
	if err != nil {
		fail(w, 502, err)
		return
	}
	writeJSON(w, 200, map[string]any{"draft": draft, "memories": mems})
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

const systemPrompt = `你是外贸业务员的回复助手。根据下面的对话，为业务员起草对客户最新来信的回复。

规则：
1. 使用客户最新来信的语言回复，语气专业、简洁、礼貌。
2. 你不知道任何价格、折扣、最小起订量(MOQ)、交期、付款条款、运费、认证资质、产品参数。绝对不要写出任何具体数字或承诺。
3. 客户问到的每一项上述信息，都必须在回复里逐项列出，并在对应位置原样插入占位符，格式固定为：
   [需业务员确认：<该项内容>]
   占位符本身保持中文，不要翻译；不要用"我们会确认后回复"之类的笼统说法代替占位符。
4. 只回答客户在来信中明确问到的项目。客户没有问的项目（例如没问 MOQ、认证、运费）一律不要列出。
5. 只输出回复正文，不要加解释、标题或签名。

示例：
客户来信：Can you quote 500 pcs of model X and tell me the MOQ?
回复：
Thank you for your inquiry about model X.
- Unit price for 500 pcs: [需业务员确认：500 pcs 单价]
- MOQ: [需业务员确认：最小起订量]
Please let us know if you need any other details.`

const maxHistory = 20

var errNoCustomerMsg = errors.New("no customer message to reply to")

func (s *Server) generateDraft(ctx context.Context, convID int64) (*store.Message, []store.Memory, error) {
	if !s.llm.Enabled() {
		return nil, nil, errors.New("LLM is not configured (set LLM_MODEL for Ollama, or LLM_PROVIDER=claude with ANTHROPIC_API_KEY)")
	}
	msgs, err := s.store.ListMessages(ctx, convID)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, errNoCustomerMsg
	}

	// 召回查询 = 上一条业务员回复之后客户发来的全部消息（客户可能连发多条）
	var pending []string
	for i := len(turns) - 1; i >= 0 && turns[i].Role == "user"; i-- {
		pending = append([]string{turns[i].Content}, pending...)
	}
	mems := s.recall(ctx, convID, strings.Join(pending, "\n"))

	cctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	text, err := s.llm.Complete(cctx, systemPrompt+memoryBlock(mems), turns, 1024)
	if err != nil {
		return nil, nil, err
	}
	if err := s.store.DiscardDrafts(ctx, convID); err != nil {
		return nil, nil, err
	}
	draft, err := s.store.AddMessage(ctx, convID, "ai_draft", strings.TrimSpace(text), "draft")
	return draft, mems, err
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
