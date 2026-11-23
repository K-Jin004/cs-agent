// 通用 UI：图标、按钮状态、徽标、开关、弹窗、侧滑抽屉、提示条、确认框。
import { clsx } from 'clsx';
import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';

/* ---------------- 图标（内联 SVG，统一 1.6 线宽） ---------------- */

const PATHS = {
  chat: 'M4 5h16v11H8l-4 4V5z',
  memory: 'M12 3a6 6 0 0 0-6 6c0 2.2 1.2 3.6 2.4 4.6.8.7 1.1 1.5 1.1 2.4V17h5v-1c0-.9.3-1.7 1.1-2.4C16.8 12.6 18 11.2 18 9a6 6 0 0 0-6-6zM9.5 20h5',
  book: 'M5 4h9a3 3 0 0 1 3 3v13H8a3 3 0 0 1-3-3V4zM5 17a3 3 0 0 1 3-3h9',
  plus: 'M12 5v14M5 12h14',
  search: 'M11 4a7 7 0 1 1 0 14 7 7 0 0 1 0-14zM20 20l-4-4',
  sparkle: 'M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8L12 3zM19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8L19 15z',
  send: 'M4 12l16-8-6 16-2.5-6.5L4 12z',
  refresh: 'M20 11a8 8 0 0 0-14.3-4.9L4 8M4 4v4h4M4 13a8 8 0 0 0 14.3 4.9L20 16M20 20v-4h-4',
  trash: 'M5 7h14M10 7V4h4v3M7 7l1 13h8l1-13',
  undo: 'M9 14L4 9l5-5M4 9h10a6 6 0 0 1 0 12h-3',
  wand: 'M4 20L15 9M14 4v2M18 6l-1.5 1.5M20 10h-2M12 8h2v2',
  warn: 'M12 4l9 16H3l9-16zM12 10v4M12 17v.5',
  x: 'M6 6l12 12M18 6L6 18',
  user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0',
  upload: 'M12 16V4M7 9l5-5 5 5M4 16v4h16v-4',
  doc: 'M6 3h8l4 4v14H6V3zM14 3v4h4M9 12h6M9 16h6',
  check: 'M5 12l5 5 9-10',
  info: 'M12 8v.5M12 11v5M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18z',
  globe: 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18zM3 12h18M12 3c2.5 2.5 3.5 5.5 3.5 9s-1 6.5-3.5 9c-2.5-2.5-3.5-5.5-3.5-9s1-6.5 3.5-9z',
  chevron: 'M9 6l6 6-6 6',
  layers: 'M12 3l9 5-9 5-9-5 9-5zM3 13l9 5 9-5',
  mail: 'M3 6h18v12H3V6zM3 7l9 6 9-6',
} as const;

export type IconName = keyof typeof PATHS;

export function Icon({ name, className }: { name: IconName; className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round"
      className={clsx('shrink-0', !/\bh-/.test(className ?? '') && 'h-4 w-4', className)} aria-hidden>
      <path d={PATHS[name]} />
    </svg>
  );
}

/* ---------------- 小部件 ---------------- */

export function Spinner({ className }: { className?: string }) {
  return <span className={clsx('inline-block animate-spin rounded-full border-2 border-current border-t-transparent', className ?? 'h-4 w-4')} aria-label="加载中" />;
}

type Tone = 'lime' | 'teal' | 'coral' | 'amber' | 'pulse' | 'violet' | 'gray';
const TONE: Record<Tone, string> = {
  lime: 'border-lime/40 text-lime bg-lime/10',
  teal: 'border-teal/40 text-teal bg-teal/10',
  coral: 'border-coral/40 text-coral bg-coral/10',
  amber: 'border-amber/40 text-amber bg-amber/10',
  pulse: 'border-pulse/40 text-pulse bg-pulse/10',
  violet: 'border-violet/40 text-violet bg-violet/10',
  gray: 'border-smoke text-fog bg-graphite',
};

export function Badge({ tone = 'gray', children, className }: { tone?: Tone; children: ReactNode; className?: string }) {
  return (
    <span className={clsx('inline-flex items-center gap-1 whitespace-nowrap rounded-full border px-2 py-0.5 text-[11px] font-medium', TONE[tone], className)}>
      {children}
    </span>
  );
}

