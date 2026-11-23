package store

import (
	"context"
	"time"
)

type KBDocument struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Source    string    `json:"source"`
	Chars     int       `json:"chars"`
	Chunks    int       `json:"chunks"`
	CreatedAt time.Time `json:"created_at"`
	Content   string    `json:"content,omitempty"`
	ChunkList []KBHit   `json:"chunk_list,omitempty"`
}

// KBHit 知识库分块（检索命中时带距离和融合分）。
type KBHit struct {
	ID       int64   `json:"id"`
	DocID    int64   `json:"doc_id"`
	DocTitle string  `json:"doc_title"`
	Seq      int     `json:"seq"`
	Content  string  `json:"content"`
	Distance float64 `json:"distance"`
	Score    float64 `json:"score"`
	Via      string  `json:"via,omitempty"` // 命中路数：向量 / 关键词 / 向量+关键词
}

// CreateKBDocument 在一个事务里写入文档和全部分块向量。
func (s *Store) CreateKBDocument(ctx context.Context, title, source, content string, chunks []string, vecs [][]float32) (*KBDocument, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	d := KBDocument{Title: title, Source: source, Chars: len([]rune(content)), Chunks: len(chunks)}
	if err := tx.QueryRow(ctx,
		`INSERT INTO kb_documents (title, source, content) VALUES ($1,$2,$3) RETURNING id, created_at`,
		title, source, content).Scan(&d.ID, &d.CreatedAt); err != nil {
		return nil, err
	}
	for i, c := range chunks {
		if _, err := tx.Exec(ctx,
			`INSERT INTO kb_chunks (doc_id, seq, content, embedding) VALUES ($1,$2,$3,$4::vector)`,
			d.ID, i, c, vecLiteral(vecs[i])); err != nil {
			return nil, err
		}
	}
	return &d, tx.Commit(ctx)
}

func (s *Store) ListKBDocuments(ctx context.Context) ([]KBDocument, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.title, d.source, char_length(d.content),
		       (SELECT count(*) FROM kb_chunks c WHERE c.doc_id = d.id), d.created_at
		FROM kb_documents d ORDER BY d.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KBDocument{}
	for rows.Next() {
		var d KBDocument
		if err := rows.Scan(&d.ID, &d.Title, &d.Source, &d.Chars, &d.Chunks, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetKBDocument(ctx context.Context, id int64) (*KBDocument, error) {
	var d KBDocument
	if err := s.pool.QueryRow(ctx,
		`SELECT id, title, source, content, char_length(content), created_at FROM kb_documents WHERE id=$1`, id).
		Scan(&d.ID, &d.Title, &d.Source, &d.Content, &d.Chars, &d.CreatedAt); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, seq, content FROM kb_chunks WHERE doc_id=$1 ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		h := KBHit{DocID: d.ID, DocTitle: d.Title}
		if err := rows.Scan(&h.ID, &h.Seq, &h.Content); err != nil {
			return nil, err
		}
		d.ChunkList = append(d.ChunkList, h)
	}
	d.Chunks = len(d.ChunkList)
	return &d, rows.Err()
}

func (s *Store) DeleteKBDocument(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM kb_documents WHERE id=$1`, id) // 分块级联删除
	return err
}

// KBDenseIDs 稠密检索：按余弦距离取前 k 个分块。
func (s *Store) KBDenseIDs(ctx context.Context, vec []float32, k int) ([]int64, error) {
	return s.ids(ctx, `SELECT id FROM kb_chunks ORDER BY embedding <=> $1::vector LIMIT $2`, vecLiteral(vec), k)
}

// KBKeywordIDs 关键词检索：tsquery 由调用方构造（词之间 OR），按 ts_rank_cd 排序。
func (s *Store) KBKeywordIDs(ctx context.Context, tsquery string, k int) ([]int64, error) {
	if tsquery == "" {
		return nil, nil
	}
	return s.ids(ctx, `
		SELECT id FROM kb_chunks, to_tsquery('simple', $1) q
		WHERE tsv @@ q ORDER BY ts_rank_cd(tsv, q) DESC, id LIMIT $2`, tsquery, k)
}

func (s *Store) ids(ctx context.Context, sql string, args ...any) ([]int64, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// KBChunksByIDs 取分块详情及其与查询向量的距离。
func (s *Store) KBChunksByIDs(ctx context.Context, ids []int64, vec []float32) (map[int64]KBHit, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.doc_id, d.title, c.seq, c.content, c.embedding <=> $2::vector
		FROM kb_chunks c JOIN kb_documents d ON d.id = c.doc_id
		WHERE c.id = ANY($1)`, ids, vecLiteral(vec))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]KBHit{}
	for rows.Next() {
		var h KBHit
		if err := rows.Scan(&h.ID, &h.DocID, &h.DocTitle, &h.Seq, &h.Content, &h.Distance); err != nil {
			return nil, err
		}
		out[h.ID] = h
	}
	return out, rows.Err()
}

// ---------- 记忆总览 ----------

type MemoryRow struct {
	Memory
	ContactID    int64  `json:"contact_id"`
	ContactEmail string `json:"contact_email"`
}

// ListAllMemories 所有客户的记忆（记忆页用），最新在前。
func (s *Store) ListAllMemories(ctx context.Context) ([]MemoryRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.conversation_id, m.content, m.created_at, ct.id, ct.name, ct.email
		FROM memories m JOIN contacts ct ON ct.id = m.contact_id
		ORDER BY m.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemoryRow{}
	for rows.Next() {
		var r MemoryRow
		if err := rows.Scan(&r.ID, &r.ConversationID, &r.Content, &r.CreatedAt, &r.ContactID, &r.ContactName, &r.ContactEmail); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecentAgentReplies 业务员在【其他会话】里最近实际发出的回复，用作"我的语气"示例。
// 不取本会话：本会话内容已在对话记录里，再当示例会被模型照抄（例如跟进时重复上一封邮件）。
// 只取业务员真正改写过的回复（edit_ratio >= minEdit），原样发出的 AI 草稿不算"我的语气"。
func (s *Store) RecentAgentReplies(ctx context.Context, convID int64, limit, minChars int, minEdit float64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT content FROM messages
		WHERE sender = 'agent' AND status = 'sent' AND char_length(content) >= $3 AND conversation_id <> $1
		  AND edit_ratio >= $4
		ORDER BY id DESC LIMIT $2`, convID, limit, minChars, minEdit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
