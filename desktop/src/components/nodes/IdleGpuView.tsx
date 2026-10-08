import { useMemo, useState } from 'react';
import { BellPlus, CheckCircle2, Pause, Play, RefreshCw, Trash2, Zap } from 'lucide-react';
import type { EnrichedNode } from '../../lib/node-logic';
import type { IdleReservation, IdleReservationFilters } from '../../../electron/types/ipc';
import type { NexusError } from '../../../electron/types/ipc';
import type { IdleGpuCandidate } from '../../lib/idle-gpu';
import { useLanguage } from '../../hooks/useLanguage';
import { localizedError } from '../../lib/errors';
import AppModal from '../common/AppModal';

interface IdleGpuViewProps {
  nodes: EnrichedNode[];
  candidates: IdleGpuCandidate[];
  filters: IdleReservationFilters;
  reservations: IdleReservation[];
  loadingReservations: boolean;
  historyLoading: boolean;
  error: NexusError | null;
  onFiltersChange: (filters: IdleReservationFilters) => void;
  onCreateReservation: (input: { name: string; filters: IdleReservationFilters; notifyMode: 'once' | 'continuous'; expiresInHours: 0 | 1 | 4 | 8 | 24 | 72 }) => Promise<boolean>;
  onReservationStatus: (id: number, status: 'active' | 'paused') => void;
  onRemoveReservation: (id: number) => void;
  onSelectNode: (nodeId: number, gpuId: number) => void;
  onRefresh: () => void;
}

const DURATIONS = [0, 5, 10, 30, 60] as const;
const EXPIRATIONS = [1, 4, 8, 24, 72] as const;

