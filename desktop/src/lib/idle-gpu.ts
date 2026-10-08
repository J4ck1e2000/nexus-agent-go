import type { EnrichedNode } from './node-logic';
import type { IdleReservationFilters, NodeHistorySample } from '../../electron/types/ipc';

export interface IdleGpuCandidate {
  key: string;
  node: EnrichedNode;
  gpu: NonNullable<EnrichedNode['data']>['gpus'][number];
  freeVramGb: number;
  freeSystemMemoryGb: number;
  activeGpuProcessCount: number;
  matches: boolean;
}

export type NodeHistoryById = Record<number, NodeHistorySample[]>;

const IDLE_GPU_UTILIZATION_PERCENT = 10;
const IDLE_GPU_MEMORY_PERCENT = 3;
const PROCESS_VRAM_THRESHOLD_MB = 100;
const MAX_HISTORY_GAP_SECONDS = 180;
const IDLE_FILTERS_KEY = 'nexus.idleGpuFilters.v1';

export const DEFAULT_IDLE_GPU_FILTERS: IdleReservationFilters = {
  minFreeVramGb: 0,
  minFreeSystemMemoryGb: 0,
  gpuModel: '',
  cpuModel: '',
  maxGpuUtilization: 10,
  processPolicy: 'emptyOnly',
  nodeIds: [],
  idleDurationMinutes: 0,
};

export function loadIdleGpuFilters(): IdleReservationFilters {
  if (typeof localStorage === 'undefined') return { ...DEFAULT_IDLE_GPU_FILTERS };
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(IDLE_FILTERS_KEY) ?? '{}');
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return { ...DEFAULT_IDLE_GPU_FILTERS };
    const value = raw as Partial<IdleReservationFilters>;
    const duration = value.idleDurationMinutes;
    const nodeIds = Array.isArray(value.nodeIds) ? value.nodeIds.filter((id): id is number => typeof id === 'number' && Number.isInteger(id) && id > 0) : [];
    return {
      minFreeVramGb: nonNegative(value.minFreeVramGb, 1024),
      minFreeSystemMemoryGb: nonNegative(value.minFreeSystemMemoryGb, 16384),
      gpuModel: typeof value.gpuModel === 'string' ? value.gpuModel.slice(0, 128) : '',
      cpuModel: typeof value.cpuModel === 'string' ? value.cpuModel.slice(0, 256) : '',
      maxGpuUtilization: value.maxGpuUtilization === undefined ? 10 : nonNegative(value.maxGpuUtilization, 100),
      processPolicy: value.processPolicy === 'any' ? 'any' : 'emptyOnly',
      nodeIds: [...new Set(nodeIds)],
      idleDurationMinutes: duration === 5 || duration === 10 || duration === 30 || duration === 60 ? duration : 0,
    };
  } catch {
    return { ...DEFAULT_IDLE_GPU_FILTERS };
  }
}

export function saveIdleGpuFilters(filters: IdleReservationFilters): void {
  try {
    localStorage.setItem(IDLE_FILTERS_KEY, JSON.stringify(filters));
  } catch {
    // Search filters are only a convenience; the view remains usable without storage.
  }
}

export function idleGpuKey(nodeId: number, gpuId: number): string {
  return `${nodeId}:${gpuId}`;
}