export function Toggle({ checked, onChange, disabled, label }: { checked: boolean; onChange: (v: boolean) => void; disabled?: boolean; label?: ReactNode }) {
  return (
    <label className={clsx('inline-flex select-none items-center gap-2 text-sm', disabled ? 'opacity-50' : 'cursor-pointer')}>
      <button type="button" role="switch" aria-checked={checked} disabled={disabled} onClick={() => onChange(!checked)}
        className={clsx('relative h-5 w-9 rounded-full transition-colors duration-200', checked ? 'bg-lime' : 'bg-smoke')}>
        <span className={clsx('absolute top-0.5 h-4 w-4 rounded-full transition-all duration-200', checked ? 'left-[18px] bg-void' : 'left-0.5 bg-mist')} />
      </button>
      {label}
    </label>
  );
}

const AVATAR_TONES = ['bg-lime/15 text-lime', 'bg-teal/15 text-teal', 'bg-violet/15 text-violet', 'bg-amber/15 text-amber', 'bg-coral/15 text-coral', 'bg-pulse/15 text-pulse'];

export function Avatar({ name, size = 'md' }: { name: string; size?: 'sm' | 'md' | 'lg' }) {
  const initials = name.replace(/\(.*?\)/g, '').trim().split(/\s+/).map((w) => w[0]).join('').slice(0, 2).toUpperCase() || '?';
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return (
    <span className={clsx('inline-flex shrink-0 items-center justify-center rounded-full font-semibold', AVATAR_TONES[h % AVATAR_TONES.length],
      size === 'sm' && 'h-7 w-7 text-[11px]', size === 'md' && 'h-9 w-9 text-xs', size === 'lg' && 'h-10 w-10 text-sm')}>
      {initials}
    </span>
  );
}

export function EmptyState({ icon, title, hint, action }: { icon: IconName; title: string; hint?: ReactNode; action?: ReactNode }) {
  return (
    <div className="anim-fade-up flex flex-col items-center justify-center gap-2 px-6 py-16 text-center">
      <span className="mb-1 flex h-12 w-12 items-center justify-center rounded-2xl border border-smoke/60 bg-obsidian text-fog">
        <Icon name={icon} className="h-5 w-5" />
      </span>
      <p className="text-sm font-medium text-mist">{title}</p>
      {hint && <p className="max-w-sm text-xs leading-relaxed text-ash">{hint}</p>}
      {action && <div className="mt-3">{action}</div>}
    </div>
  );
}

export function PageHeader({ title, desc, action }: { title: string; desc?: ReactNode; action?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div className="min-w-0">
        <h1 className="text-xl font-semibold text-bone">{title}</h1>
        {desc && <p className="mt-1.5 max-w-2xl text-[13px] leading-relaxed text-fog">{desc}</p>}
      </div>
      {action}
    </div>
  );
}

/* ---------------- 弹窗与抽屉 ---------------- */

function useEsc(onClose: () => void) {
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', h);
    return () => window.removeEventListener('keydown', h);
  }, [onClose]);
}

export function Modal({ open, onClose, title, children, footer, width = 'max-w-lg' }: {
  open: boolean; onClose: () => void; title: ReactNode; children: ReactNode; footer?: ReactNode; width?: string;
}) {
  if (!open) return null;
  return <ModalInner onClose={onClose} title={title} footer={footer} width={width}>{children}</ModalInner>;
}

function ModalInner({ onClose, title, children, footer, width }: { onClose: () => void; title: ReactNode; children: ReactNode; footer?: ReactNode; width: string }) {
  useEsc(onClose);
  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-void/70 p-4 backdrop-blur-sm" onMouseDown={onClose}>
      <div className={clsx('anim-pop panel flex max-h-[90vh] w-full flex-col shadow-2xl shadow-black/50', width)} onMouseDown={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between border-b border-smoke/60 px-5 py-3.5">
          <h2 className="text-[15px] font-semibold text-bone">{title}</h2>
          <button className="btn-quiet -mr-2 px-2" onClick={onClose} aria-label="关闭"><Icon name="x" /></button>
        </div>
        <div className="overflow-y-auto px-5 py-4">{children}</div>
        {footer && <div className="flex justify-end gap-2 border-t border-smoke/60 px-5 py-3">{footer}</div>}
      </div>
    </div>
  );
}

export function Drawer({ open, onClose, title, subtitle, children }: { open: boolean; onClose: () => void; title: ReactNode; subtitle?: ReactNode; children: ReactNode }) {
  if (!open) return null;
  return <DrawerInner onClose={onClose} title={title} subtitle={subtitle}>{children}</DrawerInner>;
}

