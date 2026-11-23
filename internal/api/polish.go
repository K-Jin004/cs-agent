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
	"strconv"
	"strings"
	"time"

	"cs-agent/internal/llm"
	"cs-agent/internal/store"
)

// 业务员把占位符填好后，"润色"把条目式草稿改写成通顺的完整回复。
// 安全措施不靠模型自觉：
//   1. 还有未填的占位符 → 拒绝润色；
//   2. 用代码比对润色前后的数字集合，有缺失/改动/新增 → 自动重试一次，仍不一致则拒绝并返回差异。

const polishPrompt = `你是外贸邮件润色助手。【要润色的回复稿】是业务员已经确认过内容的回复，请把它改写成通顺、自然的邮件正文，并且写得像这位业务员本人。

用户消息里可能有【我过去的回答】：业务员以前遇到类似问题时实际发出的回复。用它学习这位业务员的写法：
称呼和开头结尾的习惯、句子长短、语气（正式还是随和）、回答问题的顺序和组织方式。
【我过去的回答】只学写法：里面的事实、数字、价格、条款、承诺、产品信息一律不能搬进这封邮件。
没有【我过去的回答】时，按专业、简洁的外贸邮件写。

规则：
1. 使用与原稿相同的语言。
2. 原稿里的每一个数字、单位、币种、型号、产品名、条款都必须原样保留，一个字符都不能改（例如 8.5 不能写成 8.50，2000 不能写成 2,000 或 two thousand）。
3. 不能增加原稿没有的任何信息、数字、承诺或产品细节；也不能删掉原稿里的任何信息。
4. 可以把条目改写成段落；项目较多时也可以保留条目，但语句要完整自然。
5. 不要加原稿没有的否定说法（例如"我们没有/不能"），也不要加原稿没有的条款（保修、免费、起订量、付款、交期等）。
6. 不要用 markdown 加粗，不要用 "I hope this message finds you well" 之类的套话。
7. 只输出改写后的正文，不要加解释、标题、出处或签名。`

var (
	placeholderRe = regexp.MustCompile(`\[需业务员确认[:：][^\]]*\]`)
	numberRe      = regexp.MustCompile(`\d+(?:[.,]\d+)*`)
	thousandsRe   = regexp.MustCompile(`^\d{1,3}(,\d{3})+(\.\d+)?$`)
)

// numbers 提取文本中的数字，规范化后计数（"2,000"="2000"，"8.50"="8.5"）。
func numbers(s string) map[string]int {
	out := map[string]int{}
	for _, raw := range numberRe.FindAllString(s, -1) {
		n := raw
		// 千分位逗号：1,000 / 12,500,000
		if thousandsRe.MatchString(n) {
			n = strings.ReplaceAll(n, ",", "")
		}
		n = strings.ReplaceAll(n, ",", ".") // 欧式小数 8,5
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			n = strconv.FormatFloat(f, 'f', -1, 64)
		}
		out[n]++
	}
	return out
}

// diffNumbers 返回原稿有而新稿缺的、新稿多出来的数字（按出现次数比较）。
func diffNumbers(before, after string) (missing, added []string) {
	b, a := numbers(before), numbers(after)
	for n, c := range b {
		if a[n] < c {
			missing = append(missing, n)
		}
	}
	for n, c := range a {
		if b[n] < c {
			added = append(added, n)
		}
	}
	sort.Strings(missing)
	sort.Strings(added)
	return
}

type polishError struct {
	Error        string   `json:"error"`
	Placeholders []string `json:"placeholders,omitempty"`
	Missing      []string `json:"missing,omitempty"`
	Added        []string `json:"added,omitempty"`
	AddedTerms   []string `json:"added_terms,omitempty"` // 润色稿多出的商务条款词
	AddedDenial  bool     `json:"added_denial,omitempty"`
	Candidate    string   `json:"candidate,omitempty"` // 未通过核对的润色结果，仅供查看
}

