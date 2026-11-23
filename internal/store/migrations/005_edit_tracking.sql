-- 改稿记录：发送时保存 AI 原稿和业务员改动比例。
-- 用途：1) 只把业务员真正改写过的回复当作"我的语气"示例；2) 以后统计改稿率（评估里程碑）。
-- 老数据为 NULL（不知道是否改过），不会被当作语气示例。
ALTER TABLE messages ADD COLUMN IF NOT EXISTS ai_original TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS edit_ratio REAL
