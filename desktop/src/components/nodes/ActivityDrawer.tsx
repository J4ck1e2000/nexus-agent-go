import { useEffect, useState } from 'react';
import type { DrawerUserGroup } from '../../lib/node-logic';
import { avatarTone, initialsOf } from '../../lib/format';
import { useLanguage } from '../../hooks/useLanguage';

/** Right slide-over listing active GPU users across all online nodes. */
export default function ActivityDrawer({
  open,
  groups,
  onClose,
}: {
  open: boolean;
  groups: DrawerUserGroup[];
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [expanded, setExpanded] = useState<string | null>(null);

  // Matches AppModal: Esc closes and background scrolling is locked.
  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') {
        event.preventDefault();
        onClose();
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50" role="dialog" aria-modal="true" data-nexus-overlay="open">
      <div
        className="absolute inset-0 bg-[#2f2922]/25 backdrop-blur-[2px]"
        onClick={onClose}
        aria-hidden="true"
      />
      <aside className="animate-[drawerIn_200ms_ease-out] absolute inset-y-0 right-0 flex w-full max-w-md flex-col border-l border-line bg-panel shadow-soft">
        <header className="flex items-start justify-between gap-3 border-b border-line p-5">
          <div>
            <p className="eyebrow">{t('drawer.eyebrow')}</p>
            <h2 className="mt-1 text-lg font-semibold tracking-tight text-ink">{t('drawer.title')}</h2>
            <p className="mt-1 text-[11px] text-muted">{t('drawer.thresholdNote')}</p>
          </div>
          <button type="button" className="muted-button px-3 py-1.5" onClick={onClose} aria-label={t('action.close')}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M18 6 6 18" />
              <path d="m6 6 12 12" />
            </svg>
          </button>
        </header>

        <div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto p-5">
          {groups.length === 0 ? (
            <p className="text-sm text-muted">{t('empty.noActiveGpuUsers')}</p>
          ) : (
            <ul className="flex flex-col gap-2.5">
              {groups.map((group, index) => {
                const isExpanded = expanded === group.user;
                return (
                  <li key={group.user} className="soft-panel-subtle p-3.5">
                    <div className="flex items-center gap-3">
                      <span
                        className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold ${avatarTone(group.user)}`}
                      >
                        {initialsOf(group.user)}
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-semibold text-ink">
                          <span className="mr-1 font-mono text-xs text-muted">#{index + 1}</span>
                          {group.user}
                        </p>
                        <div className="mt-1 flex flex-wrap gap-1.5">
                          <span className="rounded-full bg-panel px-2 py-0.5 font-mono text-[10px] text-muted">
                            {(group.totalVramMb / 1024).toFixed(1)} GB
                          </span>
                          <span className="rounded-full bg-panel px-2 py-0.5 font-mono text-[10px] text-muted">
                            {group.gpuCount} {t('node.gpuCount')}
                          </span>
                        </div>
                      </div>
                      <button
                        type="button"
                        className="cursor-pointer rounded-full p-1.5 text-muted transition hover:bg-panel hover:text-ink"
                        aria-expanded={isExpanded}
                        aria-label={group.user}
                        onClick={() => setExpanded(isExpanded ? null : group.user)}
                      >
                        <svg
                          className={isExpanded ? 'rotate-180 transition' : 'transition'}
                          width="14"
                          height="14"
                          viewBox="0 0 24 24"
                          fill="none"
                          stroke="currentColor"
                          strokeWidth="2"
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          aria-hidden="true"
                        >
                          <path d="m6 9 6 6 6-6" />
                        </svg>
                      </button>
                    </div>

                    {isExpanded && (
                      <div className="mt-3 grid gap-1.5 border-t border-line pt-3">
                        {group.processes.map((process, processIndex) => (
                          <div
                            key={`${process.serverName}-${process.pid}-${processIndex}`}
                            className="grid grid-cols-4 gap-2 rounded-lg bg-panel px-3 py-2 font-mono text-[10px] text-muted"
                          >
                            <span className="truncate text-ink" title={process.serverName}>
                              {process.serverName}
                            </span>
                            <span>GPU {process.gpuIndex ?? '—'}</span>
                            <span>PID {process.pid}</span>
                            <span className="text-right">
                              {(process.vramUsedMb / 1024).toFixed(2)} GB
                            </span>
                          </div>
                        ))}
                      </div>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </aside>
    </div>
  );
}
