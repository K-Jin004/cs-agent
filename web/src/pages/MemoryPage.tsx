import { clsx } from 'clsx';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { api, type Conversation, type MemoryRow } from '../lib/api';
import { Avatar, Badge, EmptyState, Icon, Modal, PageHeader, Spinner, useFeedback } from '../components/ui';
import { MemoryCard } from '../components/MemoryCard';

export function MemoryPage() {
  const { toast, confirm } = useFeedback();
  const [mems, setMems] = useState<MemoryRow[] | null>(null);
  const [filter, setFilter] = useState<number | null>(null);
  const [q, setQ] = useState('');
  const [pickOpen, setPickOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      setMems(await api.allMemories());
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), 'err');
      setMems([]);
    }
  }, [toast]);

  useEffect(() => { load(); }, [load]);

  const contacts = useMemo(() => {
    const m = new Map<number, { name: string; email: string; n: number }>();
    for (const r of mems ?? []) {
      const c = m.get(r.contact_id) ?? { name: r.contact_name ?? '', email: r.contact_email, n: 0 };
      c.n++;
      m.set(r.contact_id, c);
    }
    return [...m.entries()];
  }, [mems]);

  const list = useMemo(() => {
    const s = q.trim().toLowerCase();
    return (mems ?? []).filter((m) => (filter === null || m.contact_id === filter) && (!s || m.content.toLowerCase().includes(s)));
  }, [mems, filter, q]);

  const del = async (m: MemoryRow) => {
    if (!(await confirm({ title: '删除这条记忆？', body: '原始消息不受影响，只是之后起草时不再参考它。', okText: '删除', danger: true }))) return;
    try {
      await api.deleteMemory(m.id);
      setMems((l) => l?.filter((x) => x.id !== m.id) ?? null);
      toast('已删除记忆');
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), 'err');
    }
  };

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-6xl px-8 py-8">
        <PageHeader title="长期记忆"
          desc="选定会话里你发出的问答会转成向量。起草时先找这个客户的历史往来，再在全部记忆里找其他客户的类似问答——给 AI 前会隐藏价格、百分比和客户名。"
          action={<button className="btn-primary" onClick={() => setPickOpen(true)}><Icon name="plus" />记住一次对话</button>} />

        <div className="mb-6 grid grid-cols-3 gap-3">
          <Stat label="记忆条数" value={mems?.length ?? '—'} />
          <Stat label="涉及客户" value={mems ? contacts.length : '—'} />
          <Stat label="检索方式" value="bge-m3 向量" small />
        </div>

        <div className="flex gap-6">
          <aside className="w-56 shrink-0">
            <div className="section-title mb-2 px-2">按客户筛选</div>
            <div className="space-y-0.5">
              <FilterItem active={filter === null} onClick={() => setFilter(null)} label="全部客户" n={mems?.length ?? 0} />
              {contacts.map(([id, c]) => (
                <FilterItem key={id} active={filter === id} onClick={() => setFilter(id)} label={c.name} n={c.n} avatar />
              ))}
            </div>
          </aside>

          <div className="min-w-0 flex-1">
            <div className="relative mb-4">
              <Icon name="search" className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-ash" />
              <input className="field pl-8" placeholder="在记忆内容里搜索" value={q} onChange={(e) => setQ(e.target.value)} />
            </div>
            {mems === null ? (
              <div className="flex justify-center py-16 text-fog"><Spinner /></div>
            ) : !list.length ? (
              <EmptyState icon="memory" title={mems.length ? '没有匹配的记忆' : '暂无长期记忆'}
                hint={mems.length ? undefined : '点右上角「记住一次对话」，或在会话页打开「记住这次对话」。'} />
            ) : (
              <div className="space-y-3">
                {list.map((m) => (
                  <MemoryCard key={m.id} m={m} contact={{ name: m.contact_name ?? '', email: m.contact_email }} onDelete={() => del(m)} />
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
      <RememberModal open={pickOpen} onClose={() => setPickOpen(false)} onDone={load} />
    </div>
  );
}

function Stat({ label, value, small }: { label: string; value: string | number; small?: boolean }) {
  return (
    <div className="panel px-4 py-3">
      <div className="text-[11px] text-ash">{label}</div>
      <div className={clsx('mt-1 font-semibold text-bone', small ? 'text-sm' : 'text-2xl')}>{value}</div>
    </div>
  );
}

function FilterItem({ active, onClick, label, n, avatar }: { active: boolean; onClick: () => void; label: string; n: number; avatar?: boolean }) {
  return (
    <button onClick={onClick}
      className={clsx('flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-[13px] transition-colors',
        active ? 'bg-graphite text-bone' : 'text-fog hover:bg-obsidian hover:text-mist')}>
      {avatar ? <Avatar name={label} size="sm" /> : <span className="flex h-7 w-7 items-center justify-center"><Icon name="layers" className="h-3.5 w-3.5" /></span>}
      <span className="flex-1 truncate">{label}</span>
      <span className="text-[11px] text-ash">{n}</span>
    </button>
  );
}

function RememberModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const { toast } = useFeedback();
  const [convs, setConvs] = useState<Conversation[] | null>(null);
  const [sel, setSel] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setConvs(null);
    setSel(null);
    api.conversations().then(setConvs).catch((e) => toast(String(e.message ?? e), 'err'));
  }, [open, toast]);

  const go = async () => {
    if (!sel) return;
    setBusy(true);
    try {
      const r = await api.setRemember(sel, true);
      toast(`已记住会话 #${sel}：新存 ${r.saved} 条记忆${r.saved === 0 ? '（该会话还没有你发出的回复，或已全部存过）' : ''}`);
      onDone();
      onClose();
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), 'err');
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="记住一次对话" width="max-w-xl"
      footer={<>
        <button className="btn-ghost" onClick={onClose}>取消</button>
        <button className="btn-primary" disabled={!sel || busy} onClick={go}>{busy && <Spinner className="h-3.5 w-3.5" />}记住</button>
      </>}>
      <p className="mb-3 text-xs leading-relaxed text-fog">会把该会话中你已发出的回复（连同对应的客户来信）存为记忆，并对之后发出的回复持续生效。</p>
      {convs === null ? (
        <div className="flex justify-center py-8 text-fog"><Spinner /></div>
      ) : !convs.length ? (
        <p className="py-6 text-center text-sm text-ash">还没有会话</p>
      ) : (
        <div className="max-h-80 space-y-1 overflow-y-auto">
          {convs.map((c) => (
            <button key={c.id} onClick={() => setSel(c.id)}
              className={clsx('flex w-full items-center gap-3 rounded-lg border px-3 py-2 text-left transition-colors',
                sel === c.id ? 'border-lime/50 bg-lime/[0.06]' : 'border-transparent hover:bg-graphite')}>
              <Avatar name={c.contact_name} size="sm" />
              <span className="min-w-0 flex-1">
                <span className="flex items-center gap-2 text-[13px] text-mist">
                  #{c.id} {c.contact_name}
                  {c.remember && <Badge tone="lime">已在记忆中</Badge>}
                </span>
                <span className="block truncate text-xs text-ash">{c.last_message || '暂无消息'}</span>
              </span>
            </button>
          ))}
        </div>
      )}
    </Modal>
  );
}
