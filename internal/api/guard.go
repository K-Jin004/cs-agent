package api

import (
	"fmt"
	"regexp"
	"strings"

	"cs-agent/internal/store"
)

// 否认类说法（"我们没有 / 不生产 / 不能"）比编数字更危险：会直接替公司拒掉生意。
// 小模型不一定遵守 prompt，所以用代码找出这类句子提醒业务员核对，不自动改写。
var negationRe = regexp.MustCompile(`(?i)\b(we|i)\s+(do not|don't|cannot|can't|are not able to|are unable to|no longer|currently do not)\b` +
	`|\bnot\s+(currently\s+)?(available|manufactured?|produced?|made|offered|supplied)\b` +
	`|\bdo(es)?\s+not\s+(currently\s+)?(have|offer|make|manufacture|produce|supply)\b` +
	`|\bno\s+[\w-]+(\s+[\w-]+)?\s+in our\b` +
	`|我们没有|不提供|不生产|无法提供|不能做`)

var negWordRe = regexp.MustCompile(`(?i)\b(not|no|don't|cannot)\b|没有|不`)

// 替业务员做出的承诺（报价仍有效、保证……）也要提醒。
var commitmentRe = regexp.MustCompile(`(?i)\b(remains? valid|still valid|is valid (?:until|for)|price remains|we guarantee|guaranteed)\b|报价(?:仍然|依然)?有效|保证`)

// draftWarnings 返回草稿里需要业务员特别核对的句子。
func draftWarnings(draft string, kb []store.KBHit) []string {
	var out []string
	for _, line := range sentences(placeholderRe.ReplaceAllString(draft, "")) {
		if commitmentRe.MatchString(line) {
			out = append(out, "草稿替你做了承诺（例如报价仍有效），请确认是否属实："+line)
		}
		if !negationRe.MatchString(line) {
			continue
		}
		if supportedNegation(line, kb) {
			out = append(out, fmt.Sprintf("草稿否认了产品/能力（知识库里有类似的否定说法，仍请核对）：%s", line))
		} else {
			out = append(out, fmt.Sprintf("草稿否认了产品/能力，但知识库里没有依据，很可能是模型自己推断的，请核对后再发：%s", line))
		}
	}
	return out
}

// supportedNegation 粗略判断：有某段资料本身含否定词，且与该句共享至少 2 个关键词。
func supportedNegation(line string, kb []store.KBHit) bool {
	terms := strings.Split(keywordQuery(line), " | ")
	for _, h := range kb {
		if !negWordRe.MatchString(h.Content) {
			continue
		}
		lc := strings.ToLower(h.Content)
		shared := 0
		for _, t := range terms {
			if t != "" && !negWordRe.MatchString(t) && strings.Contains(lc, t) {
				shared++
			}
		}
		if shared >= 2 {
			return true
		}
	}
	return false
}

var sentenceEnd = regexp.MustCompile(`[.!?。！？]\s+|\n+`)

// sentences 按行和句末标点切句（只判断含否定词的那一句，避免同一行的其他句子被当成依据）。
func sentences(s string) []string {
	s = strings.NewReplacer("’", "'", "‘", "'").Replace(s) // 弯撇号 don’t → don't，否定检测才认得
	var out []string
	for _, p := range sentenceEnd.Split(s, -1) {
		p = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(p), "-•* "))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------- 价格检查 ----------
// 金额/折扣只能来自业务员自己：指示里写的，或已经发给这个客户的消息。
// 知识库价格表、其他会话的记忆都只是内部参考，出现在草稿正文里就要提醒。
var moneyRe = regexp.MustCompile(`(?i)(?:USD|US\$|EUR|RMB|CNY|\$|€|¥)\s?\d[\d,]*(?:\.\d+)?` +
	`|\d[\d,]*(?:\.\d+)?\s?(?:USD|EUR|RMB|CNY|/\s?pcs?\b|/\s?piece|/\s?unit|per (?:pc|pcs|piece|unit))` +
	`|\d+(?:\.\d+)?\s?%\s*(?:discount|off)`)

func priceWarnings(draft, trusted string) []string {
	have := numbers(trusted)
	var out []string
	seen := map[string]bool{}
	for _, m := range moneyRe.FindAllString(placeholderRe.ReplaceAllString(draft, ""), -1) {
		for n := range numbers(m) {
			if have[n] == 0 && !seen[m] {
				seen[m] = true
				out = append(out, fmt.Sprintf("草稿直接写出了价格/折扣「%s」，它不在你的指示或你已发给该客户的消息里（可能来自知识库内部价格表），不能直接报给客户，请改成确认后的价格", strings.TrimSpace(m)))
			}
		}
	}
	return out
}

// sentByAgent 本会话里业务员已经发给客户的内容（价格检查的可信来源）。
func sentByAgent(msgs []store.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Sender == "agent" && m.Status == "sent" {
			b.WriteString(m.Content + "\n")
		}
	}
	return b.String()
}

// 模型常在结尾加 "[Your Name]" 之类的英文占位行；署名由业务员自己的邮箱签名负责，直接删掉。
var signaturePlaceholderRe = regexp.MustCompile(`(?m)^[ \t]*\[(?:Your|Sales|Company|Name|Title|Phone|Email|Contact)[^\]\n]*\][ \t]*\n?`)

func stripSignaturePlaceholders(s string) string {
	return strings.TrimSpace(signaturePlaceholderRe.ReplaceAllString(s, ""))
}
