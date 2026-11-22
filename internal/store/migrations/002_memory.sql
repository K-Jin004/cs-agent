-- 对话记忆：业务员每发出一条回复，就把"客户来信 + 业务员实际回复"存成一条记忆。
-- 只存业务员审核发送过的内容（AI 草稿不入库），并记录来源消息，便于核对。
CREATE TABLE IF NOT EXISTS memories (
  id BIGSERIAL PRIMARY KEY,
  contact_id BIGINT NOT NULL REFERENCES contacts(id),
  conversation_id BIGINT NOT NULL REFERENCES conversations(id),
  reply_message_id BIGINT NOT NULL UNIQUE REFERENCES messages(id),
  source_message_ids BIGINT[] NOT NULL,
  content TEXT NOT NULL,
  embedding vector(1024) NOT NULL, -- bge-m3
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 检索总是先按客户过滤，单个客户的记忆量很小，精确扫描即可，暂不建 HNSW。
CREATE INDEX IF NOT EXISTS idx_memories_contact ON memories(contact_id);
CREATE INDEX IF NOT EXISTS idx_contacts_email ON contacts(lower(email));
CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts(lower(name))
