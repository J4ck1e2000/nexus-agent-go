import { useMemo } from 'react';
import type { EnrichedNode } from '../../lib/node-logic';
import { useLanguage } from '../../hooks/useLanguage';
import GPUCard from '../gpu/GPUCard';
import GPUDetail from '../gpu/GPUDetail';
import EventFeed from './EventFeed';
import UserSummaryList from './UserSummaryList';
import NodeHeroCard from './NodeHeroCard';
import { ResourceOverviewTiles } from './NodeKpiTile';
import NodeHistoryPanel from './NodeHistoryPanel';

export interface SelectedGpu {
  serverId: number;
  gpuIndex: number;
  gpuId: number;
  gpuName: string;
}

interface NodeDetailProps {
  node: EnrichedNode | null;
  selectedGpu: SelectedGpu | null;
  onSelectGpu: (selection: SelectedGpu | null) => void;
  onOpenTerminal: () => void;
}

/** Right column: hero, resource overview, GPU matrix + inline detail, feeds. */
export default function NodeDetail({ node, selectedGpu, onSelectGpu, onOpenTerminal }: NodeDetailProps) {
  const { t } = useLanguage();
  const gpuMatrix = useMemo(() => node?.data?.gpus ?? [], [node]);

  if (!node) {
    return (
      <div className="soft-panel flex h-full items-center justify-center p-10 text-sm text-muted">
        {t('panel.emptySelect')}
      </div>
    );
  }

  const resolvedGpuIndex = selectedGpu?.serverId === node.id ? selectedGpu.gpuIndex : null;
  const resolvedGpu =
    resolvedGpuIndex !== null ? (gpuMatrix[resolvedGpuIndex] ?? null) : null;

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <NodeHeroCard node={node} onOpenTerminal={onOpenTerminal} />

      <section>
        <h3 className="eyebrow mb-2">{t('detail.resourceOverview')}</h3>
        <ResourceOverviewTiles node={node} />
      </section>

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 xl:grid-cols-[1.6fr_1fr]">
        <div className="flex min-h-0 flex-col gap-4">
          <section className="soft-panel p-5">
            <div className="flex items-center justify-between">
              <h3 className="eyebrow">{t('detail.gpuMatrix')}</h3>
              {gpuMatrix.length > 0 && (
                <p className="text-[11px] text-muted">{t('detail.clickGpuHint')}</p>
              )}
            </div>
            {gpuMatrix.length === 0 ? (
              <p className="mt-4 text-xs text-muted">{t('detail.noGpu')}</p>
            ) : (
              <div className="mt-3 grid grid-cols-2 gap-2.5 md:grid-cols-3">
                {gpuMatrix.map((gpu, index) => (
                  <GPUCard
                    key={gpu.id}
                    gpu={gpu}
                    selected={resolvedGpuIndex === index}
                    onSelect={() =>
                      onSelectGpu({
                        serverId: node.id,
                        gpuIndex: index,
                        gpuId: gpu.id,
                        gpuName: gpu.name,
                      })
                    }
                  />
                ))}
              </div>
            )}
          </section>

          {resolvedGpu && resolvedGpuIndex !== null && (
            <section className="min-h-0">
              <GPUDetail
                gpu={resolvedGpu}
                gpuIndex={resolvedGpuIndex}
                nodeName={node.name}
                onClose={() => onSelectGpu(null)}
              />
            </section>
          )}
        </div>

        <div className="flex min-h-0 flex-col gap-4">
          <EventFeed node={node} />
          <UserSummaryList node={node} />
        </div>
      </div>

      <NodeHistoryPanel node={node} />
    </div>
  );
}
