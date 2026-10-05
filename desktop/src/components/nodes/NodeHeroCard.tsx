import type { EnrichedNode } from '../../lib/node-logic';
import { TIER_PILL_STYLES } from '../../lib/node-logic';
import { formatPercent, formatSpeed } from '../../lib/format';
import { useLanguage } from '../../hooks/useLanguage';
import { TerminalSquare } from 'lucide-react';

/** Hero card at the top of the node detail column. */
export default function NodeHeroCard({ node, onOpenTerminal }: { node: EnrichedNode; onOpenTerminal: () => void }) {
  const { t } = useLanguage();
  const tierStyle = TIER_PILL_STYLES[node.effectiveAvailabilityTier] ?? TIER_PILL_STYLES.offline;

  return (
    <div className="soft-panel-subtle p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="eyebrow">{t('detail.nodeHeroEyebrow')}</p>
          <h2 className="mt-1 truncate text-xl font-semibold tracking-tight text-ink">{node.name}</h2>
          <p className="mt-0.5 truncate font-mono text-xs text-muted">{node.hostDisplay}</p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <button type="button" className="muted-button !px-2.5 !py-1.5" onClick={onOpenTerminal} disabled={!node.sshHost} title={t('terminal.open')}>
            <TerminalSquare size={14} /> {t('terminal.open')}
          </button>
          <span
            className={`rounded-full border px-2.5 py-1 text-[10px] font-semibold ${
              node.online
                ? 'bg-[#dcefe5] text-[#2f655a] border-[#c8dfd4]'
                : 'bg-[#ebe5dc] text-[#6e665d] border-[#dad0c4]'
            }`}
          >
            {node.online ? t('status.online') : t('availability.offline')}
          </span>
          <span className={`rounded-full border px-2.5 py-1 text-[10px] font-semibold ${tierStyle}`}>
            {t(`availability.${node.effectiveAvailabilityTier}`)} · {node.effectiveAvailabilityScore}
          </span>
        </div>
      </div>

      <div className="mt-4 flex flex-wrap gap-x-5 gap-y-2 font-mono text-[11px] text-muted">
        <span>
          CPU <span className="text-ink">{formatPercent(node.cpuPercent)}%</span>
        </span>
        <span>
          RAM <span className="text-ink">{formatPercent(node.ramPercent)}%</span>
        </span>
        <span>
          {t('node.gpuCount')}{' '}
          <span className="text-ink">{node.effectiveGpuSummary.gpuCount}</span>
        </span>
        <span>
          ↓ <span className="text-ink">{formatSpeed(node.networkDown)}</span> / ↑{' '}
          <span className="text-ink">{formatSpeed(node.networkUp)}</span>
        </span>
        <span>
          {t('node.users')} <span className="text-ink">{node.effectiveUserCount}</span>
        </span>
      </div>
    </div>
  );
}
