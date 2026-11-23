import { clsx } from 'clsx';
import { useEffect, useRef } from 'react';
import { PLACEHOLDER_RE, type Sources } from '../../lib/api';
import { Badge, Icon, Spinner } from '../../components/ui';
import { SourcesView } from './SourcesView';

export type Busy = null | 'customer' | 'regen' | 'compose' | 'polish' | 'send' | 'discard';

/** 占位符在列表里显示的短标签：去掉括号里的出处提示 */
function shortLabel(p: string) {
  const inner = p.replace(/^\[需业务员确认[:：]/, '').replace(/\]$/, '');
  return inner.split(/[（(]/)[0].trim() || inner;
}

export function DraftPanel({ text, setText, edited, sources, busy, canUndo, onSend, onPolish, onUndo, onRegen, onDiscard }: {
  text: string;
  setText: (s: string) => void;
  edited: boolean;
  sources?: Sources;
  busy: Busy;
  canUndo: boolean;
  onSend: () => void;
  onPolish: () => void;
  onUndo: () => void;
  onRegen: () => void;
  onDiscard: () => void;
}) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const placeholders = text.match(PLACEHOLDER_RE) ?? [];
  const warnings = sources?.warnings ?? [];

  // 高度跟随内容
  useEffect(() => {
    const t = ref.current;
    if (!t) return;
    t.style.height = 'auto';
    t.style.height = `${Math.min(Math.max(t.scrollHeight, 160), window.innerHeight * 0.45)}px`;
  }, [text]);

  /** 点占位符：在文本框里选中它，直接输入就能替换 */
  const jumpTo = (p: string) => {
    const t = ref.current;
    if (!t) return;
    const i = t.value.indexOf(p);
    if (i < 0) return;
    t.focus();
    t.setSelectionRange(i, i + p.length);
    // 粗略滚动到选区所在行
    const line = t.value.slice(0, i).split('\n').length;
    t.scrollTop = Math.max(0, (line - 3) * 22);
  };

  const working = busy !== null;

  return (
    <div className="anim-fade-up panel overflow-hidden border-lime/25">
      <div className="flex flex-wrap items-center gap-2 border-b border-smoke/60 bg-lime/[0.04] px-4 py-2.5">
        <span className="flex items-center gap-1.5 text-[13px] font-semibold text-lime"><Icon name="sparkle" className="h-4 w-4" />AI 草稿</span>
        <span className="text-xs text-fog">发送前请核对价格、交期和条款</span>
        <span className="ml-auto flex items-center gap-1.5">
          {edited && <Badge tone="gray">已修改</Badge>}
          {placeholders.length > 0
            ? <Badge tone="amber">待填 {placeholders.length} 项</Badge>
            : <Badge tone="pulse"><Icon name="check" className="h-3 w-3" />占位符已填完</Badge>}
        </span>
      </div>

      <div className="space-y-3 p-4">
        {warnings.length > 0 && (
          <ul className="space-y-1.5">
            {warnings.map((w, i) => (
              <li key={i} className="flex gap-2 rounded-lg border border-coral/30 bg-coral/[0.07] px-3 py-2 text-xs leading-relaxed text-mist">
                <Icon name="warn" className="mt-0.5 h-3.5 w-3.5 text-coral" />
                <span>{w}</span>
              </li>
            ))}
          </ul>
        )}

        {placeholders.length > 0 && (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-[11px] text-ash">点击定位并替换：</span>
            {placeholders.map((p, i) => (
              <button key={i} onClick={() => jumpTo(p)} title={p}
                className="max-w-[260px] truncate rounded-md border border-amber/40 bg-amber/10 px-2 py-0.5 text-[11px] text-amber transition-colors hover:bg-amber/20">
                {shortLabel(p)}
              </button>
            ))}
          </div>
        )}

        <textarea ref={ref} value={text} onChange={(e) => setText(e.target.value)} spellCheck={false} disabled={working}
          className="field resize-none font-sans text-[13.5px] leading-[22px]" />

        {sources && <SourcesView memories={sources.memories} kb={sources.kb} />}

        <div className="flex flex-wrap items-center gap-2 pt-1">
          <button className="btn-primary" onClick={onSend} disabled={working}>
            {busy === 'send' ? <Spinner className="h-3.5 w-3.5" /> : <Icon name="send" />}审核并发送
          </button>
          <button className="btn-ghost" onClick={onPolish} disabled={working || placeholders.length > 0}
            title={placeholders.length ? '先把所有占位符填好，再让 AI 润色' : '把填好的内容改写成通顺的完整回复；程序会核对数字，不一致就不替换'}>
            {busy === 'polish' ? <Spinner className="h-3.5 w-3.5" /> : <Icon name="wand" />}按填好的内容润色
          </button>
          {canUndo && (
            <button className="btn-quiet" onClick={onUndo} disabled={working}><Icon name="undo" />撤回润色</button>
          )}
          <span className="flex-1" />
          <button className={clsx('btn-quiet')} onClick={onRegen} disabled={working} title="丢弃当前草稿，按对话重新生成">
            {busy === 'regen' ? <Spinner className="h-3.5 w-3.5" /> : <Icon name="refresh" />}重新生成
          </button>
          <button className="btn-quiet hover:text-coral" onClick={onDiscard} disabled={working}><Icon name="trash" />丢弃</button>
        </div>
      </div>
    </div>
  );
}
