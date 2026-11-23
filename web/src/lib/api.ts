// 后端接口与数据类型（对应 internal/api 与 internal/store 的 JSON）。

export interface Conversation {
  id: number;
  contact_id: number;
  contact_name: string;
  language: string;
  status: string;
  remember: boolean;
  last_message: string;
  created_at: string;
}

export interface Message {
  id: number;
  conversation_id: number;
  sender: 'customer' | 'agent' | 'ai_draft';
  content: string;
  status: 'sent' | 'draft' | 'discarded';
  created_at: string;
}

export interface Memory {
  id: number;
  conversation_id: number;
  content: string;
  distance: number;
  created_at: string;
  contact_name?: string;
  other_customer: boolean;
}

export interface MemoryRow extends Memory {
  contact_id: number;
  contact_email: string;
}

export interface KBHit {
  id: number;
  doc_id: number;
  doc_title: string;
  seq: number;
  content: string;
  distance: number;
  score: number;
  via?: string;
}

export interface KBDocument {
  id: number;
  title: string;
  source: string;
  chars: number;
  chunks: number;
  created_at: string;
  content?: string;
  chunk_list?: KBHit[];
}

/** 生成草稿时返回的参考来源（后端不持久化，前端按会话缓存） */
export interface Sources {
  memories: Memory[];
  kb: KBHit[];
  warnings: string[];
}

export interface DraftResult extends Sources {
  draft: Message;
}

export class ApiError extends Error {
  constructor(message: string, public status: number, public body: Record<string, unknown>) {
    super(message);
  }
}

async function req<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const r = await fetch(path, {
    method,
    headers: { 'content-type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (r.status === 204) return null as T;
  let j: Record<string, unknown> = {};
  try {
    j = await r.json();
  } catch {
    /* 非 JSON 响应 */
  }
  if (!r.ok) throw new ApiError(String(j.error ?? `HTTP ${r.status}`), r.status, j);
  return j as T;
}

export const api = {
  conversations: () => req<Conversation[]>('/api/conversations'),
  createConversation: (name: string, email: string, language: string) =>
    req<Conversation>('/api/conversations', 'POST', { name, email, language }),
  messages: (id: number) => req<Message[]>(`/api/conversations/${id}/messages`),
  customerMessage: (id: number, content: string) =>
    req<Partial<DraftResult> & { message: Message; draft_error?: string }>(`/api/conversations/${id}/messages`, 'POST', { content }),
  regenerate: (id: number) => req<DraftResult>(`/api/conversations/${id}/draft`, 'POST'),
  compose: (id: number, instruction: string) => req<DraftResult>(`/api/conversations/${id}/compose`, 'POST', { instruction }),
  send: (msgId: number, content: string) => req<Message>(`/api/messages/${msgId}/send`, 'POST', { content }),
  discard: (msgId: number) => req<null>(`/api/messages/${msgId}/discard`, 'POST'),
  polish: (msgId: number, content: string) =>
    req<{ content: string; numbers_checked: number; attempts: number }>(`/api/messages/${msgId}/polish`, 'POST', { content }),
  setRemember: (id: number, remember: boolean) =>
    req<{ remember: boolean; saved: number }>(`/api/conversations/${id}/remember`, 'POST', { remember }),
  contactMemories: (id: number) => req<Memory[]>(`/api/conversations/${id}/memories`),
  allMemories: () => req<MemoryRow[]>('/api/memories'),
  deleteMemory: (id: number) => req<null>(`/api/memories/${id}`, 'DELETE'),
  kbDocuments: () => req<KBDocument[]>('/api/kb/documents'),
  kbDocument: (id: number) => req<KBDocument>(`/api/kb/documents/${id}`),
  createKBDocument: (title: string, content: string, source: string) =>
    req<KBDocument>('/api/kb/documents', 'POST', { title, content, source }),
  deleteKBDocument: (id: number) => req<null>(`/api/kb/documents/${id}`, 'DELETE'),
  kbSearch: (query: string) => req<KBHit[]>('/api/kb/search', 'POST', { query }),
};

export const LANGUAGES: Record<string, string> = {
  en: '英语',
  es: '西班牙语',
  fr: '法语',
  de: '德语',
  ru: '俄语',
  ar: '阿拉伯语',
};

export function fmtDate(iso: string) {
  const d = new Date(iso);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

export function fmtTime(iso: string) {
  const d = new Date(iso);
  const today = new Date();
  const hm = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  return d.toDateString() === today.toDateString() ? hm : `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`;
}

/** 草稿里的占位符，和后端 placeholderRe 一致 */
export const PLACEHOLDER_RE = /\[需业务员确认[:：][^\]]*\]/g;