export function rankIdleGpuCandidates(
  nodes: EnrichedNode[],
  historyByNode: NodeHistoryById,
  filters: IdleReservationFilters,
  nowUnix = Math.floor(Date.now() / 1000),
): IdleGpuCandidate[] {
  const selectedNodes = filters.nodeIds.length > 0 ? new Set(filters.nodeIds) : null;
  return nodes.flatMap((node) => {
    if (!node.online || !node.data || (selectedNodes && !selectedNodes.has(node.id))
      || nowUnix - node.collectedAtUnix > MAX_HISTORY_GAP_SECONDS
      || (filters.cpuModel && node.data.cpu_model !== filters.cpuModel)) return [];
    const freeSystemMemoryGb = Math.max(0, node.data.ram_total - node.data.ram_used);
    return node.data.gpus.map((gpu): IdleGpuCandidate => {
      const freeVramGb = Math.max(0, gpu.memory_total - gpu.memory_used);
      const activeGpuProcessCount = node.data?.processes.filter((process) =>
        process.gpu_index === gpu.id && (process.vram_used_mb ?? 0) >= PROCESS_VRAM_THRESHOLD_MB,
      ).length ?? 0;
      const currentIdle = gpu.memory_total > 0 && gpu.utilization <= filters.maxGpuUtilization
        && (filters.processPolicy !== 'emptyOnly' || activeGpuProcessCount === 0);
      const historicalIdle = filters.idleDurationMinutes === 0 || hasStableIdleWindow(
        historyByNode[node.id] ?? [],
        gpu.id,
        filters,
        nowUnix,
        node.data?.ram_total ?? 0,
      );
      const matches = freeVramGb >= filters.minFreeVramGb
        && freeSystemMemoryGb >= filters.minFreeSystemMemoryGb
        && (filters.gpuModel === '' || filters.gpuModel === gpu.name)
        && currentIdle
        && historicalIdle;
      return {
        key: idleGpuKey(node.id, gpu.id),
        node,
        gpu,
        freeVramGb,
        freeSystemMemoryGb,
        activeGpuProcessCount,
        matches,
      };
    });
  }).sort((left, right) => Number(right.matches) - Number(left.matches)
    || right.freeVramGb - left.freeVramGb
    || left.node.name.localeCompare(right.node.name)
    || left.gpu.id - right.gpu.id);
}

export function isIdleGpuMetric(utilization: number, memoryUsed: number, memoryTotal: number): boolean {
  const memoryPercent = memoryTotal > 0 ? (memoryUsed / memoryTotal) * 100 : 100;
  return utilization <= IDLE_GPU_UTILIZATION_PERCENT && memoryPercent <= IDLE_GPU_MEMORY_PERCENT;
}

function nonNegative(value: unknown, maximum: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, Math.min(maximum, value)) : 0;
}

export function hasStableIdleWindow(
  samples: NodeHistorySample[],
  gpuId: number,
  filters: IdleReservationFilters,
  nowUnix: number,
  systemMemoryTotalGb = 0,
): boolean {
  const durationSeconds = filters.idleDurationMinutes * 60;
  if (durationSeconds <= 0) return true;
  const cutoff = nowUnix - durationSeconds;
  const points = samples
    .filter((sample) => sample.timestampUnix >= cutoff - MAX_HISTORY_GAP_SECONDS && sample.timestampUnix <= nowUnix)
    .sort((left, right) => left.timestampUnix - right.timestampUnix);
  const gaps = points.slice(1).map((point, index) => point.timestampUnix - points[index].timestampUnix).filter((gap) => gap > 0 && gap <= MAX_HISTORY_GAP_SECONDS).sort((a, b) => a - b);
  const coverageTolerance = Math.min(MAX_HISTORY_GAP_SECONDS, Math.max(30, (gaps[Math.floor(gaps.length / 2)] ?? 15) * 3));
  if (points.length < 2 || points[0].timestampUnix > cutoff + coverageTolerance || points[points.length - 1].timestampUnix < nowUnix - coverageTolerance) {
    return false;
  }
  // Keep only the closest observation before the window; older buffer samples
  // must not make an otherwise valid window fail.
  const preceding = points.filter((point) => point.timestampUnix < cutoff).at(-1);
  const windowPoints = points.filter((point) => point.timestampUnix >= cutoff);
  if (preceding) windowPoints.unshift(preceding);
  for (let index = 0; index < windowPoints.length; index += 1) {
    const point = windowPoints[index];
    const gpu = point.gpus?.find((item) => item.id === gpuId);
    if (point.status !== 'online' || !gpu || gpu.memoryTotal <= 0 || gpu.utilization > filters.maxGpuUtilization
      || gpu.memoryTotal - gpu.memoryUsed < filters.minFreeVramGb) return false;
    if (filters.minFreeSystemMemoryGb > 0 && (point.ramPercent === undefined || systemMemoryTotalGb <= 0
      || systemMemoryTotalGb * (1 - point.ramPercent / 100) < filters.minFreeSystemMemoryGb)) return false;
    if (filters.processPolicy === 'emptyOnly' && (typeof gpu.processCount !== 'number' || gpu.processCount > 0)) return false;
    const next = windowPoints[index + 1];
    if (next && next.timestampUnix - point.timestampUnix > coverageTolerance) return false;
  }
  return true;
}
