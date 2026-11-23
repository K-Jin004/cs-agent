import { clsx } from 'clsx';
import { useCallback, useEffect, useRef, useState } from 'react';
import { api, fmtDate, type KBDocument, type KBHit } from '../lib/api';
import { Badge, Drawer, EmptyState, Icon, Modal, PageHeader, Spinner, useFeedback } from '../components/ui';

export function KBPage() {
  const { toast, confirm } = useFeedback();
  const [docs, setDocs] = useState<KBDocument[] | null>(null);
  const [newOpen, setNewOpen] = useState(false);
  const [detail, setDetail] = useState<KBDocument | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      setDocs(await api.kbDocuments());
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), 'err');
      setDocs([]);
    }
  }, [toast]);

  useEffect(() => { load(); }, [load]);

  const show = async (id: number) => {
    setDetail(null);
    setDetailOpen(true);
    try {
      setDetail(await api.kbDocument(id));
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), 'err');
    }
  };

  const del = async (d: KBDocument) => {
    if (!(await confirm({ title: `删除《${d.title}》？`, body: `会同时删除它的 ${d.chunks} 个分块，之后起草不再参考这份资料。`, okText: '删除', danger: true }))) return;
    try {
      await api.deleteKBDocument(d.id);
      toast(`已删除《${d.title}》`);
      load();
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), 'err');
    }
  };

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-6xl px-8 py-8">
        <PageHeader title="知识库"
          desc="公司事实资料（产品规格、认证、公司介绍等），所有客户共用。切块后用「向量 + 关键词」混合检索（RRF 融合），作为草稿核对产品事实的依据。"
          action={<button className="btn-primary" onClick={() => setNewOpen(true)}><Icon name="plus" />新建资料</button>} />

        {docs === null ? (
          <div className="flex justify-center py-16 text-fog"><Spinner /></div>
        ) : !docs.length ? (
          <div className="panel">
            <EmptyState icon="book" title="知识库还是空的" hint="上传产品规格书、公司介绍等 .txt / .md 资料，或直接粘贴文本。"
              action={<button className="btn-primary" onClick={() => setNewOpen(true)}><Icon name="plus" />新建资料</button>} />
          </div>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-3">
            {docs.map((d) => (
              <div key={d.id} className="anim-fade-up group panel flex flex-col p-4 transition-colors hover:border-smoke">
                <div className="flex items-start gap-3">
                  <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-teal/10 text-teal"><Icon name="doc" /></span>
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium text-bone" title={d.title}>{d.title}</div>
                    <div className="mt-0.5 truncate text-[11px] text-ash">{d.source || '粘贴'} · {fmtDate(d.created_at)}</div>
                  </div>
                  <Badge tone="pulse">已索引</Badge>
                </div>
                <div className="mt-4 flex items-center gap-3 text-xs text-fog">
                  <span>{d.chars.toLocaleString()} 字</span>
                  <span className="h-1 w-1 rounded-full bg-smoke" />
                  <span>{d.chunks} 个分块</span>
                  <span className="flex-1" />
                  <button className="btn-quiet px-2 py-1 text-xs" onClick={() => show(d.id)}>查看分块</button>
                  <button className="btn-quiet px-2 py-1 text-xs hover:text-coral" onClick={() => del(d)} aria-label="删除"><Icon name="trash" className="h-3.5 w-3.5" /></button>
                </div>
              </div>
            ))}
          </div>
        )}

        <SearchTest />
      </div>

      <NewDocModal open={newOpen} onClose={() => setNewOpen(false)} onDone={load} />
      <Drawer open={detailOpen} onClose={() => setDetailOpen(false)} title={detail ? `《${detail.title}》` : '加载中…'}
        subtitle={detail ? `${detail.chunks} 个分块 · ${detail.chars.toLocaleString()} 字 · 每块约 800 字，相邻块重叠 100 字` : undefined}>
        {!detail ? (
          <div className="flex justify-center py-10 text-fog"><Spinner /></div>
        ) : (
          <div className="space-y-3">
            {detail.chunk_list?.map((c) => (
              <div key={c.id} className="panel p-3">
                <div className="mb-1.5 text-[11px] text-ash">第 {c.seq + 1} 块 · {c.content.length} 字</div>
                <div className="whitespace-pre-wrap text-[13px] leading-relaxed text-mist">{c.content}</div>
              </div>
            ))}
          </div>
        )}
      </Drawer>
    </div>
  );
}

