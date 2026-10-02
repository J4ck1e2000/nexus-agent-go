import { useEffect, useMemo, useState } from 'react';
import { useNodes } from '../hooks/useNodes';
import { useLanguage } from '../hooks/useLanguage';
import { collectDrawerGroups, matchesFilter, sortNodes, type EnrichedNode, type SortMode } from '../lib/node-logic';
import SummaryTiles from '../components/nodes/SummaryTiles';
import NodeGrid from '../components/nodes/NodeGrid';
import NodeDetail, { type SelectedGpu } from '../components/nodes/NodeDetail';
import ActivityDrawer from '../components/nodes/ActivityDrawer';
import { formatTime } from '../lib/format';

export default function DashboardPage() {
  const { t } = useLanguage();
  const { nodes, lastSyncedAt } = useNodes(true);

  const [query, setQuery] = useState('');
  const [sortMode, setSortMode] = useState<SortMode>('availability');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [selectedGpu, setSelectedGpu] = useState<SelectedGpu | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);

  const visibleNodes = useMemo(
    () => sortNodes(nodes.filter((node) => matchesFilter(node, query)), sortMode),
    [nodes, query, sortMode],
  );

  // Keep a valid selection: re-pick when it vanished (e.g. node deleted).
  useEffect(() => {
    if (nodes.length === 0) {
      setSelectedId(null);
      return;
    }
    setSelectedId((current) => {
      if (current !== null && nodes.some((node) => node.id === current)) return current;
      const firstOnline = nodes.find((node) => node.online) ?? nodes[0];
      return firstOnline.id;
    });
  }, [nodes]);

  // GPU selection is per-node: clear it when the inspected node changes.
  useEffect(() => {
    setSelectedGpu(null);
  }, [selectedId]);

  const selectedNode: EnrichedNode | null = useMemo(
    () => nodes.find((node) => node.id === selectedId) ?? null,
    [nodes, selectedId],
  );

  const drawerGroups = useMemo(() => collectDrawerGroups(nodes), [nodes]);

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <SummaryTiles nodes={nodes} />
      </div>

      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted">
        <div className="flex items-center gap-2">
          <span className="rounded-full border border-line bg-panel-soft px-3 py-1.5">
            {t('summary.synced')}{' '}
            <span className="font-mono text-ink">
              {lastSyncedAt ? formatTime(lastSyncedAt) : '—'}
            </span>
          </span>
          <span className="rounded-full border border-line bg-panel-soft px-3 py-1.5">
            {t('summary.refresh')} · {t('summary.refreshValue')}
          </span>
        </div>
        <button type="button" className="muted-button" onClick={() => setDrawerOpen(true)}>
          {t('action.gpuUsers')}
        </button>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 lg:grid-cols-[370px_1fr]">
        <NodeGrid
          nodes={nodes}
          filteredNodes={visibleNodes}
          selectedId={selectedId}
          onSelect={setSelectedId}
          query={query}
          onQueryChange={setQuery}
          sortMode={sortMode}
          onSortModeChange={setSortMode}
          onClearFilter={() => setQuery('')}
        />
        <NodeDetail
          node={selectedNode}
          selectedGpu={selectedGpu}
          onSelectGpu={setSelectedGpu}
        />
      </div>

      <ActivityDrawer open={drawerOpen} groups={drawerGroups} onClose={() => setDrawerOpen(false)} />
    </div>
  );
}
