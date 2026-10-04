import { useEffect, useMemo, useRef, useState } from 'react';
import type { EnrichedNode } from '../lib/node-logic';
import {
  applyFrozenOrder,
  collectDrawerGroups,
  matchesFilter,
  sortNodes,
  type SortMode,
} from '../lib/node-logic';
import { useNodes } from '../hooks/useNodes';
import { useNodeConfigs, type AddNodeInput } from '../hooks/useNodeConfigs';
import { localizedError } from '../lib/errors';
import { useAuth } from '../hooks/useAuth';
import { useLanguage } from '../hooks/useLanguage';
import { useToastContext } from '../context/ToastContext';
import SummaryTiles from '../components/nodes/SummaryTiles';
import NodeGrid from '../components/nodes/NodeGrid';
import NodeDetail, { type SelectedGpu } from '../components/nodes/NodeDetail';
import ActivityDrawer from '../components/nodes/ActivityDrawer';
import AddNodeDialog from '../components/admin/AddNodeDialog';
import UserManager from '../components/admin/UserManager';
import ConfirmDialog from '../components/common/ConfirmDialog';
import AIAssistant from '../components/ai/AIAssistant';
import { formatTime } from '../lib/format';

export default function DashboardPage() {
  const { t } = useLanguage();
  const { user } = useAuth();
  const { showToast } = useToastContext();
  const { nodes, lastSyncedAt, error: pollError, loading } = useNodes(true);
  const { configs, addNode, removeNode } = useNodeConfigs(user?.role === 'admin');

  const [query, setQuery] = useState('');
  const [sortMode, setSortMode] = useState<SortMode>('availability');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [selectedGpu, setSelectedGpu] = useState<SelectedGpu | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [addNodeOpen, setAddNodeOpen] = useState(false);
  const [savingNode, setSavingNode] = useState(false);
  const [deleteNodeTarget, setDeleteNodeTarget] = useState<{ id: number; name: string } | null>(null);
  const [deletingNode, setDeletingNode] = useState(false);
  const [usersOpen, setUsersOpen] = useState(false);
  // While the pointer rests on the node list, polling keeps updating card
  // values in place but no longer re-orders rows (see applyFrozenOrder).
  const [listHovered, setListHovered] = useState(false);
  const frozenOrderRef = useRef<EnrichedNode[]>([]);

  const isAdmin = user?.role === 'admin';

  const visibleNodes = useMemo(() => {
    const filtered = nodes.filter((node) => matchesFilter(node, query));
    if (listHovered) {
      return applyFrozenOrder(frozenOrderRef.current, filtered, sortMode);
    }
    const sorted = sortNodes(filtered, sortMode);
    frozenOrderRef.current = sorted;
    return sorted;
  }, [nodes, query, sortMode, listHovered]);

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

  const selectedNode = useMemo(
    () => nodes.find((node) => node.id === selectedId) ?? null,
    [nodes, selectedId],
  );

  const drawerGroups = useMemo(() => collectDrawerGroups(nodes), [nodes]);

  const handleCreateNode = async (payload: AddNodeInput): Promise<{ ok: true } | { ok: false; message: string }> => {
    setSavingNode(true);
    try {
      const result = await addNode(payload);
      if (result.ok) {
        setAddNodeOpen(false);
        return { ok: true };
      }
      const message = localizedError(result.error, t);
      const warning = result.error.detail === 'duplicate_node_url' || result.error.detail === 'duplicate_node_endpoint';
      showToast(message, warning ? 'warning' : 'error');
      return { ok: false, message };
    } finally {
      setSavingNode(false);
    }
  };

  const handleDeleteNode = async (): Promise<void> => {
    if (!deleteNodeTarget) return;
    setDeletingNode(true);
    try {
      const result = await removeNode(deleteNodeTarget.id);
      if (result.ok) {
        showToast(t('notify.nodeDeleted', { name: deleteNodeTarget.name }), 'success');
        setDeleteNodeTarget(null);
      } else {
        showToast(localizedError(result.error, t), 'error');
        setDeleteNodeTarget(null);
      }
    } finally {
      setDeletingNode(false);
    }
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      {pollError && nodes.length > 0 && (
        <div
          className="rounded-2xl border border-[#dfd0c5] bg-[#f1e7df] px-4 py-2.5 text-xs font-medium text-[#5f4a42]"
          role="status"
        >
          {t('desktop.connection.staleBanner')}
        </div>
      )}

      <SummaryTiles nodes={nodes} />

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
        <div className="flex items-center gap-2">
          {isAdmin && (
            <>
              <button type="button" className="primary-button" onClick={() => setAddNodeOpen(true)}>
                {t('action.addNode')}
              </button>
              <button type="button" className="muted-button" onClick={() => setUsersOpen(true)}>
                {t('action.manageAccounts')}
              </button>
            </>
          )}
          <button type="button" className="muted-button" onClick={() => setDrawerOpen(true)}>
            {t('action.gpuUsers')}
          </button>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 lg:grid-cols-[370px_1fr]">
        <NodeGrid
          nodes={nodes}
          filteredNodes={visibleNodes}
          loading={loading}
          selectedId={selectedId}
          onSelect={setSelectedId}
          query={query}
          onQueryChange={setQuery}
          sortMode={sortMode}
          onSortModeChange={setSortMode}
          onClearFilter={() => setQuery('')}
          onHoverChange={setListHovered}
          onDeleteNode={isAdmin ? (node) => setDeleteNodeTarget({ id: node.id, name: node.name }) : undefined}
          onAddNode={isAdmin ? () => setAddNodeOpen(true) : undefined}
        />
        <NodeDetail node={selectedNode} selectedGpu={selectedGpu} onSelectGpu={setSelectedGpu} />
      </div>

      <ActivityDrawer open={drawerOpen} groups={drawerGroups} onClose={() => setDrawerOpen(false)} />

      <AIAssistant selectedNodeName={selectedNode?.name ?? null} />

      {isAdmin && (
        <AddNodeDialog
          open={addNodeOpen}
          configs={configs}
          busy={savingNode}
          onClose={() => setAddNodeOpen(false)}
          onCreate={handleCreateNode}
        />
      )}

      <UserManager open={usersOpen && isAdmin} onClose={() => setUsersOpen(false)} />

      <ConfirmDialog
        open={deleteNodeTarget !== null}
        title={t('dialog.deleteNodeTitle')}
        description={t('dialog.deleteNodeDescription', { name: deleteNodeTarget?.name ?? '' })}
        confirmTone="danger"
        confirmLabel={t('action.deleteNode')}
        busy={deletingNode}
        onConfirm={() => void handleDeleteNode()}
        onCancel={() => setDeleteNodeTarget(null)}
      />
    </div>
  );
}