function SearchTest() {
  const [q, setQ] = useState('');
  const [hits, setHits] = useState<KBHit[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const run = async () => {
    if (!q.trim()) return;
    setBusy(true);
    setErr('');
    try {
      setHits(await api.kbSearch(q.trim()));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-10">
      <div className="mb-3 flex items-center gap-2">
        <Icon name="search" className="text-fog" />
        <h2 className="text-[15px] font-semibold text-bone">检索测试</h2>
        <span className="text-xs text-ash">输入一句客户可能问的话，看会命中哪些资料</span>
      </div>
      <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); run(); }}>
        <input className="field" placeholder="例如：Is the 60x60 panel IP65 rated?" value={q} onChange={(e) => setQ(e.target.value)} />
        <button className="btn-ghost" disabled={busy || !q.trim()}>{busy ? <Spinner className="h-3.5 w-3.5" /> : <Icon name="search" />}检索</button>
      </form>
      {err && <p className="mt-3 text-sm text-coral">{err}</p>}
      {hits && (
        <div className="mt-4 space-y-2">
          {!hits.length ? (
            <p className="rounded-lg border border-smoke/60 bg-obsidian px-4 py-3 text-sm text-fog">没有足够相关的资料——草稿会把相关内容写成占位符。</p>
          ) : hits.map((h, i) => (
            <div key={h.id} className="anim-fade-up panel p-3">
              <div className="mb-1.5 flex flex-wrap items-center gap-2 text-[11px] text-ash">
                <span className="font-semibold text-lime">#{i + 1}</span>
                <span className="text-mist">《{h.doc_title}》第 {h.seq + 1} 块</span>
                {h.via && <Badge tone={h.via.includes('+') ? 'lime' : 'teal'}>{h.via}</Badge>}
                <span>向量距离 {h.distance.toFixed(3)}</span>
                <span>RRF {h.score.toFixed(4)}</span>
              </div>
              <div className="whitespace-pre-wrap text-[13px] leading-relaxed text-mist">{h.content}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function NewDocModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const { toast } = useFeedback();
  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');
  const [source, setSource] = useState('');
  const [drag, setDrag] = useState(false);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const readFile = (f: File) => {
    if (f.size > 1024 * 1024) {
      toast('文件超过 1MB，请拆分后再上传', 'err');
      return;
    }
    const r = new FileReader();
    r.onload = () => {
      setContent(String(r.result));
      setSource(f.name);
      setTitle((t) => t || f.name.replace(/\.[^.]+$/, ''));
    };
    r.readAsText(f, 'utf-8');
  };

  const save = async () => {
    if (!title.trim() || !content.trim()) return;
    setBusy(true);
    try {
      const d = await api.createKBDocument(title.trim(), content.trim(), source || '粘贴');
      toast(`已保存《${d.title}》：${d.chunks} 个分块已建立索引`);
      setTitle('');
      setContent('');
      setSource('');
      onDone();
      onClose();
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), 'err');
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="新建资料" width="max-w-2xl"
      footer={<>
        <span className="mr-auto self-center text-[11px] text-ash">只放可以对客户说的事实；价格表也可以放，但草稿里价格仍会写成占位符。</span>
        <button className="btn-ghost" onClick={onClose}>取消</button>
        <button className="btn-primary" disabled={!title.trim() || !content.trim() || busy} onClick={save}>
          {busy && <Spinner className="h-3.5 w-3.5" />}{busy ? '切块并向量化…' : '保存并建立索引'}
        </button>
      </>}>
      <div className="space-y-4">
        <div>
          <label className="field-label">标题 *</label>
          <input className="field" placeholder="例如 IP65 面板规格书" value={title} onChange={(e) => setTitle(e.target.value)} />
        </div>
        <div
          onDragOver={(e) => { e.preventDefault(); setDrag(true); }}
          onDragLeave={() => setDrag(false)}
          onDrop={(e) => { e.preventDefault(); setDrag(false); const f = e.dataTransfer.files[0]; if (f) readFile(f); }}
          onClick={() => fileRef.current?.click()}
          className={clsx('flex cursor-pointer flex-col items-center gap-1.5 rounded-xl border border-dashed px-4 py-5 text-center transition-colors',
            drag ? 'border-lime/60 bg-lime/[0.05]' : 'border-smoke hover:border-fog')}>
          <Icon name="upload" className="h-5 w-5 text-fog" />
          <span className="text-sm text-mist">{source ? `已读取：${source}` : '拖入文件，或点击选择'}</span>
          <span className="text-[11px] text-ash">支持 .txt / .md / .csv，最大 1MB</span>
          <input ref={fileRef} type="file" className="hidden" accept=".txt,.md,.markdown,.csv,text/plain,text/markdown"
            onChange={(e) => { const f = e.target.files?.[0]; if (f) readFile(f); e.target.value = ''; }} />
        </div>
        <div>
          <label className="field-label">内容 *</label>
          <textarea className="field h-56 resize-y font-mono text-xs leading-relaxed" placeholder="或者直接粘贴资料（支持 # 标题分段）"
            value={content} onChange={(e) => setContent(e.target.value)} />
          <div className="mt-1 text-right text-[11px] text-ash">{content.length.toLocaleString()} 字</div>
        </div>
      </div>
    </Modal>
  );
}
