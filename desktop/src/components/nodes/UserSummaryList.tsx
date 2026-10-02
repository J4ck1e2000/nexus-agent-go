import type { EnrichedNode } from '../../lib/node-logic';
import { summarizeNodeUsers } from '../../lib/node-logic';
import { avatarTone, initialsOf } from '../../lib/format';
import { useLanguage } from '../../hooks/useLanguage';

/** Top GPU users on the selected node (by VRAM). */
export default function UserSummaryList({ node }: { node: EnrichedNode }) {
  const { t } = useLanguage();
  const users = summarizeNodeUsers(node);

  return (
    <section className="soft-panel flex min-h-0 flex-col p-5">
      <h3 className="eyebrow">{t('detail.userSummary')}</h3>
      {users.length === 0 ? (
        <p className="mt-3 text-xs text-muted">{t('empty.noActiveGpuUsersSnapshot')}</p>
      ) : (
        <ul className="custom-scrollbar mt-3 flex max-h-64 flex-col gap-2 overflow-y-auto pr-1">
          {users.map((entry, index) => (
            <li key={entry.user} className="flex items-center gap-2.5">
              <span
                className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-[10px] font-semibold ${avatarTone(entry.user)}`}
              >
                {initialsOf(entry.user)}
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-xs font-medium text-ink">
                  <span className="mr-1 font-mono text-muted">#{index + 1}</span>
                  {entry.user}
                </p>
                <p className="truncate text-[11px] text-muted">
                  {(entry.totalVramMb / 1024).toFixed(1)} GB VRAM ·{' '}
                  {entry.processCount > 1
                    ? t('misc.processCountPlural', { count: entry.processCount })
                    : t('misc.processCount', { count: entry.processCount })}
                </p>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
