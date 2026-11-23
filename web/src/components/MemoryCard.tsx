import { fmtDate, type Memory } from '../lib/api';
import { Avatar, Icon } from './ui';

/** 一条记忆：把"客户（日期）：… / 业务员（日期）：…"拆成两段显示 */
export function MemoryCard({ m, contact, onDelete }: { m: Memory; contact?: { name: string; email?: string }; onDelete: () => void }) {
  const parts = m.content.split(/\n(?=业务员（)/);
  return (
    <div className="anim-fade-up group panel p-4 transition-colors hover:border-smoke">
      <div className="mb-3 flex items-center gap-2.5">
        {contact && <Avatar name={contact.name} size="sm" />}
        <div className="min-w-0 flex-1 text-xs">
          {contact && (
            <div className="truncate font-medium text-mist">
              {contact.name}{contact.email && <span className="ml-1.5 font-normal text-ash">{contact.email}</span>}
            </div>
          )}
          <div className="text-ash">记忆 #{m.id} · 来自会话 #{m.conversation_id} · {fmtDate(m.created_at)}</div>
        </div>
        <button className="btn-quiet px-2 py-1 text-xs opacity-0 transition-opacity hover:text-coral group-hover:opacity-100 focus:opacity-100" onClick={onDelete}>
          <Icon name="trash" className="h-3.5 w-3.5" />删除
        </button>
      </div>
      <div className="space-y-2">
        {parts.map((p, i) => {
          const agent = p.startsWith('业务员（');
          const label = p.match(/^(客户|业务员)（([^）]*)）[:：]/);
          const body = label ? p.slice(label[0].length) : p;
          return (
            <div key={i} className={agent ? 'rounded-lg border border-lime/15 bg-lime/[0.05] px-3 py-2' : 'rounded-lg bg-graphite/60 px-3 py-2'}>
              {label && (
                <div className={agent ? 'mb-1 text-[11px] font-medium text-lime/80' : 'mb-1 text-[11px] font-medium text-fog'}>
                  {label[1] === '客户' ? '客户问' : '我们答'} · {label[2]}
                </div>
              )}
              <div className="whitespace-pre-wrap text-[13px] leading-relaxed text-mist">{body.trim()}</div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
