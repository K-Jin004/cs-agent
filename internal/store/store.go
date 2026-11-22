package store

import (
	"context"
	"embed"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ pool *pgxpool.Pool }

type Conversation struct {
	ID          int64     `json:"id"`
	ContactID   int64     `json:"contact_id"`
	ContactName string    `json:"contact_name"`
	Language    string    `json:"language"`
	Status      string    `json:"status"`
	Remember    bool      `json:"remember"` // 是否"记住这次对话"
	LastMessage string    `json:"last_message"`
	CreatedAt   time.Time `json:"created_at"`
}

type Message struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	Sender         string    `json:"sender"` // customer | agent | ai_draft
	Content        string    `json:"content"`
	Status         string    `json:"status"` // sent | draft | discarded
	CreatedAt      time.Time `json:"created_at"`
}

// New 连接数据库并执行建表（幂等）。
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	files, err := migrations.ReadDir("migrations") // 按文件名排序
	if err != nil {
		pool.Close()
		return nil, err
	}
	for _, f := range files {
		sql, err := migrations.ReadFile("migrations/" + f.Name())
		if err != nil {
			pool.Close()
			return nil, err
		}
		for _, stmt := range strings.Split(string(sql), ";\n") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := pool.Exec(ctx, stmt); err != nil {
				pool.Close()
				return nil, fmt.Errorf("migrate %s: %w", f.Name(), err)
			}
		}
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

const convSelect = `
SELECT c.id, c.contact_id, ct.name, ct.language, c.status, c.remember,
  COALESCE((SELECT m.content FROM messages m
            WHERE m.conversation_id = c.id AND m.status <> 'discarded'
            ORDER BY m.id DESC LIMIT 1), ''),
  c.created_at
FROM conversations c JOIN contacts ct ON ct.id = c.contact_id `

func (s *Store) CreateConversation(ctx context.Context, name, email, lang string) (*Conversation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 同一客户复用联系人，记忆才能跨会话召回：有邮箱按邮箱匹配，否则按名称匹配。
	var contactID, convID int64
	email = strings.TrimSpace(email)
	var findErr error
	if email != "" {
		findErr = tx.QueryRow(ctx,
			`SELECT id FROM contacts WHERE lower(email) = lower($1) ORDER BY id LIMIT 1`, email).Scan(&contactID)
	} else {
		findErr = tx.QueryRow(ctx,
			`SELECT id FROM contacts WHERE email = '' AND lower(name) = lower($1) ORDER BY id LIMIT 1`,
			strings.TrimSpace(name)).Scan(&contactID)
	}
	if findErr != nil {
		if err := tx.QueryRow(ctx,
			`INSERT INTO contacts (name, email, language) VALUES ($1,$2,$3) RETURNING id`,
			strings.TrimSpace(name), email, lang).Scan(&contactID); err != nil {
			return nil, err
		}
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO conversations (contact_id) VALUES ($1) RETURNING id`,
		contactID).Scan(&convID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetConversation(ctx, convID)
}

func (s *Store) GetConversation(ctx context.Context, id int64) (*Conversation, error) {
	var c Conversation
	err := s.pool.QueryRow(ctx, convSelect+`WHERE c.id = $1`, id).
		Scan(&c.ID, &c.ContactID, &c.ContactName, &c.Language, &c.Status, &c.Remember, &c.LastMessage, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListConversations(ctx context.Context) ([]Conversation, error) {
	rows, err := s.pool.Query(ctx, convSelect+`ORDER BY c.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.ContactID, &c.ContactName, &c.Language, &c.Status, &c.Remember, &c.LastMessage, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListMessages 返回会话中未被丢弃的消息（含待审核草稿），按时间升序。
func (s *Store) ListMessages(ctx context.Context, convID int64) ([]Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, conversation_id, sender, content, status, created_at
		 FROM messages WHERE conversation_id = $1 AND status <> 'discarded' ORDER BY id`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Sender, &m.Content, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AddMessage(ctx context.Context, convID int64, sender, content, status string) (*Message, error) {
	m := Message{ConversationID: convID, Sender: sender, Content: content, Status: status}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO messages (conversation_id, sender, content, status)
		 VALUES ($1,$2,$3,$4) RETURNING id, created_at`,
		convID, sender, content, status).Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) GetMessage(ctx context.Context, id int64) (*Message, error) {
	var m Message
	err := s.pool.QueryRow(ctx,
		`SELECT id, conversation_id, sender, content, status, created_at FROM messages WHERE id = $1`, id).
		Scan(&m.ID, &m.ConversationID, &m.Sender, &m.Content, &m.Status, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) UpdateMessage(ctx context.Context, id int64, sender, content, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE messages SET sender=$2, content=$3, status=$4 WHERE id=$1`, id, sender, content, status)
	return err
}

// DiscardDrafts 丢弃会话中所有待审核草稿（生成新草稿前调用）。
func (s *Store) DiscardDrafts(ctx context.Context, convID int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE messages SET status='discarded' WHERE conversation_id=$1 AND status='draft'`, convID)
	return err
}

// ---------- 对话记忆 ----------

type Memory struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	Content        string    `json:"content"`
	Distance       float64   `json:"distance"` // 余弦距离，越小越相似
	CreatedAt      time.Time `json:"created_at"`
}

