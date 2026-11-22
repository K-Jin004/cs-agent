package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"cs-agent/internal/store"
)

const (
	recallTopK    = 3
	recallMaxDist = 0.55 // 余弦距离阈值，超过视为不相关（bge-m3 经验值，需用真实数据再调）
)

// remember 把"这条业务员回复之前、上一条业务员回复之后的客户来信 + 这条回复"存成一条记忆。
// 只在会话打开了"记住这次对话"时生效，只记录业务员审核发出的内容。返回是否新存了一条。
func (s *Server) remember(ctx context.Context, convID, replyID int64) (bool, error) {
	conv, err := s.store.GetConversation(ctx, convID)
	if err != nil {
		return false, err
	}
	if !conv.Remember {
		return false, nil
	}
	if s.embed == nil || !s.embed.Enabled() {
		return false, errors.New("embedding model not configured (EMBED_MODEL)")
	}
	if done, err := s.store.HasMemory(ctx, replyID); err != nil || done {
		return false, err
	}
	msgs, err := s.store.ListMessages(ctx, convID)
	if err != nil {
		return false, err
	}
	idx := -1
	for i, m := range msgs {
		if m.ID == replyID {
			idx = i
			break
		}
	}
	if idx < 0 || msgs[idx].Sender != "agent" || msgs[idx].Status != "sent" {
		return false, nil
	}
	var customer []store.Message
	for i := idx - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Status != "sent" {
			continue
		}
		if m.Sender == "agent" {
			break
		}
		customer = append([]store.Message{m}, customer...)
	}

	reply := msgs[idx]
	var b strings.Builder
	sourceIDs := []int64{}
	for _, m := range customer {
		fmt.Fprintf(&b, "客户（%s）：%s\n", m.CreatedAt.Format("2006-01-02"), m.Content)
		sourceIDs = append(sourceIDs, m.ID)
	}
	fmt.Fprintf(&b, "业务员（%s）：%s", reply.CreatedAt.Format("2006-01-02"), reply.Content)
	sourceIDs = append(sourceIDs, reply.ID)
	content := b.String()

	ectx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	vecs, err := s.embed.Embed(ectx, []string{content})
	if err != nil {
		return false, fmt.Errorf("embed reply %d: %w", replyID, err)
	}
	if err := s.store.AddMemory(ctx, conv.ContactID, convID, replyID, sourceIDs, content, vecs[0]); err != nil {
		return false, fmt.Errorf("save reply %d: %w", replyID, err)
	}
	log.Printf("memory: saved reply %d (contact %d, %d source msgs)", replyID, conv.ContactID, len(sourceIDs))
	return true, nil
}

// ---------- 记忆接口 ----------

// setRemember POST /api/conversations/{id}/remember {"remember":true|false}
// 打开时立即把本会话已发出的回复补存为记忆（相当于 nexus-agent 的"记住一次对话"）。
func (s *Server) setRemember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Remember bool `json:"remember"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, 400, errors.New("remember (bool) is required"))
		return
	}
	if in.Remember && (s.embed == nil || !s.embed.Enabled()) {
		fail(w, 400, errors.New("embedding model not configured (EMBED_MODEL)"))
		return
	}
	if err := s.store.SetRemember(r.Context(), id, in.Remember); err != nil {
		fail(w, 404, err)
		return
	}
	saved := 0
	if in.Remember {
		msgs, err := s.store.ListMessages(r.Context(), id)
		if err != nil {
			fail(w, 500, err)
			return
		}
		for _, m := range msgs {
			if m.Sender != "agent" || m.Status != "sent" {
				continue
			}
			ok, err := s.remember(r.Context(), id, m.ID)
			if err != nil {
				fail(w, 502, err)
				return
			}
			if ok {
				saved++
			}
		}
	}
	writeJSON(w, 200, map[string]any{"remember": in.Remember, "saved": saved})
}

// listMemories GET /api/conversations/{id}/memories —— 该会话所属客户的全部记忆。
func (s *Server) listMemories(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	conv, err := s.store.GetConversation(r.Context(), id)
	if err != nil {
		fail(w, 404, err)
		return
	}
	mems, err := s.store.ListContactMemories(r.Context(), conv.ContactID)
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, mems)
}

// deleteMemory DELETE /api/memories/{id}
func (s *Server) deleteMemory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteMemory(r.Context(), id); err != nil {
		fail(w, 500, err)
		return
	}
	w.WriteHeader(204)
}

// recall 用客户最新来信检索【该客户】过去会话里的相关记忆。失败返回空，不阻塞草稿。
func (s *Server) recall(ctx context.Context, convID int64, query string) []store.Memory {
	if s.embed == nil || !s.embed.Enabled() || strings.TrimSpace(query) == "" {
		return nil
	}
	conv, err := s.store.GetConversation(ctx, convID)
	if err != nil {
		log.Printf("recall: get conversation %d: %v", convID, err)
		return nil
	}
	ectx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	vecs, err := s.embed.Embed(ectx, []string{query})
	if err != nil {
		log.Printf("recall: embed: %v", err)
		return nil
	}
	mems, err := s.store.SearchMemories(ctx, conv.ContactID, convID, vecs[0], recallTopK, recallMaxDist)
	if err != nil {
		log.Printf("recall: search: %v", err)
		return nil
	}
	return mems
}

// memoryBlock 把召回的记忆拼成 system prompt 的一段。
func memoryBlock(mems []store.Memory) string {
	if len(mems) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`

## 与该客户的历史往来（仅供参考）
以下是过去会话中，业务员实际发给该客户的回复，可用来保持前后一致。
使用规则：
- 客户问到的项目如果在历史里出现过具体数字或条款，必须写进该项占位符的括号里，注明日期，供业务员核对，格式：
  [需业务员确认：<该项内容>（历史 <日期>：<历史上的说法>）]
- 历史数字可能已过期，不能在占位符之外直接写出，也不能当成当前承诺。
- 历史里与客户本次问题无关的内容不要提。
示例：客户问"上次的价格还有效吗？"，历史里业务员 2026-09-10 报过 USD 8.5/pc，则写：
- Price: [需业务员确认：当前单价是否仍有效（历史 2026-09-10：USD 8.5/pc FOB Shenzhen）]
`)
	for i, m := range mems {
		fmt.Fprintf(&b, "\n[历史%d]\n%s\n", i+1, m.Content)
	}
	return b.String()
}
