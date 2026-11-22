CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE IF NOT EXISTS contacts (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  language TEXT NOT NULL DEFAULT 'en',
  attributes JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS conversations (
  id BIGSERIAL PRIMARY KEY,
  contact_id BIGINT NOT NULL REFERENCES contacts(id),
  status TEXT NOT NULL DEFAULT 'open',
  assignee TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS messages (
  id BIGSERIAL PRIMARY KEY,
  conversation_id BIGINT NOT NULL REFERENCES conversations(id),
  sender TEXT NOT NULL CHECK (sender IN ('customer','agent','ai_draft')),
  content TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'sent' CHECK (status IN ('sent','draft','discarded')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_messages_conv ON messages(conversation_id, id);
