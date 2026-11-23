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
	"strings"
	"time"

	"cs-agent/internal/llm"
	"cs-agent/internal/store"
)

// 业务员主动起草：客户没有新来信时，业务员给一句指示（"follow up"、"催付款"）或一段粗稿，
// AI 结合对话记录、知识库、该客户记忆和业务员自己的语气写成邮件草稿。

const composePrompt = `你是外贸业务员的写信助手。业务员现在要主动给客户发一封邮件（客户不一定有新来信）。
用户消息里有：【业务员的指示】【对话记录】【历史往来】（与该客户的过往回复）、
【类似问答】（我们过去回答其他客户同类问题的方式，价格已隐藏）、【知识库】（公司资料，辅助核对）、【我的语气示例】。
参考顺序：业务员的指示 > 对话记录 > 历史往来 > 类似问答 > 知识库。

最重要：【业务员的指示】优先于一切资料。业务员说的就是事实，知识库没提到的以业务员为准；
草稿绝不能写出与指示意思相反的内容（例如指示是 "Yes"，就不能写"我们没有/不能"）。

写法：
1. 按【业务员的指示】写。指示可能是：
   - 对客户问题的简短答复（例如客户问 "Do you make X?"，指示 "Yes" / "可以" = 肯定回答；"No" = 否定回答）；
   - 一个任务（例如 "follow up"、"催一下付款"）；
   - 一段粗稿：保留其中每一条信息和每一个数字，只改写成通顺、专业的邮件。
   指示里给了参数（瓦数、尺寸、数量、交期…）时，只用指示里的参数；
   不要从知识库找"相近型号"来替代或补充，也不要写出和指示不同的参数。
2. 结合【对话记录】自然地接上：知道上次聊到哪、隔了多久、客户还没回复什么、我们在等什么。
   例如 follow up 时提到上次发的报价或客户问过的产品，礼貌询问进展，不要催得生硬。
3. 模仿【我的语气示例】的称呼、开头结尾习惯和句子长短；示例只学语气，里面的内容一律不能用。
4. 使用对话里和客户沟通的语言；没有对话时用会话设置的语言。
5. 以称呼开头（例如 "Hi Tom,"，用客户名字里的名，不要用公司名）。
6. 写得像真人业务员：开门见山、句子简短。不要复述客户的问题，
   不要用 "Thank you for your message asking about…"、"I hope this message finds you well"、
   "We are pleased to inform you" 这类套话；不要用 markdown 加粗。

事实规则：
- 业务员指示里写的数字和事实，可以直接用。
- 【对话记录】里业务员已经发给客户的内容，可以引用（例如"我们 11 月 21 日的报价"）。
- 只有指示要求介绍产品或回答产品问题时，才使用【类似问答】【知识库】里的产品信息，并照原文写；
  过往回复和知识库说法冲突时，写占位符并列出两种说法；其他客户的商务条款（保修、免费服务、起订量等，即使不带数字）只能写进占位符，除非指示里业务员自己说了；
  follow up、催款、寄样通知这类邮件不要罗列产品参数，也不要重复整份报价。
- 【知识库】里的价格表是内部参考，任何价格、折扣都绝不能写在占位符之外；
  需要提价格时，只能用指示里写的、或【对话记录】里"我"已经报给客户的价格，否则写占位符。
- 任何新的价格、折扣、交期、付款条款、承诺，都不能自己编，写成占位符：[需业务员确认：<该项内容>]
- 不要编造客户说过的话；资料里没提到不等于没有，不要否认产品。

只输出邮件正文：不要标题、不要解释、不要出处标注、不要写署名（名字、公司、电话）。`

const (
	composeHistory  = 20
	toneExamples    = 4
	toneMinChars    = 40
	toneMinEdit     = 0.3 // 改动 30% 以上才算业务员自己的写法
	composeMaxInstr = 4000
)

