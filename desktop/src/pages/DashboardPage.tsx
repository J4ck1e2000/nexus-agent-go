import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react';
import { LayoutDashboard, Zap } from 'lucide-react';
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
import { loadIdleGpuFilters, rankIdleGpuCandidates, saveIdleGpuFilters } from '../lib/idle-gpu';
import { useFleetIdleHistory } from '../hooks/useNodeHistory';
import { useIdleReservationMatcher, useIdleReservations } from '../hooks/useIdleReservations';
import { useNotificationPreferences } from '../context/NotificationPreferencesContext';
import { useSystemAlerts } from '../hooks/useSystemAlerts';
import type { IdleReservationFilters } from '../../electron/types/ipc';
import IdleGpuView from '../components/nodes/IdleGpuView';
const SshTerminalDialog = lazy(() => import('../components/nodes/SshTerminalDialog'));

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
  const [terminalNode, setTerminalNode] = useState<EnrichedNode | null>(null);
  const [dashboardView, setDashboardView] = useState<'overview' | 'idle'>('overview');
  const [idleFilters, setIdleFilters] = useState<IdleReservationFilters>(loadIdleGpuFilters);
  // While the pointer rests on the node list, polling keeps updating card
  // values in place but no longer re-orders rows (see applyFrozenOrder).
  const [listHovered, setListHovered] = useState(false);
  const frozenOrderRef = useRef<EnrichedNode[]>([]);
  const pendingGpuSelection = useRef<SelectedGpu | null>(null);

  const isAdmin = user?.role === 'admin';

  const idleReservations = useIdleReservations();
  const needsIdleHistory = (dashboardView === 'idle' && idleFilters.idleDurationMinutes > 0)
    || idleReservations.reservations.some((item) => item.status === 'active' && item.filters.idleDurationMinutes > 0);
  const { historyByNode, loading: idleHistoryLoading, refresh: refreshIdleHistory } = useFleetIdleHistory(nodes, needsIdleHistory);
  const idleCandidates = useMemo(() => rankIdleGpuCandidates(nodes, historyByNode, idleFilters), [nodes, historyByNode, idleFilters]);
  const { preferences: notificationPreferences } = useNotificationPreferences();
  useSystemAlerts(nodes, notificationPreferences, t, user?.id ?? null);
  useIdleReservationMatcher(idleReservations.reservations, nodes, historyByNode, t, idleReservations.applyEvaluation);

  useEffect(() => {
    saveIdleGpuFilters(idleFilters);
  }, [idleFilters]);

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
    if (pendingGpuSelection.current?.serverId === selectedId) {
      setSelectedGpu(pendingGpuSelection.current);
      pendingGpuSelection.current = null;
    } else {
      pendingGpuSelection.current = null;
      setSelectedGpu(null);
    }
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

  const handleSelectIdleGpu = (nodeId: number, gpuId: number): void => {
    const node = nodes.find((item) => item.id === nodeId);
    const gpuIndex = node?.data?.gpus.findIndex((gpu) => gpu.id === gpuId) ?? -1;
    const gpu = gpuIndex >= 0 ? node?.data?.gpus[gpuIndex] : null;
    if (!node || !gpu) return;
    const selection = { serverId: nodeId, gpuIndex, gpuId: gpu.id, gpuName: gpu.name };
    if (selectedId === nodeId) setSelectedGpu(selection);
    else {
      pendingGpuSelection.current = selection;
      setSelectedId(nodeId);
    }
    setDashboardView('overview');
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <nav className="flex w-fit items-center gap-1 rounded-xl border border-line bg-panel-soft p-1" aria-label={t('idle.viewNavigation')}>
        <button type="button" className={`rounded-lg px-3 py-2 text-xs font-medium ${dashboardView === 'overview' ? 'bg-panel text-ink shadow-sm' : 'text-muted hover:text-ink'}`} onClick={() => setDashboardView('overview')}>
          <LayoutDashboard className="mr-1.5 inline" size={14} />{t('idle.overviewTab')}
        </button>
        <button type="button" className={`rounded-lg px-3 py-2 text-xs font-medium ${dashboardView === 'idle' ? 'bg-panel text-ink shadow-sm' : 'text-muted hover:text-ink'}`} onClick={() => setDashboardView('idle')}>
          <Zap className="mr-1.5 inline" size={14} />{t('idle.idleTab')}
        </button>
      </nav>

      {dashboardView === 'idle' ? (
        <IdleGpuView
          nodes={nodes}
          candidates={idleCandidates}
          filters={idleFilters}
          reservations={idleReservations.reservations}
          loadingReservations={idleReservations.loading}
          historyLoading={idleHistoryLoading}
          error={idleReservations.error}
          onFiltersChange={setIdleFilters}
          onCreateReservation={idleReservations.create}
          onReservationStatus={idleReservations.setStatus}
          onRemoveReservation={idleReservations.remove}
          onSelectNode={handleSelectIdleGpu}
          onRefresh={() => { refreshIdleHistory(); void idleReservations.refresh(); }}
        />
      ) : (
        <>
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
        <NodeDetail node={selectedNode} selectedGpu={selectedGpu} onSelectGpu={setSelectedGpu} onOpenTerminal={() => selectedNode && setTerminalNode(selectedNode)} />
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
        </>
      )}
      {terminalNode && user && <Suspense fallback={<div className="fixed inset-0 z-[80] flex items-center justify-center bg-[#2f2922]/25 p-4"><div className="soft-panel px-5 py-4 text-sm text-muted">{t('terminal.loading')}</div></div>}><SshTerminalDialog node={terminalNode} userId={user.id} onClose={() => setTerminalNode(null)} /></Suspense>}
    </div>
  );
}