// vecLiteral 把向量转成 pgvector 文本格式 "[0.1,0.2,...]"，避免引入额外依赖。
func vecLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// AddMemory 写入一条记忆（同一条回复重复写入时忽略）。
func (s *Store) AddMemory(ctx context.Context, contactID, convID, replyMsgID int64, sourceIDs []int64, content string, vec []float32) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO memories (contact_id, conversation_id, reply_message_id, source_message_ids, content, embedding)
		 VALUES ($1,$2,$3,$4,$5,$6::vector) ON CONFLICT (reply_message_id) DO NOTHING`,
		contactID, convID, replyMsgID, sourceIDs, content, vecLiteral(vec))
	return err
}

// SearchMemories 在【该客户】的记忆里做向量检索：先按 contact_id 过滤，再按相似度排序。
// excludeConv 排除当前会话（当前会话已在上下文里）。
func (s *Store) SearchMemories(ctx context.Context, contactID, excludeConv int64, vec []float32, k int, maxDist float64) ([]Memory, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, conversation_id, content, embedding <=> $3::vector AS dist, created_at
		 FROM memories
		 WHERE contact_id = $1 AND conversation_id <> $2
		 ORDER BY dist LIMIT $4`,
		contactID, excludeConv, vecLiteral(vec), k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Content, &m.Distance, &m.CreatedAt); err != nil {
			return nil, err
		}
		if m.Distance <= maxDist {
			out = append(out, m)
		}
	}
	return out, rows.Err()
}

// SetRemember 打开/关闭会话的"记住这次对话"。
func (s *Store) SetRemember(ctx context.Context, convID int64, on bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE conversations SET remember=$2 WHERE id=$1`, convID, on)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("conversation %d not found", convID)
	}
	return nil
}

// HasMemory 该回复是否已存为记忆（避免重复向量化）。
func (s *Store) HasMemory(ctx context.Context, replyMsgID int64) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memories WHERE reply_message_id=$1)`, replyMsgID).Scan(&ok)
	return ok, err
}

// ListContactMemories 列出某客户的全部记忆（最新在前）。
func (s *Store) ListContactMemories(ctx context.Context, contactID int64) ([]Memory, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, conversation_id, content, 0::float8, created_at
		 FROM memories WHERE contact_id=$1 ORDER BY id DESC`, contactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Content, &m.Distance, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMemory 删除一条记忆（只删记忆，不删原始消息）。
func (s *Store) DeleteMemory(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM memories WHERE id=$1`, id)
	return err
}
