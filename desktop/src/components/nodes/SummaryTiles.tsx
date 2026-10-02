import type { EnrichedNode } from '../../lib/node-logic';
import { isActiveGpuProcess } from '../../lib/node-logic';
import { formatPercent } from '../../lib/format';
import { useLanguage } from '../../hooks/useLanguage';

/** Header KPI tiles computed from online nodes only (web parity). */
export default function SummaryTiles({ nodes }: { nodes: EnrichedNode[] }) {
  const { t } = useLanguage();

  const onlineNodes = nodes.filter((node) => node.online);
  const activeCount = onlineNodes.length;
  const totalCount = nodes.length;
  const totalGpu = onlineNodes.reduce((sum, node) => sum + node.effectiveGpuSummary.gpuCount, 0);
  const totalPower = onlineNodes.reduce((sum, node) => sum + node.effectiveGpuSummary.totalPower, 0);
  const cpuValues = onlineNodes
    .map((node) => node.cpuPercent)
    .filter((value): value is number => value !== null);
  const ramValues = onlineNodes
    .map((node) => node.ramPercent)
    .filter((value): value is number => value !== null);
  const avgCpu = cpuValues.length > 0 ? cpuValues.reduce((a, b) => a + b, 0) / cpuValues.length : null;
  const avgRam = ramValues.length > 0 ? ramValues.reduce((a, b) => a + b, 0) / ramValues.length : null;
  const activeUsers = new Set<string>();
  for (const node of onlineNodes) {
    for (const process of node.activeProcesses) {
      if (isActiveGpuProcess(process)) activeUsers.add(process.user);
    }
  }

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
      <Tile
        label={t('metric.onlineServers')}
        value={`${activeCount}`}
        sub={t('summary.onlineRatio', { online: activeCount, total: totalCount })}
      />
      <Tile label={t('metric.totalGpu')} value={`${totalGpu}`} sub={t('metric.gpuUnit')} />
      <Tile
        label={t('metric.totalPower')}
        value={totalPower > 0 ? `${totalPower.toFixed(0)}W` : '—'}
        sub={t('metric.summedGpuDraw')}
      />
      <Tile
        label={t('metric.avgCpu')}
        value={avgCpu === null ? '—' : `${formatPercent(avgCpu)}%`}
        sub={t('metric.acrossReachableNodes')}
      />
      <Tile
        label={t('metric.avgRam')}
        value={avgRam === null ? '—' : `${formatPercent(avgRam)}%`}
        sub={t('metric.acrossReachableNodes')}
      />
      <Tile
        label={t('summary.activeUsers')}
        value={`${activeUsers.size}`}
        sub={t('metric.detectedOnlineNodes')}
      />
    </div>
  );
}

function Tile({ label, value, sub }: { label: string; value: string; sub: string }) {
  return (
    <div className="soft-panel-subtle px-4 py-3">
      <p className="eyebrow">{label}</p>
      <p className="mt-1.5 font-mono text-xl font-semibold text-ink">{value}</p>
      <p className="mt-0.5 truncate text-[11px] text-muted">{sub}</p>
    </div>
  );
}
