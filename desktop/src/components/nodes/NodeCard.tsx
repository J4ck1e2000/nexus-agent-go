import type { EnrichedNode } from '../../lib/node-logic';
import { TIER_BAR_COLORS, TIER_PILL_STYLES } from '../../lib/node-logic';
import { formatPercent, formatSpeed, formatTime, initialsOf, avatarTone } from '../../lib/format';
import MiniProgress from '../common/MiniProgress';
import { useLanguage } from '../../hooks/useLanguage';

interface NodeCardProps {
  node: EnrichedNode;
  selected: boolean;
  onSelect: () => void;
  /** Admin-only delete action; rendered when provided. */
  onDelete?: () => void;
}

/** Sidebar node card: status, tier, mini tiles, score bar, last sync. */
export default function NodeCard({ node, selected, onSelect, onDelete }: NodeCardProps) {
  const { t } = useLanguage();
  const tierStyle = TIER_PILL_STYLES[node.effectiveAvailabilityTier] ?? TIER_PILL_STYLES.offline;
  const barColor = TIER_BAR_COLORS[node.effectiveAvailabilityTier] ?? TIER_BAR_COLORS.offline;
  const statusLabel = node.online ? t('availability.available') : t('availability.offline');

  return (
    <button
      type="button"
      onClick={onSelect}
      className={`soft-panel-subtle w-full p-4 text-left transition hover:-translate-y-px hover:shadow-soft ${
        selected ? 'border-[#5f9189] shadow-[0_0_0_3px_rgba(95,145,137,0.14)]' : ''
      } ${node.online ? '' : 'opacity-80'}`}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span
              className="h-2 w-2 shrink-0 rounded-full"
              style={{ background: node.online ? '#5f9189' : '#a39a8f' }}
              aria-hidden="true"
            />
            <span className="truncate text-sm font-semibold text-ink">{node.name}</span>
          </div>
          <p className="mt-1 truncate font-mono text-[11px] text-muted">{node.hostDisplay}</p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <span className={`rounded-full border px-2 py-0.5 text-[10px] font-semibold ${tierStyle}`}>
            {t(`availability.${node.effectiveAvailabilityTier}`)}
          </span>
          {onDelete && (
            <span
              role="button"
              tabIndex={0}
              aria-label={t('action.deleteNode')}
              className="cursor-pointer rounded-full p-1 text-[#a39a8f] transition hover:bg-[#f2e3df] hover:text-[#7a4740]"
              onClick={(event) => {
                event.stopPropagation();
                onDelete();
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.stopPropagation();
                  onDelete();
                }
              }}
            >
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M3 6h18" />
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
                <path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
              </svg>
            </span>
          )}
        </div>
      </div>

      <div className="mt-3 grid grid-cols-4 gap-1.5 text-center">
        <MiniTile label={t('node.users')} value={`${node.effectiveUserCount}`} />
        <MiniTile label={t('node.gpuCount')} value={`${node.effectiveGpuSummary.gpuCount}`} />
        <MiniTile label={t('node.idleGpu')} value={`${node.effectiveGpuSummary.idleGpuCount}`} />
        <MiniTile label={t('node.busyGpu')} value={`${node.effectiveGpuSummary.busyGpuCount}`} />
      </div>

      <div className="mt-3 flex items-center gap-2 font-mono text-[11px] text-muted">
        <span>CPU {formatPercent(node.cpuPercent)}%</span>
        <span className="text-[#d8cfc4]">·</span>
        <span>RAM {formatPercent(node.ramPercent)}%</span>
        <span className="text-[#d8cfc4]">·</span>
        <span>
          {t('node.avgGpu')} {formatPercent(node.effectiveGpuSummary.avgUtilization)}%
        </span>
      </div>

      <div className="mt-2 flex items-center justify-between font-mono text-[11px] text-muted">
        <span>
          {t('node.vram')} {formatNumber(node.effectiveGpuSummary.totalMemoryUsed)}/
          {formatNumber(node.effectiveGpuSummary.totalMemory)} GB
        </span>
        <span>
          ↓ {formatSpeed(node.networkDown)} / ↑ {formatSpeed(node.networkUp)}
        </span>
      </div>

      <div className="mt-3">
        <div className="flex items-center justify-between text-[11px] text-muted">
          <span>{t('availability.score')}</span>
          <span className="font-mono font-semibold text-ink">
            {node.effectiveAvailabilityScore}
          </span>
        </div>
        <div className="mt-1">
          <MiniProgress value={node.effectiveAvailabilityScore} color={barColor} />
        </div>
      </div>

      <div className="mt-3 flex items-center justify-between text-[11px] text-muted">
        <span>
          {t('node.lastSync')}{' '}
          <span className="font-mono">
            {node.lastSeenAt ? formatTime(new Date(node.lastSeenAt)) : '—'}
          </span>
        </span>
        {node.effectiveActiveUsers.length > 0 && (
          <span className="flex items-center -space-x-1.5">
            {node.effectiveActiveUsers.slice(0, 3).map((user) => (
              <span
                key={user}
                title={user}
                className={`flex h-5 w-5 items-center justify-center rounded-full text-[9px] font-semibold ring-2 ring-panel-soft ${avatarTone(user)}`}
              >
                {initialsOf(user)}
              </span>
            ))}
          </span>
        )}
      </div>
      <span className="sr-only">{statusLabel}</span>
    </button>
  );
}

function MiniTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-panel px-1 py-1.5">
      <p className="truncate text-[9px] uppercase tracking-wide text-muted">{label}</p>
      <p className="font-mono text-xs font-semibold text-ink">{value}</p>
    </div>
  );
}

function formatNumber(value: number): string {
  return value.toFixed(1);
}
