package rag

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitRespectsSizeAndHeadings(t *testing.T) {
	doc := "# IP65 Panel\n" + strings.Repeat("The IP65 panel is waterproof. ", 60) +
		"\n\n# Company\nWe were founded in 2010.\n"
	chunks := Split(doc, 300, 40)
	if len(chunks) < 3 {
		t.Fatalf("expected several chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if n := utf8.RuneCountInString(c); n > 300 {
			t.Errorf("chunk %d has %d runes > 300", i, n)
		}
		if !strings.HasPrefix(c, "# ") {
			t.Errorf("chunk %d lost its heading: %q", i, c[:20])
		}
	}
	last := chunks[len(chunks)-1]
	if !strings.HasPrefix(last, "# Company") || strings.Contains(last, "waterproof") {
		t.Errorf("sections must not mix: %q", last)
	}
}

func TestSplitOverlap(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("Sentence number " + string(rune('A'+i%26)) + " here. ")
	}
	chunks := Split(b.String(), 200, 50)
	for i := 1; i < len(chunks); i++ {
		prevTail := chunks[i-1][len(chunks[i-1])-15:]
		if !strings.Contains(chunks[i], strings.TrimSpace(prevTail)) {
			t.Errorf("chunk %d does not overlap previous tail %q", i, prevTail)
		}
	}
}

func TestSplitCJKAndHardCut(t *testing.T) {
	cjk := strings.Repeat("这是一段没有标点的中文", 50) // 500 字，无分隔符
	for _, c := range Split(cjk, 120, 10) {
		if utf8.RuneCountInString(c) > 120 {
			t.Fatalf("hard cut failed: %d runes", utf8.RuneCountInString(c))
		}
	}
}

func TestFuse(t *testing.T) {
	// 7 在两路都排第二，应超过只在一路排第一的 1 和 9
	got := Fuse(60, []int64{1, 7, 3}, []int64{9, 7})
	if got[0].ID != 7 {
		t.Fatalf("want 7 first, got %+v", got)
	}
	if len(got) != 4 {
		t.Fatalf("want 4 unique ids, got %d", len(got))
	}
}