// polishDraft POST /api/messages/{id}/polish {"content": "业务员改好的草稿"}
// 只返回润色结果，不改数据库；业务员确认后照常"审核并发送"。
func (s *Server) polishDraft(w http.ResponseWriter, r *http.Request) {
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
	if !s.llm.Enabled() {
		fail(w, 400, errors.New("LLM is not configured"))
		return
	}
	if ph := placeholderRe.FindAllString(in.Content, -1); len(ph) > 0 {
		writeJSON(w, 422, polishError{
			Error:        fmt.Sprintf("还有 %d 处占位符未填写，请先填好再润色", len(ph)),
			Placeholders: ph,
		})
		return
	}

	// 参考"我过去的回答"：用这封回复针对的客户来信去检索全部记住的对话
	msgs, err := s.store.ListMessages(r.Context(), m.ConversationID)
	if err != nil {
		fail(w, 500, err)
		return
	}
	refs := s.styleRefs(r.Context(), m.ConversationID, polishQuery(msgs, in.Content))
	if refs == nil {
		refs = []store.Memory{}
	}

	var last polishError
	for attempt := 1; attempt <= 2; attempt++ {
		out, err := s.polishOnce(r.Context(), in.Content, refs)
		if err != nil {
			fail(w, 502, err)
			return
		}
		missing, added := diffNumbers(in.Content, out)
		leftover := placeholderRe.FindAllString(out, -1)
		terms := addedTerms(in.Content, out)
		denial := hasNegation(out) && !hasNegation(in.Content)
		if len(missing) == 0 && len(added) == 0 && len(leftover) == 0 && len(terms) == 0 && !denial {
			total := 0
			for _, c := range numbers(in.Content) {
				total += c
			}
			writeJSON(w, 200, map[string]any{"content": out, "numbers_checked": total, "attempts": attempt, "references": refs})
			return
		}
		log.Printf("polish msg %d attempt %d rejected: missing=%v added=%v terms=%v denial=%v", id, attempt, missing, added, terms, denial)
		last = polishError{
			Error:       "润色结果与原稿内容不一致，已拒绝替换，原稿保持不变",
			Missing:     missing,
			Added:       added,
			AddedTerms:  terms,
			AddedDenial: denial,
			Candidate:   out,
		}
	}
	writeJSON(w, 422, last)
}

// polishQuery 检索"我过去的回答"用的问题：上一条业务员回复之后客户发来的消息；
// 没有（例如主动跟进）就用回复稿本身。
func polishQuery(msgs []store.Message, draft string) string {
	var pending []string
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Status != "sent" {
			continue
		}
		if m.Sender == "agent" {
			break
		}
		pending = append([]string{m.Content}, pending...)
	}
	if len(pending) == 0 {
		return draft
	}
	return strings.Join(pending, "\n")
}

func (s *Server) polishOnce(ctx context.Context, draft string, refs []store.Memory) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	var b strings.Builder
	if len(refs) > 0 {
		b.WriteString("【我过去的回答】（只学写法，内容不能用）\n")
		for i, m := range refs {
			fmt.Fprintf(&b, "[例%d]\n%s\n\n", i+1, m.Content)
		}
	}
	b.WriteString("【要润色的回复稿】\n" + draft)
	text, err := s.llm.Complete(cctx, polishPrompt, []llm.Turn{{Role: "user", Content: b.String()}}, 1024)
	if err != nil {
		return "", err
	}
	return stripSignaturePlaceholders(stripCitations(strings.TrimSpace(text))), nil
}

// 出处标注只允许出现在占位符括号里（给业务员看）；出现在正文里会被发给客户，所以用代码删掉，
// 不依赖模型遵守"不要加出处标注"。
// 标注里可能嵌套括号（例如"（类似问答：客户（2026-11-21）：…）"），所以匹配到该行最后一个右括号。
var citationRe = regexp.MustCompile(`\s*[（(](?:资料《[^》]*》|历史\s*\d{4}-\d{2}-\d{2}[^：:\n]*|类似问答|其他客户[^：:\n]*)\s*[:：][^\n]*[）)]`)

// 模型偶尔把 prompt 模板里的占位词原样抄进来。
var templateTokens = strings.NewReplacer("<原话>", "", "<日期>", "", "<文档名>", "", "<该项内容>", "")

func stripCitations(s string) string {
	s = templateTokens.Replace(s)
	var b strings.Builder
	last := 0
	for _, loc := range placeholderRe.FindAllStringIndex(s, -1) {
		b.WriteString(citationRe.ReplaceAllString(s[last:loc[0]], ""))
		b.WriteString(s[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(citationRe.ReplaceAllString(s[last:], ""))
	return b.String()
}
