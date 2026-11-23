package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"cs-agent/internal/store"
)

const (
	recallTopK    = 3
	recallMaxDist = 0.55 // 余弦距离阈值，超过视为不相关（bge-m3 经验值，需用真实数据再调）
	styleTopK     = 3    // 润色时参考的"我过去的回答"条数
	styleMaxDist  = 0.55
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

// recall 起草用：只检索【该客户】过去会话里的问答，保持对这个客户说法前后一致。
// 其他客户的记忆不参与起草（那是别人的条件），只在润色时作为语气和回答方式的参考，见 styleRefs。
// 失败返回空，不阻塞草稿。
func (s *Server) recall(ctx context.Context, convID int64, query string) []store.Memory {
	vec, conv := s.embedQuery(ctx, convID, query)
	if vec == nil {
		return nil
	}
	mems, err := s.store.SearchMemories(ctx, conv.ContactID, convID, vec, recallTopK, recallMaxDist)
	if err != nil {
		log.Printf("recall: search: %v", err)
		return nil
	}
	return mems
}

// styleRefs 润色用：在【全部】记住的对话里找类似问题，看业务员当时是怎么回答的。
// 只用来学语气和回答方式；其他客户的金额、百分比和客户名先遮掉。
func (s *Server) styleRefs(ctx context.Context, convID int64, query string) []store.Memory {
	vec, conv := s.embedQuery(ctx, convID, query)
	if vec == nil {
		return nil
	}
	own, err := s.store.SearchMemories(ctx, conv.ContactID, convID, vec, styleTopK, styleMaxDist)
	if err != nil {
		log.Printf("style refs: search: %v", err)
	}
	others, err := s.store.SearchOtherMemories(ctx, conv.ContactID, vec, styleTopK, styleMaxDist)
	if err != nil {
		log.Printf("style refs: search others: %v", err)
	}
	for i := range others {
		others[i].Content = maskOtherCustomer(others[i].Content, others[i].ContactName)
	}
	all := append(own, others...)
	sort.Slice(all, func(i, j int) bool { return all[i].Distance < all[j].Distance })
	if len(all) > styleTopK {
		all = all[:styleTopK]
	}
	return all
}

func (s *Server) embedQuery(ctx context.Context, convID int64, query string) ([]float32, *store.Conversation) {
	if s.embed == nil || !s.embed.Enabled() || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	conv, err := s.store.GetConversation(ctx, convID)
	if err != nil {
		log.Printf("recall: get conversation %d: %v", convID, err)
		return nil, nil
	}
	ectx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	vecs, err := s.embed.Embed(ectx, []string{query})
	if err != nil {
		log.Printf("recall: embed: %v", err)
		return nil, nil
	}
	return vecs[0], conv
}

var percentRe = regexp.MustCompile(`\d+(?:\.\d+)?\s?%`)

// maskOtherCustomer 遮盖其他客户记忆里的价格、百分比条款和客户名字。
func maskOtherCustomer(content, contactName string) string {
	content = moneyRe.ReplaceAllString(content, "[该客户的价格，已隐藏]")
	content = percentRe.ReplaceAllString(content, "[已隐藏]")
	for _, n := range strings.Fields(strings.NewReplacer("(", " ", ")", " ", ",", " ").Replace(contactName)) {
		if len([]rune(n)) < 2 {
			continue
		}
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(n) + `\b`)
		content = re.ReplaceAllString(content, "[客户]")
	}
	return content
}

// referenceBlock 拼成放在客户来信前面的参考资料段：【知识库】+【历史往来】（只有该客户自己的）。
// 没有内容时明确写"无"，让模型知道没有可用的已知事实。
func referenceBlock(kb []store.KBHit, mems []store.Memory) string {
	var b strings.Builder
	if len(kb) == 0 {
		b.WriteString("【知识库】无\n")
	} else {
		b.WriteString("【知识库】\n")
		for i, h := range kb {
			fmt.Fprintf(&b, "[资料%d]《%s》\n%s\n\n", i+1, h.DocTitle, h.Content)
		}
	}
	if len(mems) == 0 {
		b.WriteString("【历史往来】无\n\n")
	} else {
		b.WriteString("【历史往来】（与该客户）\n")
		for i, m := range mems {
			fmt.Fprintf(&b, "[历史%d]\n%s\n\n", i+1, m.Content)
		}
	}
	return b.String()
}
