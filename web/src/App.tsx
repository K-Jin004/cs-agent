import { clsx } from 'clsx';
import { useEffect, useState } from 'react';
import { Icon, type IconName } from './components/ui';
import { ChatPage } from './pages/chat/ChatPage';
import { KBPage } from './pages/KBPage';
import { MemoryPage } from './pages/MemoryPage';

type View = 'chat' | 'memory' | 'kb';

const NAV: { id: View; label: string; icon: IconName; hint: string }[] = [
  { id: 'chat', label: '会话', icon: 'chat', hint: '客户来信与 AI 草稿' },
  { id: 'memory', label: '记忆', icon: 'memory', hint: '记住的历史问答' },
  { id: 'kb', label: '知识库', icon: 'book', hint: '公司事实资料' },
];

function initialView(): View {
  const h = window.location.hash.replace('#', '');
  if (h === 'chat' || h === 'memory' || h === 'kb') return h;
  try {
    const v = localStorage.getItem('view');
    if (v === 'chat' || v === 'memory' || v === 'kb') return v;
  } catch {
    /* 隐私模式等 */
  }
  return 'chat';
}

export function App() {
  const [view, setView] = useState<View>(initialView);

  // 浏览器前进/后退或手动改 #hash 时同步切换页面
  useEffect(() => {
    const h = () => setView(initialView());
    window.addEventListener('hashchange', h);
    return () => window.removeEventListener('hashchange', h);
  }, []);

  useEffect(() => {
    if (window.location.hash !== `#${view}`) window.location.hash = view;
    try {
      localStorage.setItem('view', view);
    } catch {
      /* ignore */
    }
  }, [view]);

  return (
    <div className="flex h-full">
      <aside className="flex w-56 shrink-0 flex-col border-r border-smoke/60 bg-carbon">
        <div className="flex h-14 items-center gap-2.5 px-5">
          <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-lime/10 text-lime">
            <Icon name="mail" className="h-4 w-4" />
          </span>
          <span className="text-sm font-bold tracking-wide text-bone">
            CS<span className="text-glow text-lime">Agent</span>
          </span>
        </div>
        <nav className="mt-3 flex-1 space-y-1 px-3">
          {NAV.map((n) => (
            <button key={n.id} onClick={() => setView(n.id)}
              className={clsx('group relative flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm transition-colors duration-200',
                view === n.id ? 'bg-obsidian text-lime' : 'text-fog hover:bg-obsidian/60 hover:text-mist')}>
              <span className={clsx('absolute left-0 h-5 w-0.5 rounded-full bg-lime transition-opacity', view === n.id ? 'opacity-100' : 'opacity-0')} />
              <Icon name={n.icon} />
              <span className="flex flex-col">
                <span className="font-medium">{n.label}</span>
                <span className="text-[11px] text-ash">{n.hint}</span>
              </span>
            </button>
          ))}
        </nav>
        <div className="space-y-1 px-5 py-4 text-[10px] leading-relaxed text-ash">
          <div className="flex items-center gap-1.5">
            <span className="h-1.5 w-1.5 rounded-full bg-pulse" /> 仅本机运行 · 127.0.0.1
          </div>
          <div>外贸客服草稿助手</div>
        </div>
      </aside>
      <main className="min-w-0 flex-1">
        {view === 'chat' && <ChatPage />}
        {view === 'memory' && <MemoryPage />}
        {view === 'kb' && <KBPage />}
      </main>
    </div>
  );
}