function DrawerInner({ onClose, title, subtitle, children }: { onClose: () => void; title: ReactNode; subtitle?: ReactNode; children: ReactNode }) {
  useEsc(onClose);
  return (
    <div className="fixed inset-0 z-30 flex justify-end bg-void/50" onMouseDown={onClose}>
      <aside className="anim-slide-in flex h-full w-full max-w-xl flex-col border-l border-smoke/60 bg-carbon shadow-2xl shadow-black/60" onMouseDown={(e) => e.stopPropagation()}>
        <div className="flex items-start justify-between gap-3 border-b border-smoke/60 px-5 py-4">
          <div className="min-w-0">
            <h2 className="text-[15px] font-semibold text-bone">{title}</h2>
            {subtitle && <p className="mt-1 text-xs text-fog">{subtitle}</p>}
          </div>
          <button className="btn-quiet -mr-2 px-2" onClick={onClose} aria-label="关闭"><Icon name="x" /></button>
        </div>
        <div className="flex-1 overflow-y-auto px-5 py-4">{children}</div>
      </aside>
    </div>
  );
}

/* ---------------- 提示条 + 确认框（全局） ---------------- */

type ToastKind = 'ok' | 'err' | 'info';
interface Toast { id: number; kind: ToastKind; text: string }
interface ConfirmReq { title: string; body?: ReactNode; okText?: string; danger?: boolean; resolve: (v: boolean) => void }

interface FeedbackCtx {
  toast: (text: string, kind?: ToastKind) => void;
  confirm: (opts: Omit<ConfirmReq, 'resolve'>) => Promise<boolean>;
}

const Ctx = createContext<FeedbackCtx | null>(null);

export function useFeedback() {
  const c = useContext(Ctx);
  if (!c) throw new Error('FeedbackProvider missing');
  return c;
}

export function FeedbackProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [pending, setPending] = useState<ConfirmReq | null>(null);
  const seq = useRef(0);

  const toast = useCallback((text: string, kind: ToastKind = 'ok') => {
    const id = ++seq.current;
    setToasts((t) => [...t, { id, kind, text }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), kind === 'err' ? 9000 : 4500);
  }, []);

  const confirm = useCallback((opts: Omit<ConfirmReq, 'resolve'>) => new Promise<boolean>((resolve) => setPending({ ...opts, resolve })), []);

  const close = (v: boolean) => {
    pending?.resolve(v);
    setPending(null);
  };

  return (
    <Ctx.Provider value={{ toast, confirm }}>
      {children}
      <div className="pointer-events-none fixed bottom-5 right-5 z-50 flex w-[380px] max-w-[calc(100vw-2.5rem)] flex-col gap-2">
        {toasts.map((t) => (
          <div key={t.id} className={clsx('anim-fade-up pointer-events-auto flex items-start gap-2.5 rounded-xl border bg-obsidian/95 px-4 py-3 text-[13px] leading-relaxed shadow-xl shadow-black/40 backdrop-blur',
            t.kind === 'ok' && 'border-pulse/40', t.kind === 'err' && 'border-coral/50', t.kind === 'info' && 'border-teal/40')}>
            <Icon name={t.kind === 'ok' ? 'check' : t.kind === 'err' ? 'warn' : 'info'}
              className={clsx('mt-0.5 h-4 w-4', t.kind === 'ok' && 'text-pulse', t.kind === 'err' && 'text-coral', t.kind === 'info' && 'text-teal')} />
            <span className="flex-1 whitespace-pre-wrap text-mist">{t.text}</span>
            <button className="text-ash hover:text-bone" onClick={() => setToasts((x) => x.filter((y) => y.id !== t.id))} aria-label="关闭"><Icon name="x" className="h-3.5 w-3.5" /></button>
          </div>
        ))}
      </div>
      <Modal open={!!pending} onClose={() => close(false)} title={pending?.title ?? ''} width="max-w-md"
        footer={<>
          <button className="btn-ghost" onClick={() => close(false)}>取消</button>
          <button className={pending?.danger ? 'btn bg-coral font-semibold text-white hover:brightness-110' : 'btn-primary'} onClick={() => close(true)} autoFocus>
            {pending?.okText ?? '确定'}
          </button>
        </>}>
        <div className="text-sm leading-relaxed text-mist">{pending?.body}</div>
      </Modal>
    </Ctx.Provider>
  );
}
