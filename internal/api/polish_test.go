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

func TestUnsourcedNumbers(t *testing.T) {
	src := "我（2026-11-21）：USD 8.5/pc for 5000 pcs, lead time 25 days"
	if w := unsourcedNumbers("Following up on our quote of USD 8.5/pc for 5000 pcs. [需业务员确认：新价格 7.9]", src); w != nil {
		t.Errorf("numbers inside placeholders or from sources must not warn: %v", w)
	}
	w := unsourcedNumbers("We can offer a 3% discount and ship within 15 days.", src)
	if len(w) != 1 || !strings.Contains(w[0], "15") || !strings.Contains(w[0], "3") {
		t.Errorf("want warning for 3 and 15, got %v", w)
	}
}

func TestPriceWarnings(t *testing.T) {
	trusted := "我（2026-11-21）：our price is USD 8.5/pc FOB Shenzhen"
	if w := priceWarnings("Following up on our quote of USD 8.50/pc. New price: [需业务员确认：新价（资料：USD 7.9/pc）]", trusted); w != nil {
		t.Errorf("already-sent price and placeholder hints must pass: %v", w)
	}
	w := priceWarnings("For 5000 pcs the price is USD 7.9/pc, and we can give 5% discount.", trusted)
	if len(w) != 2 {
		t.Fatalf("want 2 warnings (7.9 and 5%%), got %v", w)
	}
	if w := priceWarnings("Rated power 40W, 4000 lm, 5 years warranty.", trusted); w != nil {
		t.Errorf("product specs are not prices: %v", w)
	}
}

func TestStripSignaturePlaceholders(t *testing.T) {
	in := "Best regards,\n[Your Name]\n[Your Company]\n"
	if got := stripSignaturePlaceholders(in); got != "Best regards," {
		t.Fatalf("got %q", got)
	}
	keep := "Price: [需业务员确认：单价]"
	if got := stripSignaturePlaceholders(keep); got != keep {
		t.Fatalf("must keep business placeholders, got %q", got)
	}
}

func TestCommitmentWarning(t *testing.T) {
	w := draftWarnings("Our original quote of USD 8.5/pc remains valid. Logo printing is free.", nil)
	if len(w) != 1 || !strings.Contains(w[0], "承诺") {
		t.Fatalf("want 1 commitment warning, got %v", w)
	}
}

