import { clsx } from 'clsx';
import { useRef, useState } from 'react';
import { Icon, Spinner } from '../../components/ui';
import type { Busy } from './DraftPanel';

const CHIPS = [
  { label: '跟进 follow up', text: 'Follow up' },
  { label: '提醒确认报价', text: '礼貌提醒客户确认上次的报价' },
  { label: '样品已寄出', text: '告诉客户样品已寄出，快递单号：' },
  { label: '感谢下单', text: '感谢客户下单，确认订单细节' },
];

type Mode = 'ai' | 'customer';

export function Composer({ busy, onCompose, onCustomer }: {
  busy: Busy;
  onCompose: (instruction: string) => Promise<boolean>;
  onCustomer: (content: string) => Promise<boolean>;
}) {
  const [mode, setMode] = useState<Mode>('ai');
  const [instr, setInstr] = useState('');
  const [cust, setCust] = useState('');
  const ref = useRef<HTMLTextAreaElement>(null);
  const working = busy !== null;

  const submit = async () => {
    if (mode === 'ai') {
      if (!instr.trim()) return;
      if (await onCompose(instr.trim())) setInstr('');
    } else {
      if (!cust.trim()) return;
      if (await onCustomer(cust.trim())) setCust('');
    }
  };

  const chip = (t: string) => {
    setInstr(t);
    requestAnimationFrame(() => {
      ref.current?.focus();
      ref.current?.setSelectionRange(t.length, t.length);
    });
  };

  return (
    <div className="border-t border-smoke/60 bg-carbon/80 px-6 pb-4 pt-3">
      <div className="mb-2.5 flex items-center gap-1.5">
        <TabButton active={mode === 'ai'} onClick={() => setMode('ai')} icon="sparkle" tone="lime">AI 帮我写</TabButton>
        <TabButton active={mode === 'customer'} onClick={() => setMode('customer')} icon="mail" tone="teal">模拟客户来信</TabButton>
        {mode === 'ai' && (
          <div className="ml-3 flex flex-wrap gap-1.5">
            {CHIPS.map((c) => (
              <button key={c.label} onClick={() => chip(c.text)} disabled={working}
                className="rounded-full border border-smoke px-2.5 py-0.5 text-[11px] text-fog transition-colors hover:border-lime/50 hover:text-lime disabled:opacity-40">
                {c.label}
              </button>
            ))}
          </div>
        )}
      </div>
      <div className={clsx('flex items-end gap-2 rounded-xl border bg-graphite p-2 transition-colors focus-within:border-lime/50',
        mode === 'ai' ? 'border-smoke' : 'border-teal/30')}>
        {mode === 'ai' ? (
          <textarea ref={ref} rows={2} value={instr} onChange={(e) => setInstr(e.target.value)} disabled={working}
            onKeyDown={(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); submit(); } }}
            placeholder="一句指示（follow up / 催一下付款 / 告诉他 DHL 单号 123）或一段粗稿都行，AI 会结合对话、资料和你的语气起草"
            className="max-h-40 min-h-[44px] flex-1 resize-y bg-transparent px-2 py-1.5 text-sm text-bone outline-none placeholder:text-ash" />
        ) : (
          <textarea rows={2} value={cust} onChange={(e) => setCust(e.target.value)} disabled={working}
            onKeyDown={(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); submit(); } }}
            placeholder="以客户身份输入一条来信（测试用，也可以粘贴真实客户邮件），发送后 AI 会自动起草回复"
            className="max-h-40 min-h-[44px] flex-1 resize-y bg-transparent px-2 py-1.5 text-sm text-bone outline-none placeholder:text-ash" />
        )}
        <button className={mode === 'ai' ? 'btn-primary' : 'btn bg-teal font-semibold text-void hover:brightness-110'}
          onClick={submit} disabled={working || !(mode === 'ai' ? instr.trim() : cust.trim())}>
          {(busy === 'compose' && mode === 'ai') || (busy === 'customer' && mode === 'customer')
            ? <Spinner className="h-3.5 w-3.5" />
            : <Icon name={mode === 'ai' ? 'sparkle' : 'send'} />}
          {mode === 'ai' ? 'AI 起草' : '发送客户来信'}
        </button>
      </div>
      <p className="mt-1.5 text-[11px] text-ash">
        {mode === 'ai' ? '⌘/Ctrl + Enter 起草 · 不需要客户先来信' : '⌘/Ctrl + Enter 发送 · 当前没有接入邮箱，用这里模拟客户发来的消息'}
      </p>
    </div>
  );
}

function TabButton({ active, onClick, icon, tone, children }: { active: boolean; onClick: () => void; icon: 'sparkle' | 'mail'; tone: 'lime' | 'teal'; children: string }) {
  return (
    <button onClick={onClick}
      className={clsx('flex items-center gap-1.5 whitespace-nowrap rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors',
        active
          ? tone === 'lime' ? 'border-lime/50 bg-lime/10 text-lime' : 'border-teal/50 bg-teal/10 text-teal'
          : 'border-smoke text-fog hover:border-fog hover:text-mist')}>
      <Icon name={icon} className="h-3.5 w-3.5" />{children}
    </button>
  );
}
