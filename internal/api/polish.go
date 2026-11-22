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
)

// 业务员把占位符填好后，"润色"把条目式草稿改写成通顺的完整回复。
// 安全措施不靠模型自觉：
//   1. 还有未填的占位符 → 拒绝润色；
//   2. 用代码比对润色前后的数字集合，有缺失/改动/新增 → 自动重试一次，仍不一致则拒绝并返回差异。

const polishPrompt = `你是外贸邮件润色助手。下面是业务员已经确认过内容的回复稿，请把它改写成通顺、专业、自然的邮件正文。

规则：
1. 使用与原稿相同的语言。
2. 原稿里的每一个数字、单位、币种、型号、产品名、条款都必须原样保留，一个字符都不能改（例如 8.5 不能写成 8.50，2000 不能写成 2,000 或 two thousand）。
3. 不能增加原稿没有的任何信息、数字、承诺或产品细节；也不能删掉原稿里的任何信息。
4. 可以把条目改写成段落；项目较多时也可以保留条目，但语句要完整自然。
5. 只输出改写后的正文，不要加解释、标题或签名。`

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

	var last polishError
	for attempt := 1; attempt <= 2; attempt++ {
		out, err := s.polishOnce(r.Context(), in.Content)
		if err != nil {
			fail(w, 502, err)
			return
		}
		missing, added := diffNumbers(in.Content, out)
		leftover := placeholderRe.FindAllString(out, -1)
		if len(missing) == 0 && len(added) == 0 && len(leftover) == 0 {
			total := 0
			for _, c := range numbers(in.Content) {
				total += c
			}
			writeJSON(w, 200, map[string]any{"content": out, "numbers_checked": total, "attempts": attempt})
			return
		}
		log.Printf("polish msg %d attempt %d rejected: missing=%v added=%v", id, attempt, missing, added)
		last = polishError{
			Error:     "润色结果的数字与原稿不一致，已拒绝替换，原稿保持不变",
			Missing:   missing,
			Added:     added,
			Candidate: out,
		}
	}
	writeJSON(w, 422, last)
}

func (s *Server) polishOnce(ctx context.Context, draft string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	text, err := s.llm.Complete(cctx, polishPrompt, []llm.Turn{{Role: "user", Content: draft}}, 1024)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// 出处标注只允许出现在占位符括号里（给业务员看）；出现在正文里会被发给客户，所以用代码删掉，
// 不依赖模型遵守"不要加出处标注"。
var citationRe = regexp.MustCompile(`\s*[（(](?:资料《[^》]*》|历史\s*\d{4}-\d{2}-\d{2})\s*[:：][^）)]*[）)]`)

func stripCitations(s string) string {
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