// composeDraft POST /api/conversations/{id}/compose {"instruction": "..."}
func (s *Server) composeDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Instruction string `json:"instruction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Instruction) == "" {
		fail(w, 400, errors.New("instruction is required"))
		return
	}
	if len([]rune(in.Instruction)) > composeMaxInstr {
		fail(w, 400, fmt.Errorf("instruction too long (max %d chars)", composeMaxInstr))
		return
	}
	res, err := s.compose(r.Context(), id, strings.TrimSpace(in.Instruction))
	if err != nil {
		fail(w, 502, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) compose(ctx context.Context, convID int64, instruction string) (*draftResult, error) {
	if !s.llm.Enabled() {
		return nil, errors.New("LLM is not configured")
	}
	conv, err := s.store.GetConversation(ctx, convID)
	if err != nil {
		return nil, err
	}
	msgs, err := s.store.ListMessages(ctx, convID)
	if err != nil {
		return nil, err
	}
	var sent []store.Message
	for _, m := range msgs {
		if m.Status == "sent" {
			sent = append(sent, m)
		}
	}
	if len(sent) > composeHistory {
		sent = sent[len(sent)-composeHistory:]
	}

	// 检索查询 = 指示 + 最近一条客户来信（follow up 这类短指示本身信息很少）
	query := instruction
	for i := len(sent) - 1; i >= 0; i-- {
		if sent[i].Sender == "customer" {
			query += "\n" + sent[i].Content
			break
		}
	}
	mems := s.recall(ctx, convID, query)
	kb, err := s.searchKB(ctx, query)
	if err != nil {
		log.Printf("compose kb search (conv %d): %v", convID, err)
		kb = nil
	}
	tone, err := s.store.RecentAgentReplies(ctx, convID, toneExamples, toneMinChars, toneMinEdit)
	if err != nil {
		log.Printf("compose tone examples (conv %d): %v", convID, err)
	}

	// 起草邮件时模型看不到知识库里的金额（内部价格表）：看不到就不可能泄露。
	// 产品参数照常保留；价格由业务员在指示里给出，或沿用已报给客户的价格。
	kbForModel := make([]store.KBHit, len(kb))
	for i, h := range kb {
		h.Content = moneyRe.ReplaceAllString(h.Content, "[内部价格，需业务员确认]")
		kbForModel[i] = h
	}
	transcript := transcriptBlock(sent, conv)
	var b strings.Builder
	fmt.Fprintf(&b, "【业务员的指示】\n%s\n\n", instruction)
	b.WriteString(transcript)
	b.WriteString(situationBlock(sent))
	b.WriteString(referenceBlock(kbForModel, mems))
	b.WriteString("【我的语气示例】（只学语气，不用内容）\n")
	if len(tone) == 0 {
		b.WriteString("无\n")
	}
	for i, t := range tone {
		fmt.Fprintf(&b, "[示例%d]\n%s\n\n", i+1, t)
	}

	cctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	userMsg := b.String()
	clean := func(t string) string {
		t = stripSignaturePlaceholders(stripCitations(strings.TrimSpace(t)))
		if len(sent) > 0 && sent[len(sent)-1].Sender == "agent" {
			t = stripThanksOpener(t)
		}
		return addGreeting(t, conv)
	}
	text, err := s.llm.Complete(cctx, composePrompt, []llm.Turn{{Role: "user", Content: userMsg}}, 1024)
	if err != nil {
		return nil, err
	}
	text = clean(text)
	// 指示是肯定答复、草稿却在否认 → 带着明确提醒重写一次（小模型常被"知识库没提到"带偏）
	contradiction := isAffirmative(instruction) && hasNegation(text)
	if contradiction {
		log.Printf("compose conv %d: draft contradicts affirmative instruction %q, retrying", convID, instruction)
		retry := userMsg + "\n【注意】业务员的回答是肯定的（" + instruction + "）。上一版草稿写成了否定，这是错的。" +
			"请按肯定回答重写，不要出现\"没有/不能/不生产\"之类的话；资料里没写的具体参数用占位符。\n"
		if t, err := s.llm.Complete(cctx, composePrompt, []llm.Turn{{Role: "user", Content: retry}}, 1024); err == nil {
			text = clean(t)
			contradiction = hasNegation(text)
		}
	}
	if err := s.store.DiscardDrafts(ctx, convID); err != nil {
		return nil, err
	}
	text, termNotes := guardOtherTerms(text, mems, trustedSupport(instruction+"\n"+sentByAgent(sent), kb, mems))
	draft, err := s.store.AddMessage(ctx, convID, "ai_draft", text, "draft")
	if err != nil {
		return nil, err
	}

	// 草稿里的数字必须能在某个来源里找到（指示/对话/知识库/记忆），否则提醒业务员
	var src strings.Builder
	src.WriteString(instruction + "\n" + transcript)
	for _, h := range kb {
		src.WriteString(h.Content + "\n")
	}
	for _, m := range mems {
		src.WriteString(m.Content + "\n")
	}
	warnings := append(termNotes, draftWarnings(text, kb)...)
	if hasNegation(instruction) || negWordRe.MatchString(instruction) {
		// 否定是业务员自己说的，不需要"知识库没有依据"的提醒
		warnings = dropNegationWarnings(warnings)
	}
	if contradiction {
		warnings = append([]string{"草稿和你的指示意思相反（你说的是肯定，草稿在否认），已自动重写一次仍未纠正，请手动修改"}, warnings...)
	}
	warnings = append(warnings, priceWarnings(text, instruction+"\n"+sentByAgent(sent))...)
	warnings = append(warnings, unsourcedNumbers(text, src.String())...)
	if warnings == nil {
		warnings = []string{}
	}
	if mems == nil {
		mems = []store.Memory{}
	}
	if kb == nil {
		kb = []store.KBHit{}
	}
	return &draftResult{Draft: draft, Memories: mems, KB: kb, Warnings: warnings}, nil
}

// transcriptBlock 把对话整理成带日期的记录，并注明最后一条消息距今几天（follow up 用得上）。
func transcriptBlock(sent []store.Message, conv *store.Conversation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【对话记录】（客户：%s，会话语言：%s）\n", conv.ContactName, conv.Language)
	if len(sent) == 0 {
		b.WriteString("还没有任何往来，这是发给该客户的第一封邮件。\n\n")
		return b.String()
	}
	for _, m := range sent {
		who := "客户"
		if m.Sender == "agent" {
			who = "我"
		}
		fmt.Fprintf(&b, "%s（%s）：%s\n", who, m.CreatedAt.Format(dateFmt), m.Content)
	}
	last := sent[len(sent)-1]
	days := int(time.Since(last.CreatedAt).Hours() / 24)
	who := "客户"
	if last.Sender == "agent" {
		who = "我"
	}
	fmt.Fprintf(&b, "（最后一条消息是%s发的，距今 %d 天）\n\n", who, days)
	return b.String()
}

// dateFmt 同时给出英文日期写法，避免模型在英文邮件里写出 "11月21st" 这种混合格式。
const dateFmt = "2006-01-02 (Jan 2)"

// situationBlock 由程序先判断当前情况，直接告诉模型这是什么性质的邮件（小模型自己推断不可靠）。
func situationBlock(sent []store.Message) string {
	if len(sent) == 0 {
		return "【当前情况】这是发给该客户的第一封邮件：先简短自我介绍，再按指示写。\n\n"
	}
	last := sent[len(sent)-1]
	days := int(time.Since(last.CreatedAt).Hours() / 24)
	if last.Sender == "agent" {
		when := fmt.Sprintf("%d 天前", days)
		if days == 0 {
			when = "今天"
		}
		return fmt.Sprintf(`【当前情况】客户还没有回复我%s（%s）发的上一封邮件。如果指示是跟进：
