-- 知识库：公司事实资料（产品规格、认证、公司介绍等），所有客户共用。
CREATE TABLE IF NOT EXISTS kb_documents (
  id BIGSERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT '',   -- 文件名或"粘贴"
  content TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS kb_chunks (
  id BIGSERIAL PRIMARY KEY,
  doc_id BIGINT NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
  seq INT NOT NULL,
  content TEXT NOT NULL,
  embedding vector(1024) NOT NULL, -- bge-m3，稠密检索
  tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED -- 关键词检索
);
CREATE INDEX IF NOT EXISTS idx_kb_chunks_doc ON kb_chunks(doc_id, seq);
CREATE INDEX IF NOT EXISTS idx_kb_chunks_tsv ON kb_chunks USING GIN (tsv)
