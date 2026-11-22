package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"cs-agent/internal/rag"
	"cs-agent/internal/store"
)

const (
	kbChunkSize    = 800 // 字符
	kbChunkOverlap = 100 // 字符
	kbMaxDocChars  = 200_000
	kbEmbedBatch   = 16
	kbRecallEach   = 10   // 每路检索取前 10
	kbTopN         = 4    // 融合后放进草稿的分块数
	kbRRFK         = 60.0 // RRF 平滑常数
	kbMaxDist      = 0.6  // 融合后再按向量距离过滤，挡掉只靠常见词命中的无关块（经验值）
)

// ---------- 文档管理 ----------

// createKBDocument POST /api/kb/documents {"title","source","content"}
// 前端读取 .txt/.md 文件内容或直接粘贴文本后提交。
func (s *Server) createKBDocument(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title   string `json:"title"`
		Source  string `json:"source"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&in); err != nil {
		fail(w, 400, fmt.Errorf("invalid body: %w", err))
		return
	}
	in.Title, in.Content = strings.TrimSpace(in.Title), strings.TrimSpace(in.Content)
	if in.Title == "" || in.Content == "" {
		fail(w, 400, errors.New("title and content are required"))
		return
	}
	if n := len([]rune(in.Content)); n > kbMaxDocChars {
		fail(w, 400, fmt.Errorf("document too long (%d chars, max %d); split it into several documents", n, kbMaxDocChars))
		return
	}
	if s.embed == nil || !s.embed.Enabled() {
		fail(w, 400, errors.New("embedding model not configured (EMBED_MODEL)"))
		return
	}
	chunks := rag.Split(in.Content, kbChunkSize, kbChunkOverlap)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	var vecs [][]float32
	for i := 0; i < len(chunks); i += kbEmbedBatch {
		v, err := s.embed.Embed(ctx, chunks[i:min(i+kbEmbedBatch, len(chunks))])
		if err != nil {
			fail(w, 502, err)
			return
		}
		vecs = append(vecs, v...)
	}
	doc, err := s.store.CreateKBDocument(r.Context(), in.Title, in.Source, in.Content, chunks, vecs)
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 201, doc)
}

func (s *Server) listKBDocuments(w http.ResponseWriter, r *http.Request) {
	docs, err := s.store.ListKBDocuments(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, docs)
}

func (s *Server) getKBDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := s.store.GetKBDocument(r.Context(), id)
	if err != nil {
		fail(w, 404, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) deleteKBDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteKBDocument(r.Context(), id); err != nil {
		fail(w, 500, err)
		return
	}
	w.WriteHeader(204)
}

// kbSearchTest POST /api/kb/search {"query"} —— 知识库页的"检索测试"，看某个问题能命中哪些资料。
func (s *Server) kbSearchTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Query) == "" {
		fail(w, 400, errors.New("query is required"))
		return
	}
	hits, err := s.searchKB(r.Context(), in.Query)
	if err != nil {
		fail(w, 502, err)
		return
	}
	writeJSON(w, 200, hits)
}

// ---------- 混合检索 ----------

// searchKB 稠密检索 + 关键词检索 → RRF 融合 → 按向量距离过滤 → 取前 kbTopN。
func (s *Server) searchKB(ctx context.Context, query string) ([]store.KBHit, error) {
	if s.embed == nil || !s.embed.Enabled() {
		return nil, nil
	}
	ectx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	vecs, err := s.embed.Embed(ectx, []string{query})
	if err != nil {
		return nil, err
	}
	dense, err := s.store.KBDenseIDs(ctx, vecs[0], kbRecallEach)
	if err != nil {
		return nil, err
	}
	kw, err := s.store.KBKeywordIDs(ctx, keywordQuery(query), kbRecallEach)
	if err != nil {
		return nil, err
	}
	fused := rag.Fuse(kbRRFK, dense, kw)
	if len(fused) == 0 {
		return []store.KBHit{}, nil
	}
	ids := make([]int64, len(fused))
	for i, f := range fused {
		ids[i] = f.ID
	}
	byID, err := s.store.KBChunksByIDs(ctx, ids, vecs[0])
	if err != nil {
		return nil, err
	}
	inDense, inKW := set(dense), set(kw)
	out := []store.KBHit{}
	for _, f := range fused {
		h, ok := byID[f.ID]
		if !ok || h.Distance > kbMaxDist {
			continue
		}
		h.Score = f.Score
		switch {
		case inDense[f.ID] && inKW[f.ID]:
			h.Via = "向量+关键词"
		case inKW[f.ID]:
			h.Via = "关键词"
		default:
			h.Via = "向量"
		}
		out = append(out, h)
		if len(out) == kbTopN {
			break
		}
	}
	return out, nil
}

func set(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "you": true, "your": true, "our": true, "are": true, "can": true,
	"what": true, "which": true, "with": true, "this": true, "that": true, "have": true, "has": true,
	"please": true, "hi": true, "hello": true, "dear": true, "thanks": true, "thank": true, "we": true,
	"is": true, "it": true, "of": true, "to": true, "in": true, "on": true, "do": true, "does": true,
	"a": true, "an": true, "be": true, "me": true, "us": true, "or": true, "if": true, "at": true, "by": true,
}

// keywordQuery 把客户来信拆成词，用 OR 拼成 tsquery（'ip65' | '36w' | ...）。
// 只保留字母数字，避免 tsquery 语法注入；中文连续字符会成为一个词，'simple' 分词对中文效果有限。
func keywordQuery(q string) string {
	seen := map[string]bool{}
	var terms []string
	for _, w := range strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(w)) < 2 || stopwords[w] || seen[w] {
			continue
		}
		seen[w] = true
		terms = append(terms, w)
		if len(terms) == 30 {
			break
		}
	}
	return strings.Join(terms, " | ")
}

// ---------- 记忆总览 ----------

// listAllMemories GET /api/memories —— 记忆页：所有客户的记忆。
func (s *Server) listAllMemories(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListAllMemories(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, rows)
}
