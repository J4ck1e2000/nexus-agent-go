import { useCallback, useEffect, useMemo, useState } from 'react';
import type { EnrichedNode as _EnrichedNode } from '../lib/node-logic';
import { collectDrawerGroups, matchesFilter, sortNodes, type SortMode } from '../lib/node-logic';
import { useNodes } from '../hooks/useNodes';
import { useNodeConfigs, type AddNodeInput } from '../hooks/useNodeConfigs';
import type { SSHTestOutcome } from '../components/admin/AddNodeDialog';
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
  const { nodes, lastSyncedAt } = useNodes(true);
  const { configs, addNode, removeNode, testSSH } = useNodeConfigs(user?.role === 'admin');

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

  const isAdmin = user?.role === 'admin';

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

  const selectedNode = useMemo(
    () => nodes.find((node) => node.id === selectedId) ?? null,
    [nodes, selectedId],
  );

  const drawerGroups = useMemo(() => collectDrawerGroups(nodes), [nodes]);

  const handleCreateNode = async (payload: AddNodeInput): Promise<boolean> => {
    setSavingNode(true);
    try {
      const result = await addNode(payload);
      if (result.ok) {
        setAddNodeOpen(false);
        return true;
      }
      if (result.error.detail === 'duplicate_node_url') {
        showToast(t('notify.duplicateNodeUrl'), 'warning');
      } else if (result.error.detail === 'duplicate_node_endpoint') {
        showToast(t('desktop.nodes.duplicateEndpoint'), 'warning');
      } else {
        showToast(t('notify.saveConfigFailed'), 'error');
      }
      return false;
    } finally {
      setSavingNode(false);
    }
  };

  const handleTestSSH = useCallback(
    async (payload: { sshHost: string; sshPort: number; sshUser: string }): Promise<SSHTestOutcome> => {
      const result = await testSSH(payload);
      if (result.ok) {
        return { ok: true, result: result.data };
      }
      return { ok: false, message: localizedError(result.error, t) };
    },
    [testSSH, t],
  );

  const handleDeleteNode = async (): Promise<void> => {
    if (!deleteNodeTarget) return;
    setDeletingNode(true);
    try {
      const result = await removeNode(deleteNodeTarget.id);
      if (result.ok) {
        showToast(t('notify.nodeDeleted', { name: deleteNodeTarget.name }), 'success');
        setDeleteNodeTarget(null);
      } else {
        showToast(t('notify.saveConfigFailed'), 'error');
        setDeleteNodeTarget(null);
      }
    } finally {
      setDeletingNode(false);
    }
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
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
          selectedId={selectedId}
          onSelect={setSelectedId}
          query={query}
          onQueryChange={setQuery}
          sortMode={sortMode}
          onSortModeChange={setSortMode}
          onClearFilter={() => setQuery('')}
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
          onTestSSH={handleTestSSH}
        />
      )}

      <UserManager open={usersOpen && isAdmin} onClose={() => setUsersOpen(false)} />

      <ConfirmDialog
        open={deleteNodeTarget !== null}
        title={t('dialog.deleteNodeTitle')}
        description={t('dialog.deleteNodeDescription', { name: deleteNodeTarget?.name ?? '' })}
        confirmTone="danger"
        busy={deletingNode}
        onConfirm={() => void handleDeleteNode()}
        onCancel={() => setDeleteNodeTarget(null)}
      />
    </div>
  );
}
