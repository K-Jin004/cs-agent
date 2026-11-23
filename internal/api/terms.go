package api

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"cs-agent/internal/store"
)

// ---------- 其他客户的商务条款 ----------
// 【类似问答】来自其他客户。价格和百分比已由 maskOtherCustomer 遮掉，但"5 年保修""1000 件以上免费印 logo"
// 这类不带金额的条款遮不掉，而它们可能只是给那个客户的单独优惠。
// 规则：这类条款只能出现在占位符里，由业务员确认是否适用。小模型不一定遵守 prompt，所以用代码兜底：
// 草稿正文里的商务条款句，如果条款词出现在其他客户的记忆里、却在可信来源里找不到依据，就整句改成占位符。

var termRe = regexp.MustCompile(`(?i)\b(?:warrant(?:y|ies)|guarantee[ds]?|free(?:\s+of\s+charge)?|moq|minimum\s+order|lead\s*time|delivery\s+time|production\s+time|payment|t/t|deposit|l/c|discounts?|samples?|freight|shipping\s+cost)\b` +
	`|保修|质保|免费|起订|交期|交货期|付款|定金|折扣|样品|运费`)

// sentenceSpanRe 句子边界：英文句末标点后跟空白/结尾（不切开 8.5 这种小数）、中文句末标点、换行。
var sentenceSpanRe = regexp.MustCompile(`[.!?](?:\s+|$)|[。！？]|\n`)

const otherTermHint = "（来自其他客户的条件，是否适用该客户？）"

// guardOtherTerms 返回改写后的草稿和给业务员的提示。
// support = 可信来源：业务员的指示、业务员已发给该客户的消息、该客户自己的历史往来、知识库。
func guardOtherTerms(draft string, mems []store.Memory, support string) (string, []string) {
	var other strings.Builder
	for _, m := range mems {
		if m.OtherCustomer {
			other.WriteString(strings.ToLower(m.Content) + "\n")
		}
	}
	if other.Len() == 0 {
		return draft, nil
	}
	otherText := other.String()
	supLower := strings.ToLower(support)
	supNums := numbers(support)

	needsWrap := func(sent string) bool {
		terms := termRe.FindAllString(sent, -1)
		fromOther, unsupported := false, false
		for _, t := range terms {
			t = normTerm(t)
			fromOther = fromOther || strings.Contains(otherText, t)
			unsupported = unsupported || !strings.Contains(supLower, t)
		}
		if !fromOther {
			return false // 条款词不在其他客户的记录里，不是从那里来的
		}
		if unsupported {
			return true
		}
		for n := range numbers(sent) { // 条款词有依据，但数字（年限、件数）对不上
			if supNums[n] == 0 {
				return true
			}
		}
		return false
	}

	var out strings.Builder
	var notes []string
	// 只处理占位符之外的文本
	last := 0
	for _, loc := range append(placeholderRe.FindAllStringIndex(draft, -1), []int{len(draft), len(draft)}) {
		seg := draft[last:loc[0]]
		out.WriteString(wrapSentences(seg, needsWrap, &notes))
		out.WriteString(draft[loc[0]:loc[1]])
		last = loc[1]
	}
	return out.String(), notes
}

func normTerm(t string) string {
	t = strings.ToLower(strings.Join(strings.Fields(t), " "))
	return strings.TrimSuffix(strings.TrimSuffix(t, "ies"), "s") // warranties→warrant, samples→sample
}

func wrapSentences(seg string, needsWrap func(string) bool, notes *[]string) string {
	var b strings.Builder
	start := 0
	bounds := append(sentenceSpanRe.FindAllStringIndex(seg, -1), []int{len(seg), len(seg)})
	for _, bd := range bounds {
		span := seg[start:bd[0]]
		b.WriteString(wrapOne(span, needsWrap, notes))
		b.WriteString(seg[bd[0]:bd[1]])
		start = bd[1]
	}
	return b.String()
}

// wrapOne 把一句改成占位符；保留行首空白、列表符号和 "Warranty:" 这类短标签。
func wrapOne(span string, needsWrap func(string) bool, notes *[]string) string {
	body := strings.TrimLeft(span, " \t-•*")
	prefix := span[:len(span)-len(body)]
	body = strings.TrimRight(body, " \t")
	suffix := span[len(prefix)+len(body):]
	// 只剩 "Warranty:" 这种标签（后面紧跟占位符）时不动
	if body == "" || strings.HasSuffix(body, ":") || strings.HasSuffix(body, "：") || !needsWrap(body) {
		return span
	}
	if i := strings.IndexAny(body, ":："); i > 0 && len([]rune(body[:i])) <= 30 {
		_, size := utf8.DecodeRuneInString(body[i:])
		label, rest := body[:i+size], strings.TrimSpace(body[i+size:])
		if termRe.MatchString(label) && rest != "" {
			prefix += label + " "
			body = rest
		}
	}
	inner := strings.NewReplacer("[", "", "]", "").Replace(body)
	*notes = append(*notes, fmt.Sprintf("这句条件来自其他客户的记录，已改成占位符，请确认是否适用该客户：%s", inner))
	return prefix + "[需业务员确认：" + inner + otherTermHint + "]" + suffix
}

// trustedSupport 拼出可信来源：业务员自己写的/发的 + 该客户自己的历史往来 + 知识库。
func trustedSupport(own string, kb []store.KBHit, mems []store.Memory) string {
	var b strings.Builder
	b.WriteString(own + "\n")
	for _, h := range kb {
		b.WriteString(h.Content + "\n")
	}
	for _, m := range mems {
		if !m.OtherCustomer {
			b.WriteString(m.Content + "\n")
		}
	}
	return b.String()
}
