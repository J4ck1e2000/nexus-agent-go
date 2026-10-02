import type { EnrichedNode } from '../../lib/node-logic';
import { TIER_BAR_COLORS } from '../../lib/node-logic';
import { formatAgeShort, formatSpeed, formatTime } from '../../lib/format';
import { useLanguage } from '../../hooks/useLanguage';

interface EventItem {
  label: string;
  value: string;
  detail: string;
  color?: string;
}

/** The five snapshot pseudo-events from the web dashboard's event feed. */
export default function EventFeed({ node }: { node: EnrichedNode }) {
  const { t } = useLanguage();

  const events: EventItem[] = [
    {
      label: t('snapshot.updatedLabel'),
      value: node.lastSeenAt ? formatTime(new Date(node.lastSeenAt)) : '—',
      detail: node.online ? t('detail.onlineHint') : t('detail.offlineHint'),
    },
    {
      label: t('availability.score'),
      value: `${node.effectiveAvailabilityScore}`,
      detail: t(`availability.${node.effectiveAvailabilityTier}`),
      color: TIER_BAR_COLORS[node.effectiveAvailabilityTier],
    },
    {
      label: t('snapshot.occupancyLabel'),
      value: t('snapshot.occupancyValue', { count: node.effectiveUserCount }),
      detail: `${node.effectiveGpuSummary.idleGpuCount} ${t('node.idleGpu')} · ${node.effectiveGpuSummary.busyGpuCount} ${t('node.busyGpu')}`,
    },
    {
      label: t('node.dataAge'),
      value: formatAgeShort(node.effectiveDataAgeSec),
      detail: t('snapshot.updatedDetail'),
    },
    {
      label: t('node.network'),
      value: `↓ ${formatSpeed(node.networkDown)}`,
      detail: `↑ ${formatSpeed(node.networkUp)}`,
    },
  ];

  return (
    <section className="soft-panel p-5">
      <h3 className="eyebrow">{t('detail.snapshotEvents')}</h3>
      <ul className="mt-3 flex flex-col gap-2.5">
        {events.map((event) => (
          <li key={event.label} className="flex items-start gap-2.5">
            <span
              className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full"
              style={{ background: event.color ?? '#5f9189' }}
              aria-hidden="true"
            />
            <div className="min-w-0">
              <div className="flex items-baseline justify-between gap-2">
                <p className="truncate text-xs font-medium text-ink">{event.label}</p>
                <p className="shrink-0 font-mono text-xs text-ink">{event.value}</p>
              </div>
              <p className="truncate text-[11px] text-muted">{event.detail}</p>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