- 客户没有新消息，不要写 "Thank you for your message/reply" 这类感谢来信的话；
- 不要重复上一封邮件里的内容和数字，用一句话提到上一封（日期和主题）即可；
- 礼貌询问对方是否看过、有没有问题或需要补充的资料；
- 简短，3 到 5 句。

`, when, last.CreatedAt.Format(dateFmt))
	}
	return "【当前情况】客户发来了新消息，我还没有回复。【业务员的指示】就是我对这条消息的答复要点：" +
		"直接围绕它回答客户，不要和它相反，也不要复述客户的问题。\n\n"
}

// 客户没有新消息时，模型仍常以"感谢来信"开头；跟进场景下直接删掉这句。
var thanksOpenerRe = regexp.MustCompile(`(?i)^\s*(?:(?:hi|dear|hello)\s+[^\n,]*,\s*\n+)?\s*thank(?:s| you) (?:so much )?for your (?:message|reply|email|response|inquiry)[^.!\n]*[.!]?\s*`)

func stripThanksOpener(s string) string {
	loc := thanksOpenerRe.FindStringSubmatchIndex(s)
	if loc == nil {
		return s
	}
	// 保留称呼行（Dear John,），只删感谢句
	head := s[:loc[1]]
	greeting := regexp.MustCompile(`(?i)^\s*(?:hi|dear|hello)\s+[^\n,]*,\s*\n+`).FindString(head)
	return strings.TrimSpace(greeting + s[loc[1]:])
}

// unsourcedNumbers 找出草稿正文（占位符以外）中、在所有来源里都没出现过的数字。
func unsourcedNumbers(draft, sources string) []string {
	have := numbers(sources)
	var missing []string
	for n := range numbers(placeholderRe.ReplaceAllString(draft, "")) {
		if have[n] == 0 {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return []string{fmt.Sprintf("草稿里有来源中找不到的数字：%s，可能是模型编的，请核对", strings.Join(missing, ", "))}
}

// affirmativeRe 简短的肯定答复："Yes"、"Sure, we can"、"可以"、"有的" 等。
var affirmativeRe = regexp.MustCompile(`(?i)^\s*(yes|yep|yeah|sure|of course|certainly|ok|okay|we do|we can|we have|有|可以|能|是的|对|没问题|行)`)

func isAffirmative(instruction string) bool {
	return len([]rune(instruction)) <= 60 && affirmativeRe.MatchString(instruction)
}

func hasNegation(draft string) bool {
	for _, sent := range sentences(placeholderRe.ReplaceAllString(draft, "")) {
		if negationRe.MatchString(sent) {
			return true
		}
	}
	return false
}

// editRatio 业务员改动比例：1 - 词级最长公共子序列 / 较长一方的词数。0=原样发出，1=完全重写。
func editRatio(original, final string) float64 {
	a, b := strings.Fields(original), strings.Fields(final)
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	if len(a) == 0 || len(b) == 0 {
		return 1
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			switch {
			case a[i-1] == b[j-1]:
				cur[j] = prev[j-1] + 1
			case prev[j] >= cur[j-1]:
				cur[j] = prev[j]
			default:
				cur[j] = cur[j-1]
			}
		}
		prev, cur = cur, prev
	}
	return 1 - float64(prev[len(b)])/float64(max(len(a), len(b)))
}

func dropNegationWarnings(ws []string) []string {
	var out []string
	for _, w := range ws {
		if !strings.HasPrefix(w, "草稿否认了产品/能力") {
			out = append(out, w)
		}
	}
	return out
}

var (
	greetingRe = regexp.MustCompile(`(?i)^\s*(?:(?:hi|hello|dear|hey|hola|estimad[oa]|bonjour|cher|ch[eè]re|hallo|guten tag)\b|sehr geehrte|здравствуйте|مرحبا)`)
	greetings  = map[string]string{"en": "Hi %s,", "es": "Hola %s,", "fr": "Bonjour %s,", "de": "Hallo %s,"}
)

// addGreeting 草稿没有称呼时，按客户名（取第一个词）和会话语言补上，例如 "Hi Tom,"。
func addGreeting(text string, conv *store.Conversation) string {
	if greetingRe.MatchString(text) {
		return text
	}
	f, ok := greetings[conv.Language]
	name := strings.Fields(strings.Split(conv.ContactName, "(")[0])
	if !ok || len(name) == 0 {
		return text
	}
	return fmt.Sprintf(f, name[0]) + "\n\n" + text
}
