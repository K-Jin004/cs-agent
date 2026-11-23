package api

import (
	"regexp"
	"sort"
	"strings"
)

// ---------- 润色不能带进新的商务条款 ----------
// 润色会参考"我过去的回答"（含其他客户的记忆），学的是语气和回答方式。
// 那些回答里的条件（保修、免费服务、起订量……）可能只是给那个客户的，润色时绝不能搬进这封邮件。
// 数字由 diffNumbers 把关；不带数字的条款由这里把关：润色稿出现了原稿没有的条款词，就拒绝这一版。

var termRe = regexp.MustCompile(`(?i)\b(?:warrant(?:y|ies)|guarantee[ds]?|free(?:\s+of\s+charge)?|moq|minimum\s+order|lead\s*time|delivery\s+time|production\s+time|payment|t/t|deposit|l/c|discounts?|samples?|freight|shipping\s+cost)\b` +
	`|保修|质保|免费|起订|交期|交货期|付款|定金|折扣|样品|运费`)

// 常见客套话里的 free 不算条款。
var feelFreeRe = regexp.MustCompile(`(?i)\b(?:feel|feeling)\s+free\b|\bfree\s+to\b`)

func normTerm(t string) string {
	t = strings.ToLower(strings.Join(strings.Fields(t), " "))
	return strings.TrimSuffix(strings.TrimSuffix(t, "ies"), "s") // warranties→warrant, samples→sample
}

func termSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, t := range termRe.FindAllString(feelFreeRe.ReplaceAllString(s, ""), -1) {
		out[normTerm(t)] = true
	}
	return out
}

// addedTerms 返回润色稿里有、原稿里没有的商务条款词。
func addedTerms(before, after string) []string {
	b := termSet(before)
	var out []string
	for t := range termSet(after) {
		if !b[t] {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}
