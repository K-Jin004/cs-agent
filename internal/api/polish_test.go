package api

import (
	"reflect"
	"strings"
	"testing"

	"cs-agent/internal/store"
)

func TestDiffNumbers(t *testing.T) {
	cases := []struct {
		name           string
		before, after  string
		missing, added []string
	}{
		{"identical", "USD 8.5/pc, 2000 pcs, 25 days", "For 2000 pcs the price is USD 8.5/pc and lead time is 25 days.", nil, nil},
		{"trailing zero ok", "USD 8.5", "USD 8.50", nil, nil},
		{"thousands comma ok", "2000 pcs", "2,000 pcs", nil, nil},
		{"changed price", "USD 8.5", "USD 8.2", []string{"8.5"}, []string{"8.2"}},
		{"dropped number", "36W and 48W", "36W", []string{"48"}, nil},
		{"invented number", "lead time 25 days", "lead time 25 days, MOQ 500", nil, []string{"500"}},
		{"duplicate count", "30% deposit, 70% balance, 30 days", "30% deposit and 70% balance", []string{"30"}, nil},
		{"spelled out is missing", "2000 pcs", "two thousand pcs", []string{"2000"}, nil},
	}
	for _, c := range cases {
		m, a := diffNumbers(c.before, c.after)
		if !reflect.DeepEqual(m, c.missing) || !reflect.DeepEqual(a, c.added) {
			t.Errorf("%s: got missing=%v added=%v, want missing=%v added=%v", c.name, m, a, c.missing, c.added)
		}
	}
}

func TestPlaceholderRe(t *testing.T) {
	s := "Price: [需业务员确认：单价（历史 2026-09-10：USD 8.5）] Lead: [需业务员确认:交期] Done: USD 8.2"
	if got := len(placeholderRe.FindAllString(s, -1)); got != 2 {
		t.Fatalf("want 2 placeholders, got %d", got)
	}
}

func TestStripCitations(t *testing.T) {
	in := "- DLC: DL-6060 is NOT DLC certified. (资料《DL-6060 规格书》：DL-6060 is NOT UL or DLC certified.)\n" +
		"- Price: [需业务员确认：单价（资料《DL-6060 规格书》：USD 8.2/pc）]\n" +
		"- Colors: black（历史 2026-09-10：black and white）"
	want := "- DLC: DL-6060 is NOT DLC certified.\n" +
		"- Price: [需业务员确认：单价（资料《DL-6060 规格书》：USD 8.2/pc）]\n" +
		"- Colors: black"
	if got := stripCitations(in); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDraftWarnings(t *testing.T) {
	kb := []store.KBHit{{Content: "DL-6060 is NOT UL or DLC certified at the moment."}, {Content: "Model DL-6060 is a 600x600 mm panel."}}
	draft := "Thank you.\n- We do not have a 60x60 panel model. The DL-6060 panel is 600x600 mm.\n" +
		"- DLC certification: DL-6060 is NOT DLC certified at the moment.\n- Price: [需业务员确认：单价]\n- We do not currently manufacture 1200x300 mm panels."
	w := draftWarnings(draft, kb)
	if len(w) != 2 {
		t.Fatalf("want 2 warnings (60x60, 1200x300), got %d: %v", len(w), w)
	}
	for _, s := range w {
		if !strings.Contains(s, "没有依据") {
			t.Errorf("expected unsupported: %s", s)
		}
	}
	if w := draftWarnings("DL-6060 does not have UL or DLC certification.", kb); len(w) != 1 || !strings.Contains(w[0], "有类似") {
		t.Errorf("expected supported negation warning, got %v", w)
	}
}
