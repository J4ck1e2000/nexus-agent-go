import { useLanguage } from '../../hooks/useLanguage';
import { formatPercent, formatGbs, formatSpeed } from '../../lib/format';
import type { EnrichedNode } from '../../lib/node-logic';

/** Resource overview KPI tile (CPU / RAM / NET / GPUs). */
export default function NodeKpiTile({
  label,
  value,
  unit,
  sub,
  progress,
}: {
  label: string;
  value: string;
  unit?: string;
  sub?: string;
  progress?: number | null;
}) {
  return (
    <div className="soft-panel-subtle px-4 py-3.5">
      <p className="eyebrow">{label}</p>
      <p className="mt-1.5 font-mono text-lg font-semibold text-ink">
        {value}
        {unit && <span className="ml-1 text-xs font-normal text-muted">{unit}</span>}
      </p>
      {progress !== undefined && (
        <div className="mt-2">
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-track">
            <div
              className="h-full rounded-full bg-accent transition-[width] duration-500"
              style={{ width: `${Math.min(100, Math.max(0, progress ?? 0))}%` }}
            />
          </div>
        </div>
      )}
      {sub && <p className="mt-1.5 truncate text-[11px] text-muted">{sub}</p>}
    </div>
  );
}

/** The four resource-overview tiles for one node. */
export function ResourceOverviewTiles({ node }: { node: EnrichedNode }) {
  const { t } = useLanguage();
  const data = node.data;
  const gpuSummary = node.effectiveGpuSummary;

  return (
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
      <NodeKpiTile
        label={t('metric.cpu')}
        value={formatPercent(node.cpuPercent)}
        unit="%"
        progress={node.cpuPercent}
        sub={`${data?.cpu_cores ?? '—'} ${t('metric.cores')}`}
      />
      <NodeKpiTile
        label={t('metric.ram')}
        value={formatPercent(node.ramPercent)}
        unit="%"
        progress={node.ramPercent}
        sub={`${formatGbs(data?.ram_used ?? null)} ${t('metric.gbUsed')}`}
      />
      <NodeKpiTile
        label={t('metric.net')}
        value={formatSpeed(node.networkDown)}
        sub={`↑ ${formatSpeed(node.networkUp)}`}
      />
      <NodeKpiTile
        label={t('metric.gpus')}
        value={`${gpuSummary.gpuCount}`}
        progress={gpuSummary.avgUtilization}
        sub={`${formatGbs(gpuSummary.totalMemoryUsed)}/${formatGbs(gpuSummary.totalMemory)} · ${formatPowerOrDash(gpuSummary.totalPower)} · ${node.effectiveUserCount} ${t('node.users')}`}
      />
    </div>
  );
}

function formatPowerOrDash(watts: number): string {
  return watts > 0 ? `${watts.toFixed(0)}W` : '—';
}