func TestStripThanksOpener(t *testing.T) {
	cases := map[string]string{
		"Thank you for your message.\n\nJust checking in on my last email.": "Just checking in on my last email.",
		"Dear John,\n\nThank you for your reply! Just checking in.":         "Dear John,\n\nJust checking in.",
		"Hi Lisa,\nJust checking in on the samples.":                        "Hi Lisa,\nJust checking in on the samples.",
	}
	for in, want := range cases {
		if got := stripThanksOpener(in); got != want {
			t.Errorf("stripThanksOpener(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEditRatio(t *testing.T) {
	if r := editRatio("Thank you. Price is X.", "Thank you. Price is X."); r != 0 {
		t.Errorf("unchanged should be 0, got %v", r)
	}
	if r := editRatio("a b c d e f g h i j", "a b c d e f g h i k"); r < 0.05 || r > 0.15 {
		t.Errorf("one word of ten changed should be ~0.1, got %v", r)
	}
	if r := editRatio("Thank you for your message.", "Hi John, yes we make those."); r < 0.8 {
		t.Errorf("rewrite should be high, got %v", r)
	}
}

func TestAffirmativeContradiction(t *testing.T) {
	if !isAffirmative("Yes") || !isAffirmative("可以，交期30天") || isAffirmative("follow up") {
		t.Error("isAffirmative misclassified")
	}
	if !hasNegation("Hi,\n\nWe currently do not have a product model sized at 1200x300 mm.") {
		t.Error("should detect negation")
	}
	if hasNegation("Hi,\n\nYes, we make 1200x300 panels. [需业务员确认：功率]") {
		t.Error("false negation")
	}
}

func TestCurlyApostropheNegation(t *testing.T) {
	if !hasNegation("No, we don’t have standard 1200x300 panels.") {
		t.Error("curly apostrophe negation not detected")
	}
}

func TestAddGreeting(t *testing.T) {
	c := &store.Conversation{ContactName: "Tom (Omega Ltd)", Language: "en"}
	if got := addGreeting("Yes, we do make them.", c); got != "Hi Tom,\n\nYes, we do make them." {
		t.Errorf("got %q", got)
	}
	if got := addGreeting("Dear Tom,\n\nYes.", c); got != "Dear Tom,\n\nYes." {
		t.Errorf("must keep existing greeting, got %q", got)
	}
}

func TestAddGreetingWordBoundary(t *testing.T) {
	c := &store.Conversation{ContactName: "Tom", Language: "en"}
	if got := addGreeting("Highlights of our new range:", c); got != "Hi Tom,\n\nHighlights of our new range:" {
		t.Errorf("got %q", got)
	}
}

func TestMaskOtherCustomer(t *testing.T) {
	in := "客户（2026-11-21）：Hi, please quote 5000 pcs.\n业务员（2026-11-21）：Dear John, our price is USD 8.5/pc FOB Shenzhen, payment 30% T/T deposit, 5 years warranty."
	got := maskOtherCustomer(in, "John (ACME Ltd)")
	for _, leak := range []string{"8.5", "30%", "John", "ACME"} {
		if strings.Contains(got, leak) {
			t.Errorf("leaked %q in %q", leak, got)
		}
	}
	for _, keep := range []string{"5000 pcs", "5 years warranty", "FOB Shenzhen"} {
		if !strings.Contains(got, keep) {
			t.Errorf("should keep %q in %q", keep, got)
		}
	}
}

func TestStripNestedCitation(t *testing.T) {
	in := "- Logo printing: Yes, free for orders above 1000 pcs. (类似问答：客户（2026-11-21）：... logo printing on the carton is free.)\n" +
		"- Price: [需业务员确认：单价（历史 2026-11-21：<原话> USD 8.5/pc）]"
	want := "- Logo printing: Yes, free for orders above 1000 pcs.\n" +
		"- Price: [需业务员确认：单价（历史 2026-11-21： USD 8.5/pc）]"
	if got := stripCitations(in); got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestAddedTerms(t *testing.T) {
	before := "Hi Nina,\n\nThe price for 3000 pcs is USD 8.5/pc. Lead time is 25 days.\nFeel free to contact us."
	if got := addedTerms(before, "Hi Nina, the price for 3000 pcs is USD 8.5/pc and lead time is 25 days. Please feel free to reach out."); len(got) != 0 {
		t.Fatalf("no new terms expected, got %v", got)
	}
	got := addedTerms(before, "Hi Nina, USD 8.5/pc, lead time 25 days, and we offer a free warranty on all panels.")
	if !reflect.DeepEqual(got, []string{"free", "warranty"}) {
		t.Fatalf("got %v", got)
	}
	if got := addedTerms("请确认。", "请确认，我们提供免费样品。"); !reflect.DeepEqual(got, []string{"免费", "样品"}) {
		t.Fatalf("got %v", got)
	}
}

func TestPolishQuery(t *testing.T) {
	msgs := []store.Message{
		{Sender: "customer", Status: "sent", Content: "old q"},
		{Sender: "agent", Status: "sent", Content: "old a"},
		{Sender: "customer", Status: "sent", Content: "q1"},
		{Sender: "customer", Status: "sent", Content: "q2"},
		{Sender: "ai_draft", Status: "draft", Content: "draft"},
	}
	if got := polishQuery(msgs, "d"); got != "q1\nq2" {
		t.Fatalf("got %q", got)
	}
	if got := polishQuery(msgs[:2], "d"); got != "d" {
		t.Fatalf("follow-up should fall back to draft, got %q", got)
	}
}
