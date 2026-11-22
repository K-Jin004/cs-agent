// Package rag 知识库分块与检索融合。
package rag

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Split 把文档切成带重叠的块（思路同 nexus-agent splitter）：
//  1. 先按 markdown 标题分段，同一块不跨标题；
//  2. 段内按 "\n\n" → "\n" → 句号 → 空格 递归切分，直到不超过 size 个字符；
//  3. 相邻块之间保留约 overlap 个字符的重叠，保证语义连续。
//
// 每块前面加上所属标题，检索命中时也知道这段在讲什么。
func Split(content string, size, overlap int) []string {
	if size <= 0 {
		size = 800
	}
	if overlap < 0 || overlap >= size {
		overlap = size / 8
	}
	var out []string
	for _, sec := range splitByHeading(content) {
		body := strings.TrimSpace(sec.body)
		if body == "" {
			continue
		}
		budget := size - utf8.RuneCountInString(sec.heading) - 1
		if budget < size/2 {
			budget = size / 2
		}
		for _, p := range merge(pieces(body, budget), budget, overlap) {
			if sec.heading != "" {
				p = sec.heading + "\n" + p
			}
			out = append(out, p)
		}
	}
	return out
}

type section struct{ heading, body string }

func splitByHeading(content string) []section {
	var secs []section
	cur := section{}
	var b strings.Builder
	flush := func() {
		cur.body = b.String()
		if strings.TrimSpace(cur.body) != "" || cur.heading != "" {
			secs = append(secs, cur)
		}
		b.Reset()
	}
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			flush()
			cur = section{heading: strings.TrimSpace(line)}
			continue
		}
		b.WriteString(line + "\n")
	}
	flush()
	return secs
}

var separators = []string{"\n\n", "\n", "。", ". ", "；", "; ", " "}

// pieces 递归切分，保证每片不超过 size 个字符（实在切不开就按字符硬切）。
func pieces(text string, size int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if utf8.RuneCountInString(text) <= size {
		return []string{text}
	}
	for _, sep := range separators {
		if !strings.Contains(text, sep) {
			continue
		}
		parts := strings.SplitAfter(text, sep)
		var out []string
		for _, p := range parts {
			out = append(out, pieces(p, size)...)
		}
		return out
	}
	r := []rune(text)
	var out []string
	for i := 0; i < len(r); i += size {
		j := min(i+size, len(r))
		out = append(out, string(r[i:j]))
	}
	return out
}

// merge 把小片拼回接近 size 的块，并在块与块之间保留 overlap 个字符的尾部重叠。
func merge(ps []string, size, overlap int) []string {
	var out []string
	var cur []string
	curLen := 0
	join := func(xs []string) string { return strings.TrimSpace(strings.Join(xs, " ")) }
	for _, p := range ps {
		n := utf8.RuneCountInString(p)
		if curLen > 0 && curLen+n+1 > size {
			out = append(out, join(cur))
			// 从尾部保留不超过 overlap 的片段作为下一块开头
			var keep []string
			kl := 0
			for i := len(cur) - 1; i >= 0; i-- {
				l := utf8.RuneCountInString(cur[i])
				if kl+l > overlap {
					break
				}
				keep = append([]string{cur[i]}, keep...)
				kl += l + 1
			}
			cur, curLen = keep, kl
		}
		cur = append(cur, p)
		curLen += n + 1
	}
	if len(cur) > 0 {
		out = append(out, join(cur))
	}
	return out
}

// Fuse 用 RRF 融合多路检索结果：score(d) = Σ 1/(k + rank_i(d))，名次从 1 开始。
// 只看名次不看原始分数，避免余弦距离和关键词分数量纲不可比。
func Fuse(k float64, lists ...[]int64) []Scored {
	score := map[int64]float64{}
	for _, l := range lists {
		for i, id := range l {
			score[id] += 1 / (k + float64(i+1))
		}
	}
	out := make([]Scored, 0, len(score))
	for id, s := range score {
		out = append(out, Scored{ID: id, Score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out
}

type Scored struct {
	ID    int64
	Score float64
}