export default function IdleGpuView({
  nodes,
  candidates,
  filters,
  reservations,
  loadingReservations,
  historyLoading,
  error,
  onFiltersChange,
  onCreateReservation,
  onReservationStatus,
  onRemoveReservation,
  onSelectNode,
  onRefresh,
}: IdleGpuViewProps) {
  const { t } = useLanguage();
  const [reservationOpen, setReservationOpen] = useState(false);
  const [reservationName, setReservationName] = useState('');
  const [notifyMode, setNotifyMode] = useState<'once' | 'continuous'>('once');
  const [expiresInHours, setExpiresInHours] = useState<0 | (typeof EXPIRATIONS)[number]>(24);
  const [saving, setSaving] = useState(false);
  const gpuModels = useMemo(() => [...new Set(nodes.flatMap((node) => node.data?.gpus.map((gpu) => gpu.name) ?? []))].sort(), [nodes]);
  const cpuModels = useMemo(() => [...new Set(nodes.flatMap((node) => node.data?.cpu_model ? [node.data.cpu_model] : []))].sort(), [nodes]);
  const matchingCandidates = candidates.filter((candidate) => candidate.matches);

  async function saveReservation() {
    const name = reservationName.trim() || t('idle.defaultReservationName');
    setSaving(true);
    try {
      const saved = await onCreateReservation({ name, filters, notifyMode, expiresInHours });
      if (saved) {
        setReservationOpen(false);
        setReservationName('');
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <section className="soft-panel p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p className="eyebrow">{t('idle.eyebrow')}</p>
            <h2 className="mt-1 text-lg font-semibold text-ink">{t('idle.title')}</h2>
            <p className="mt-1 text-xs text-muted">{t('idle.subtitle')}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button type="button" className="muted-button" onClick={onRefresh}>
              <RefreshCw size={14} /> {t('idle.refresh')}
            </button>
            <button type="button" className="primary-button" onClick={() => setReservationOpen(true)}>
              <BellPlus size={15} /> {t('idle.createReservation')}
            </button>
          </div>
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('idle.minFreeVram')}
            <input className="input-field" type="number" min={0} step={0.5} value={filters.minFreeVramGb} onChange={(event) => onFiltersChange({ ...filters, minFreeVramGb: nonNegative(event.target.value) })} />
          </label>
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('idle.minFreeSystemMemory')}
            <input className="input-field" type="number" min={0} step={1} value={filters.minFreeSystemMemoryGb} onChange={(event) => onFiltersChange({ ...filters, minFreeSystemMemoryGb: nonNegative(event.target.value) })} />
          </label>
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('idle.gpuModel')}
            <select className="input-field" value={filters.gpuModel} onChange={(event) => onFiltersChange({ ...filters, gpuModel: event.target.value })}>
              <option value="">{t('idle.anyModel')}</option>
              {gpuModels.map((model) => <option key={model} value={model}>{model}</option>)}
            </select>
          </label>
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('idle.processPolicy')}
            <select className="input-field" value={filters.processPolicy} onChange={(event) => onFiltersChange({ ...filters, processPolicy: event.target.value as IdleReservationFilters['processPolicy'] })}>
              <option value="emptyOnly">{t('idle.noGpuProcesses')}</option>
              <option value="any">{t('idle.processesAny')}</option>
            </select>
          </label>
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('idle.cpuModel')}
            <select className="input-field" value={filters.cpuModel} onChange={(event) => onFiltersChange({ ...filters, cpuModel: event.target.value })}>
              <option value="">{t('idle.anyModel')}</option>
              {cpuModels.map((model) => <option key={model} value={model}>{model}</option>)}
            </select>
          </label>
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('idle.maxUtilization')}
            <input className="input-field" type="number" min={0} max={100} value={filters.maxGpuUtilization} onChange={(event) => onFiltersChange({ ...filters, maxGpuUtilization: Math.min(100, nonNegative(event.target.value)) })} />
          </label>
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('idle.stableDuration')}
            <select className="input-field" value={filters.idleDurationMinutes} onChange={(event) => onFiltersChange({ ...filters, idleDurationMinutes: Number(event.target.value) as IdleReservationFilters['idleDurationMinutes'] })}>
              {DURATIONS.map((duration) => <option key={duration} value={duration}>{duration === 0 ? t('idle.currentSnapshot') : t('idle.minutes', { count: duration })}</option>)}
            </select>
          </label>
          <label className="flex flex-col gap-1.5 text-xs text-muted md:col-span-2 xl:col-span-2">
            {t('idle.node')}
            <select className="input-field" value={filters.nodeIds[0] ?? ''} onChange={(event) => onFiltersChange({ ...filters, nodeIds: event.target.value ? [Number(event.target.value)] : [] })}>
              <option value="">{t('idle.allNodes')}</option>
              {nodes.map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}
            </select>
          </label>
          <div className="flex items-end text-xs text-muted md:col-span-2 xl:col-span-3">
            {historyLoading ? t('idle.historyLoading') : t('idle.matchCount', { count: matchingCandidates.length })}
          </div>
        </div>
        <p className="mt-3 text-[11px] text-muted">{t('idle.reservationIsReminder')}</p>
      </section>

      {error && <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-2 text-xs text-red-800" role="alert">{localizedError(error, t)}</div>}

      <section className="soft-panel min-h-0 flex-1 overflow-hidden p-5">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="eyebrow">{t('idle.availableGpus')}</h3>
          <span className="text-xs text-muted">{matchingCandidates.length}/{candidates.length}</span>
        </div>
        <div className="custom-scrollbar max-h-[42vh] overflow-y-auto">
          {matchingCandidates.length === 0 ? (
            <div className="flex min-h-36 flex-col items-center justify-center gap-2 text-sm text-muted">
              <Zap size={20} />
              <span>{historyLoading ? t('idle.historyLoading') : t('idle.noneAvailable')}</span>
            </div>
          ) : (
            <div className="grid gap-2 lg:grid-cols-2">
              {matchingCandidates.map((candidate) => (
                <button key={candidate.key} type="button" className="soft-panel-subtle flex items-center justify-between gap-3 p-4 text-left transition hover:border-[#a8cbbb]" onClick={() => onSelectNode(candidate.node.id, candidate.gpu.id)}>
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-semibold text-ink">{candidate.node.name} · GPU {candidate.gpu.id}</span>
                    <span className="mt-1 block truncate text-xs text-muted">{candidate.gpu.name} · {candidate.activeGpuProcessCount === 0 ? t('idle.noGpuProcesses') : t('idle.processCount', { count: candidate.activeGpuProcessCount })}</span>
                  </span>
                  <span className="shrink-0 text-right font-mono text-xs text-muted">
                    <span className="block text-ink">{candidate.freeVramGb.toFixed(1)} GB {t('idle.freeVram')}</span>
                    <span className="mt-1 block">RAM {candidate.freeSystemMemoryGb.toFixed(1)} GB</span>
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
      </section>

      <section className="soft-panel p-5">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="eyebrow">{t('idle.reservations')}</h3>
          {loadingReservations && <span className="text-xs text-muted">{t('idle.loadingReservations')}</span>}
        </div>
        {reservations.length === 0 ? <p className="text-xs text-muted">{t('idle.noReservations')}</p> : (
          <div className="grid gap-2 lg:grid-cols-2">
            {reservations.map((reservation) => <ReservationRow key={reservation.id} reservation={reservation} t={t} onStatus={onReservationStatus} onRemove={onRemoveReservation} />)}
          </div>
        )}
      </section>

      <AppModal open={reservationOpen} onClose={() => setReservationOpen(false)} title={t('idle.createReservation')} subtitle={t('idle.reservationIsReminder')}>
        <div className="flex flex-col gap-3">
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('idle.reservationName')}
            <input className="input-field" maxLength={64} value={reservationName} onChange={(event) => setReservationName(event.target.value)} placeholder={t('idle.defaultReservationName')} />
          </label>
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('idle.notifyMode')}
            <select className="input-field" value={notifyMode} onChange={(event) => setNotifyMode(event.target.value as 'once' | 'continuous')}>
              <option value="once">{t('idle.notifyOnce')}</option>
              <option value="continuous">{t('idle.notifyAgain')}</option>
            </select>
          </label>
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('idle.expiresIn')}
            <select className="input-field" value={expiresInHours} onChange={(event) => setExpiresInHours(Number(event.target.value) as 0 | (typeof EXPIRATIONS)[number])}>
              {EXPIRATIONS.map((hours) => <option key={hours} value={hours}>{t('idle.hours', { count: hours })}</option>)}
              <option value={0}>{t('idle.neverExpires')}</option>
            </select>
          </label>
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" className="muted-button" onClick={() => setReservationOpen(false)} disabled={saving}>{t('action.cancel')}</button>
            <button type="button" className="primary-button" onClick={() => void saveReservation()} disabled={saving}>{saving ? t('idle.saving') : t('idle.saveReservation')}</button>
          </div>
        </div>
      </AppModal>
    </div>
  );
}

function ReservationRow({ reservation, t, onStatus, onRemove }: {
  reservation: IdleReservation;
  t: ReturnType<typeof useLanguage>['t'];
  onStatus: (id: number, status: 'active' | 'paused') => void;
  onRemove: (id: number) => void;
}) {
  const canToggle = reservation.status === 'active' || reservation.status === 'paused';
  return (
    <div className="soft-panel-subtle flex items-center justify-between gap-3 p-3">
      <div className="min-w-0">
        <div className="flex items-center gap-2 text-sm font-semibold text-ink">
          {reservation.status === 'completed' ? <CheckCircle2 size={14} /> : <BellPlus size={14} />}
          <span className="truncate">{reservation.name}</span>
          <span className="rounded-full bg-panel px-2 py-0.5 text-[10px] font-medium text-muted">{t(`idle.status.${reservation.status}`)}</span>
        </div>
        <p className="mt-1 truncate text-[11px] text-muted">{reservation.filters.minFreeVramGb} GB VRAM · {reservation.filters.idleDurationMinutes ? t('idle.minutes', { count: reservation.filters.idleDurationMinutes }) : t('idle.currentSnapshot')} · {t('idle.currentMatches', { count: reservation.currentMatchKeys.length })}</p>
      </div>
      <div className="flex shrink-0 gap-1">
        {canToggle && <button type="button" className="muted-button !px-2 !py-1.5" title={reservation.status === 'active' ? t('idle.pause') : t('idle.resume')} onClick={() => onStatus(reservation.id, reservation.status === 'active' ? 'paused' : 'active')}>
          {reservation.status === 'active' ? <Pause size={14} /> : <Play size={14} />}
        </button>}
        <button type="button" className="muted-button !px-2 !py-1.5 text-red-700" title={t('idle.delete')} onClick={() => onRemove(reservation.id)}><Trash2 size={14} /></button>
      </div>
    </div>
  );
}

function nonNegative(value: string): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(0, parsed) : 0;
}
