import { clsx } from 'clsx';
import { useCallback, useEffect, useRef, useState } from 'react';
import { api, ApiError, fmtTime, LANGUAGES, PLACEHOLDER_RE, type Conversation, type Memory, type Message, type Sources } from '../../lib/api';
import { Avatar, Badge, Drawer, EmptyState, Icon, Spinner, Toggle, useFeedback } from '../../components/ui';
import { MemoryCard } from '../../components/MemoryCard';
import { ConversationList } from './ConversationList';
import { Composer } from './Composer';
import { DraftPanel, type Busy } from './DraftPanel';

const BUSY_TEXT: Partial<Record<NonNullable<Busy>, string>> = {
  customer: 'AI 正在检索历史与资料，起草回复…',
  regen: 'AI 正在重新生成草稿…',
  compose: 'AI 正在结合对话、资料和你的语气起草…',
};

function loadCurrent(): number | null {
  try {
    const v = Number(localStorage.getItem('conv'));
    return v > 0 ? v : null;
  } catch {
    return null;
  }
}

export function ChatPage() {
  const { toast, confirm } = useFeedback();
  const [convs, setConvs] = useState<Conversation[]>([]);
  const [loadingList, setLoadingList] = useState(true);
  const [current, setCurrent] = useState<number | null>(loadCurrent);
  const [msgs, setMsgs] = useState<Message[]>([]);
  const [draft, setDraft] = useState<Message | null>(null);
  const [draftText, setDraftText] = useState('');
  const [prePolish, setPrePolish] = useState<string | null>(null);
  const [polishRefs, setPolishRefs] = useState<Memory[] | null>(null);
  const [sources, setSources] = useState<Record<number, Sources>>({});
  const [busy, setBusy] = useState<Busy>(null);
  const [memOpen, setMemOpen] = useState(false);
  const [contactMems, setContactMems] = useState<Memory[] | null>(null);
  const [remBusy, setRemBusy] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  const conv = convs.find((c) => c.id === current) ?? null;
  const edited = !!draft && draftText !== draft.content;

  const fail = useCallback((e: unknown) => toast(e instanceof Error ? e.message : String(e), 'err'), [toast]);

  const loadList = useCallback(async () => {
    try {
      setConvs(await api.conversations());
    } catch (e) {
      fail(e);
    } finally {
      setLoadingList(false);
    }
  }, [fail]);

  const loadMsgs = useCallback(async (id: number) => {
    const list = await api.messages(id);
    const d = list.find((m) => m.status === 'draft') ?? null;
    setMsgs(list.filter((m) => m.status === 'sent'));
    setDraft(d);
    setDraftText(d?.content ?? '');
    setPrePolish(null);
    setPolishRefs(null);
  }, []);

  useEffect(() => { loadList(); }, [loadList]);

  useEffect(() => {
    try {
      if (current) localStorage.setItem('conv', String(current));
    } catch {
      /* ignore */
    }
    setMsgs([]);
    setDraft(null);
    setContactMems(null);
    setMemOpen(false);
    if (current) loadMsgs(current).catch(fail);
  }, [current, loadMsgs, fail]);

  // 新消息 / 草稿出现时滚到底部
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
  }, [msgs.length, draft?.id, busy]);

  const applyResult = (id: number, r: Partial<Sources>) => {
    setSources((s) => ({ ...s, [id]: { memories: r.memories ?? [], kb: r.kb ?? [], warnings: r.warnings ?? [] } }));
  };

  /* ---------- 操作 ---------- */

  const createConv = async (name: string, email: string, language: string) => {
    try {
      const c = await api.createConversation(name, email, language);
      await loadList();
      setCurrent(c.id);
      toast(`已创建会话：${c.contact_name}`);
    } catch (e) {
      fail(e);
      throw e;
    }
  };

  const customerSend = async (content: string) => {
    if (!current) return false;
    setBusy('customer');
    // 先把客户来信显示出来，草稿生成要等几十秒
    setMsgs((l) => [...l, { id: -Date.now(), conversation_id: current, sender: 'customer', content, status: 'sent', created_at: new Date().toISOString() }]);
    try {
      const r = await api.customerMessage(current, content);
      await loadMsgs(current);
      loadList();
      if (r.draft_error) toast(`客户来信已保存，但草稿生成失败：${r.draft_error}`, 'err');
      else applyResult(current, r);
      return true;
    } catch (e) {
      fail(e);
      loadMsgs(current).catch(() => {});
      return false;
    } finally {
      setBusy(null);
    }
  };

  const compose = async (instruction: string) => {
    if (!current) return false;
    if (edited && !(await confirm({ title: '替换当前草稿？', body: '当前草稿已被你修改，AI 起草会替换它，改过的内容会丢失。', okText: '替换', danger: true }))) return false;
    setBusy('compose');
    try {
      const r = await api.compose(current, instruction);
      await loadMsgs(current);
      applyResult(current, r);
      return true;
    } catch (e) {
      fail(e);
      return false;
    } finally {
      setBusy(null);
    }
  };

  const regen = async () => {
    if (!current) return;
    if (edited && !(await confirm({ title: '重新生成草稿？', body: '草稿已被修改，重新生成会丢弃你改过的内容（包括已填的数字）。', okText: '重新生成', danger: true }))) return;
    setBusy('regen');
    try {
      const r = await api.regenerate(current);
      await loadMsgs(current);
      applyResult(current, r);
    } catch (e) {
      fail(e);
    } finally {
      setBusy(null);
    }
  };

  const send = async () => {
    if (!draft || !current) return;
    const left = (draftText.match(PLACEHOLDER_RE) ?? []).length;
    const nWarn = sources[current]?.warnings.length ?? 0;
    if (left > 0) {
      const ok = await confirm({
        title: `还有 ${left} 个占位符没填`,
        body: <>客户会直接看到 <code className="rounded bg-graphite px-1 text-amber">[需业务员确认：…]</code> 这样的文字。确定要这样发送吗？</>,
        okText: '仍然发送', danger: true,
      });
      if (!ok) return;
    } else if (nWarn > 0 && !edited) {
      const ok = await confirm({ title: '草稿有风险提醒', body: `这份草稿有 ${nWarn} 条风险提醒，而且你还没有修改过。确定直接发送吗？`, okText: '直接发送', danger: true });
      if (!ok) return;
    }
    setBusy('send');
    try {
      await api.send(draft.id, draftText);
      setSources((s) => {
        const n = { ...s };
        delete n[current];
        return n;
      });
      await loadMsgs(current);
      loadList();
      toast(conv?.remember ? '已发送，并存入记忆' : '已发送');
    } catch (e) {
      fail(e);
    } finally {
      setBusy(null);
    }
  };

  const discard = async () => {
    if (!draft || !current) return;
    if (edited && !(await confirm({ title: '丢弃草稿？', body: '你改过的内容会丢失。', okText: '丢弃', danger: true }))) return;
    setBusy('discard');
    try {
      await api.discard(draft.id);
      await loadMsgs(current);
    } catch (e) {
      fail(e);
    } finally {
      setBusy(null);
    }
  };

  const polish = async () => {
    if (!draft) return;
    setBusy('polish');
    try {
      const r = await api.polish(draft.id, draftText);
      setPrePolish(draftText);
      setDraftText(r.content);
      setPolishRefs(r.references ?? []);
      const n = r.references?.length ?? 0;
      toast(`已润色${n ? `，参考了 ${n} 条你过去的回答` : '（没有找到类似的历史回答，按通用写法润色）'}；${r.numbers_checked} 个数字核对一致${r.attempts > 1 ? `（第 ${r.attempts} 次通过）` : ''}。请再读一遍后发送。`);
    } catch (e) {
      if (e instanceof ApiError) {
        const b = e.body as { placeholders?: string[]; missing?: string[]; added?: string[]; added_terms?: string[]; added_denial?: boolean };
        let msg = e.message;
        if (b.placeholders?.length) msg += '：\n' + b.placeholders.join('\n');
        if (b.missing?.length) msg += `\n润色后缺少/改动的数字：${b.missing.join(', ')}`;
        if (b.added?.length) msg += `\n润色后多出的数字：${b.added.join(', ')}`;
        if (b.added_terms?.length) msg += `\n润色后多出了原稿没有的条款：${b.added_terms.join(', ')}（可能是从过去的回答里搬来的）`;
        if (b.added_denial) msg += '\n润色后多出了否定说法（例如"我们没有"）';
        toast(msg, 'err');
      } else fail(e);
    } finally {
      setBusy(null);
    }
  };

  const undoPolish = () => {
    if (prePolish !== null) setDraftText(prePolish);
    setPrePolish(null);
    setPolishRefs(null);
    toast('已恢复润色前的内容', 'info');
  };

  const toggleRemember = async (on: boolean) => {
    if (!current) return;
    setRemBusy(true);
    try {
      const r = await api.setRemember(current, on);
      await loadList();
      toast(on ? `已开启「记住这次对话」，新存 ${r.saved} 条记忆` : '已关闭：之后发出的回复不再存为记忆（已存的保留）', on ? 'ok' : 'info');
      if (memOpen) setContactMems(await api.contactMemories(current));
    } catch (e) {
      fail(e);
    } finally {
      setRemBusy(false);
    }
  };

  const openMemories = async () => {
    if (!current) return;
    setMemOpen(true);
    setContactMems(null);
    try {
      setContactMems(await api.contactMemories(current));
    } catch (e) {
      fail(e);
    }
  };

  const deleteMemory = async (m: Memory) => {
    if (!(await confirm({ title: '删除这条记忆？', body: '原始消息不受影响，只是之后起草时不再参考它。', okText: '删除', danger: true }))) return;
    try {
      await api.deleteMemory(m.id);
      setContactMems((l) => l?.filter((x) => x.id !== m.id) ?? null);
      toast('已删除记忆');
    } catch (e) {
      fail(e);
    }
  };

  /* ---------- 渲染 ---------- */

  return (
    <div className="flex h-full">
      <ConversationList convs={convs} loading={loadingList} current={current} onSelect={setCurrent} onCreate={createConv} />

      {!conv ? (
        <div className="flex flex-1 items-center justify-center">
          <EmptyState icon="chat" title={convs.length ? '选择一个会话' : '从新建会话开始'}
            hint="输入客户来信，AI 参考这个客户的历史往来和知识库起草，关键数据留成占位符给你填；填好后润色，AI 会参考你过去回答类似问题的写法。" />
        </div>
      ) : (
        <section className="flex min-w-0 flex-1 flex-col">
          <header className="flex h-14 shrink-0 items-center gap-3 border-b border-smoke/60 px-6">
            <Avatar name={conv.contact_name} />
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold text-bone">{conv.contact_name}</div>
              <div className="flex items-center gap-1.5 text-[11px] text-ash">
                <Icon name="globe" className="h-3 w-3" />{LANGUAGES[conv.language] ?? conv.language} · 会话 #{conv.id}
              </div>
            </div>
            <div className="ml-auto flex items-center gap-4">
              <span title="打开后，本会话已发出和之后发出的回复会存入记忆：给这个客户起草时保持说法一致；润色任何客户的类似问题时，参考你的语气和回答方式。">
                <Toggle checked={conv.remember} disabled={remBusy} onChange={toggleRemember}
                  label={<span className="flex items-center gap-1.5 text-xs text-mist">{remBusy && <Spinner className="h-3 w-3" />}记住这次对话</span>} />
              </span>
              <button className="btn-ghost px-3 py-1.5 text-xs" onClick={openMemories}><Icon name="memory" className="h-3.5 w-3.5" />客户记忆</button>
            </div>
          </header>

          <div ref={scrollRef} className="flex-1 overflow-y-auto px-6 py-5">
            <div className="mx-auto max-w-3xl space-y-4">
              {!msgs.length && !draft && !busy && (
                <EmptyState icon="mail" title="还没有往来消息" hint="在下方「模拟客户来信」输入客户的消息，AI 会自动起草回复；也可以直接用「AI 帮我写」主动给客户写信。" />
              )}
              {msgs.map((m) => <Bubble key={m.id} m={m} name={conv.contact_name} />)}

              {busy && BUSY_TEXT[busy] && (
                <div className="anim-fade-up panel relative overflow-hidden px-4 py-4">
                  <div className="shimmer absolute inset-0" />
                  <div className="relative flex items-center gap-2.5 text-sm text-lime"><Spinner className="h-4 w-4" />{BUSY_TEXT[busy]}</div>
                  <p className="relative mt-1.5 text-xs text-ash">本地模型通常需要 20–60 秒。</p>
                </div>
              )}

              {draft && !(busy && BUSY_TEXT[busy]) && (
                <DraftPanel text={draftText} setText={setDraftText} edited={edited} sources={sources[conv.id]} busy={busy}
                  canUndo={prePolish !== null} polishRefs={polishRefs} onSend={send} onPolish={polish} onUndo={undoPolish} onRegen={regen} onDiscard={discard} />
              )}
            </div>
          </div>

          <Composer busy={busy} onCompose={compose} onCustomer={customerSend} />
        </section>
      )}

      <Drawer open={memOpen} onClose={() => setMemOpen(false)} title={`${conv?.contact_name ?? ''} 的记忆`}
        subtitle="这个客户过去会话里记住的问答：起草时用来保持说法一致，润色时用来模仿你的写法（只有打开了「记住这次对话」的会话才会存）。">
        {contactMems === null ? (
          <div className="flex justify-center py-10 text-fog"><Spinner /></div>
        ) : !contactMems.length ? (
          <EmptyState icon="memory" title="这个客户还没有记忆" hint="打开顶部的「记住这次对话」，你发出的回复会存到这里。" />
        ) : (
          <div className="space-y-3">
            {contactMems.map((m) => <MemoryCard key={m.id} m={m} onDelete={() => deleteMemory(m)} />)}
          </div>
        )}
      </Drawer>
    </div>
  );
}

function Bubble({ m, name }: { m: Message; name: string }) {
  const agent = m.sender === 'agent';
  return (
    <div className={clsx('anim-fade-up flex gap-2.5', agent && 'flex-row-reverse')}>
      {agent ? (
        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-lime/15 text-lime"><Icon name="user" className="h-3.5 w-3.5" /></span>
      ) : (
        <Avatar name={name} size="sm" />
      )}
      <div className={clsx('flex max-w-[78%] flex-col', agent && 'items-end')}>
        <div className={clsx('whitespace-pre-wrap break-words rounded-2xl px-4 py-2.5 text-[13.5px] leading-relaxed',
          agent ? 'rounded-tr-md border border-lime/20 bg-lime/[0.08] text-bone' : 'rounded-tl-md border border-smoke/60 bg-obsidian text-mist')}>
          {m.content}
        </div>
        <span className="mt-1 flex items-center gap-1.5 px-1 text-[10px] text-ash">
          {agent ? '我' : '客户'} · {fmtTime(m.created_at)}
          {agent && <Badge tone="gray" className="py-0 text-[9px]">已发送</Badge>}
        </span>
      </div>
    </div>
  );
}
