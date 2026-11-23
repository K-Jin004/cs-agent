import { clsx } from 'clsx';
import { useState } from 'react';
import type { KBHit, Memory } from '../../lib/api';
import { Badge, Icon } from '../../components/ui';

type Tab = 'own' | 'other' | 'kb';

/** 草稿参考了哪些资料：本客户历史 / 其他客户类似问答 / 知识库 */
export function SourcesView({ memories, kb }: { memories: Memory[]; kb: KBHit[] }) {
  const own = memories.filter((m) => !m.other_customer);
  const other = memories.filter((m) => m.other_customer);
  const tabs: { id: Tab; label: string; n: number }[] = [
    { id: 'own', label: '本客户历史', n: own.length },
    { id: 'other', label: '其他客户类似问答', n: other.length },
    { id: 'kb', label: '知识库', n: kb.length },
  ];
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<Tab>(own.length ? 'own' : other.length ? 'other' : 'kb');
  const total = memories.length + kb.length;

  if (!total) {
    return <p className="flex items-center gap-1.5 text-xs text-ash"><Icon name="info" className="h-3.5 w-3.5" />没有找到相关的历史或资料，草稿里不确定的内容都写成了占位符。</p>;
  }

  return (
    <div className="rounded-lg border border-smoke/60 bg-carbon/60">
      <button className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs text-fog hover:text-mist" onClick={() => setOpen(!open)}>
        <Icon name="chevron" className={clsx('h-3.5 w-3.5 transition-transform', open && 'rotate-90')} />
        <Icon name="layers" className="h-3.5 w-3.5" />
        参考来源
        <span className="flex gap-1.5">
          {own.length > 0 && <Badge tone="lime">本客户 {own.length}</Badge>}
          {other.length > 0 && <Badge tone="violet">类似问答 {other.length}</Badge>}
          {kb.length > 0 && <Badge tone="teal">资料 {kb.length}</Badge>}
        </span>
      </button>
      {open && (
        <div className="border-t border-smoke/60 px-3 pb-3 pt-2">
          <div className="mb-2 flex gap-1">
            {tabs.map((t) => (
              <button key={t.id} disabled={!t.n} onClick={() => setTab(t.id)}
                className={clsx('rounded-md px-2.5 py-1 text-xs transition-colors disabled:opacity-30',
                  tab === t.id ? 'bg-graphite text-bone' : 'text-fog hover:text-mist')}>
                {t.label} {t.n}
              </button>
            ))}
          </div>
          <div className="max-h-64 space-y-2 overflow-y-auto">
            {tab === 'own' && own.map((m) => (
              <SourceCard key={m.id} meta={`会话 #${m.conversation_id} · 相似度 ${sim(m.distance)}`} text={m.content} />
            ))}
            {tab === 'other' && (
              <>
                <p className="text-[11px] text-ash">给 AI 时价格、百分比和客户名已隐藏；其中的商务条款只会以占位符提示出现。</p>
                {other.map((m) => (
                  <SourceCard key={m.id} meta={`${m.contact_name} · 会话 #${m.conversation_id} · 相似度 ${sim(m.distance)}`} text={m.content} />
                ))}
              </>
            )}
            {tab === 'kb' && kb.map((h) => (
              <SourceCard key={h.id} meta={`《${h.doc_title}》第 ${h.seq + 1} 段 · ${h.via ?? ''} · 相似度 ${sim(h.distance)}`} text={h.content} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function sim(distance: number) {
  return `${Math.round((1 - distance) * 100)}%`;
}

function SourceCard({ meta, text }: { meta: string; text: string }) {
  return (
    <div className="rounded-lg border border-smoke/50 bg-obsidian px-3 py-2">
      <div className="mb-1 text-[11px] text-ash">{meta}</div>
      <div className="whitespace-pre-wrap text-xs leading-relaxed text-mist">{text}</div>
    </div>
  );
}
