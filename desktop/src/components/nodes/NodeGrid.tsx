import { useLanguage } from '../../hooks/useLanguage';
import type { EnrichedNode } from '../../lib/node-logic';
import NodeListToolbar from './NodeListToolbar';
import NodeCard from './NodeCard';
import Spinner from '../common/Spinner';
import type { SortMode } from '../../lib/node-logic';

interface NodeGridProps {
  nodes: EnrichedNode[];
  filteredNodes: EnrichedNode[];
  /** True while the first overview fetch is still in flight. */
  loading?: boolean;
  selectedId: number | null;
  onSelect: (id: number) => void;
  query: string;
  onQueryChange: (query: string) => void;
  sortMode: SortMode;
  onSortModeChange: (mode: SortMode) => void;
  onClearFilter: () => void;
  /** Hover state of the card list; while hovered the row order is frozen. */
  onHoverChange?: (hovered: boolean) => void;
  /** Admin-only delete handler; hides trash icons when absent. */
  onDeleteNode?: (node: EnrichedNode) => void;
  /** Admin-only CTA shown when no nodes exist. */
  onAddNode?: () => void;
}

/** Left column: search/sort toolbar + scrollable node card list. */
export default function NodeGrid({
  nodes,
  filteredNodes,
  loading = false,
  selectedId,
  onSelect,
  query,
  onQueryChange,
  sortMode,
  onSortModeChange,
  onClearFilter,
  onHoverChange,
  onDeleteNode,
  onAddNode,
}: NodeGridProps) {
  const { t } = useLanguage();

  return (
    <aside className="flex h-full min-h-0 w-full flex-col gap-3 lg:w-[370px] lg:shrink-0">
      <NodeListToolbar
        query={query}
        onQueryChange={onQueryChange}
        sortMode={sortMode}
        onSortModeChange={onSortModeChange}
        onClear={onClearFilter}
      />

      {loading && nodes.length === 0 ? (
        <div className="soft-panel-subtle flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center text-muted">
          <Spinner size={22} />
          <p className="text-xs">{t('desktop.nodes.loading')}</p>
        </div>
      ) : nodes.length === 0 ? (
        <div className="soft-panel-subtle flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
          <p className="text-sm font-semibold text-ink">{t('empty.noConfiguredNodes')}</p>
          <p className="text-xs text-muted">{t('empty.addFirstNode')}</p>
          {onAddNode && (
            <button type="button" className="primary-button mt-1" onClick={onAddNode}>
              {t('action.connectNode')}
            </button>
          )}
        </div>
      ) : filteredNodes.length === 0 ? (
        <div className="soft-panel-subtle flex flex-1 items-center justify-center p-6 text-center text-xs text-muted">
          {t('panel.emptyFilter')}
        </div>
      ) : (
        <div
          className="custom-scrollbar flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto pr-1"
          onMouseEnter={() => onHoverChange?.(true)}
          onMouseLeave={() => onHoverChange?.(false)}
        >
          {filteredNodes.map((node) => (
            <NodeCard
              key={node.id}
              node={node}
              selected={node.id === selectedId}
              onSelect={() => onSelect(node.id)}
              onDelete={onDeleteNode ? () => onDeleteNode(node) : undefined}
            />
          ))}
        </div>
      )}
    </aside>
  );
}
