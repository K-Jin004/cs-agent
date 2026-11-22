-- 每个会话可选"记住这次对话"：打开后，业务员发出的回复才会存为该客户的记忆。默认关闭。
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS remember BOOLEAN NOT NULL DEFAULT false
