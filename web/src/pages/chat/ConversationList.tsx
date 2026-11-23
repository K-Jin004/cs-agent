import { clsx } from 'clsx';
import { useMemo, useState } from 'react';
import { LANGUAGES, type Conversation } from '../../lib/api';
import { Avatar, EmptyState, Icon, Modal, Spinner } from '../../components/ui';

export function ConversationList({ convs, loading, current, onSelect, onCreate }: {
  convs: Conversation[];
  loading: boolean;
  current: number | null;
  onSelect: (id: number) => void;
  onCreate: (name: string, email: string, language: string) => Promise<void>;
}) {
  const [q, setQ] = useState('');
  const [open, setOpen] = useState(false);

  const list = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return convs;
    return convs.filter((c) => c.contact_name.toLowerCase().includes(s) || c.last_message.toLowerCase().includes(s));
  }, [convs, q]);

  return (
    <section className="flex w-80 shrink-0 flex-col border-r border-smoke/60 bg-carbon/60">
      <div className="flex h-14 items-center justify-between px-4">
        <h2 className="text-sm font-semibold text-bone">
          会话 <span className="ml-1 text-xs font-normal text-ash">{convs.length}</span>
        </h2>
        <button className="btn-primary px-2.5 py-1.5 text-xs" onClick={() => setOpen(true)}>
          <Icon name="plus" className="h-3.5 w-3.5" /> 新建
        </button>
      </div>
      <div className="px-3 pb-3">
        <div className="relative">
          <Icon name="search" className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-ash" />
          <input className="field pl-8" placeholder="搜索客户或消息" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
      </div>
      <div className="flex-1 overflow-y-auto px-2 pb-3">
        {loading && !convs.length ? (
          <div className="flex justify-center py-10 text-fog"><Spinner /></div>
        ) : !list.length ? (
          q ? <p className="py-10 text-center text-xs text-ash">没有匹配的会话</p> : (
            <EmptyState icon="chat" title="还没有会话" hint="新建一个客户会话，然后在「模拟客户来信」输入客户消息，AI 会起草回复。"
              action={<button className="btn-primary" onClick={() => setOpen(true)}><Icon name="plus" /> 新建会话</button>} />
          )
        ) : (
          list.map((c) => (
            <button key={c.id} onClick={() => onSelect(c.id)}
              className={clsx('mb-0.5 flex w-full items-start gap-3 rounded-lg px-3 py-2.5 text-left transition-colors duration-150',
                c.id === current ? 'bg-graphite' : 'hover:bg-obsidian')}>
              <Avatar name={c.contact_name} />
              <span className="min-w-0 flex-1">
                <span className="flex items-center gap-1.5">
                  <span className={clsx('truncate text-[13px] font-medium', c.id === current ? 'text-bone' : 'text-mist')}>{c.contact_name}</span>
                  {c.remember && <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-lime" title="记住这次对话：已开启" />}
                  <span className="ml-auto shrink-0 text-[10px] uppercase text-ash">{c.language}</span>
                </span>
                <span className="mt-0.5 block truncate text-xs text-ash">{c.last_message || '暂无消息'}</span>
              </span>
            </button>
          ))
        )}
      </div>
      <NewConversationModal open={open} onClose={() => setOpen(false)} onCreate={onCreate} />
    </section>
  );
}

function NewConversationModal({ open, onClose, onCreate }: { open: boolean; onClose: () => void; onCreate: (n: string, e: string, l: string) => Promise<void> }) {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [lang, setLang] = useState('en');
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    if (!name.trim()) return;
    setBusy(true);
    try {
      await onCreate(name.trim(), email.trim(), lang);
      setName('');
      setEmail('');
      onClose();
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="新建会话"
      footer={<>
        <button className="btn-ghost" onClick={onClose}>取消</button>
        <button className="btn-primary" disabled={!name.trim() || busy} onClick={submit}>{busy && <Spinner className="h-3.5 w-3.5" />}创建</button>
      </>}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); submit(); }}>
        <div>
          <label className="field-label">客户名称 *</label>
          <input className="field" autoFocus placeholder="例如 John (ACME Ltd)" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div>
          <label className="field-label">客户邮箱</label>
          <input className="field" type="email" placeholder="可选：同一邮箱会识别为同一客户，共享历史往来" value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div>
          <label className="field-label">回复语言</label>
          <div className="flex flex-wrap gap-2">
            {Object.entries(LANGUAGES).map(([k, v]) => (
              <button type="button" key={k} onClick={() => setLang(k)}
                className={clsx('rounded-lg border px-3 py-1.5 text-xs transition-colors', lang === k ? 'border-lime/60 bg-lime/10 text-lime' : 'border-smoke text-fog hover:text-mist')}>
                {v}
              </button>
            ))}
          </div>
        </div>
        <button type="submit" className="hidden" />
      </form>
    </Modal>
  );
}
