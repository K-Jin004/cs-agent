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

// draftWarnings 返回草稿里需要业务员特别核对的句子。
func draftWarnings(draft string, kb []store.KBHit) []string {
	var out []string
	for _, line := range sentences(placeholderRe.ReplaceAllString(draft, "")) {
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
	var out []string
	for _, p := range sentenceEnd.Split(s, -1) {
		p = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(p), "-•* "))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
