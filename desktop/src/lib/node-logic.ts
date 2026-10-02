import type {
  GpuInfo,
  NodeGPUSummary,
  NodeOverview,
  ProcessInfo,
  SystemMetrics,
} from '../../electron/types/ipc';

export type SortMode = 'availability' | 'name' | 'cpu' | 'users';

export const SORT_MODES: ReadonlyArray<{ value: SortMode; labelKey: string }> = [
  { value: 'availability', labelKey: 'panel.sortAvailability' },
  { value: 'name', labelKey: 'panel.sortName' },
  { value: 'cpu', labelKey: 'panel.sortCpu' },
  { value: 'users', labelKey: 'panel.sortUsers' },
];

const ACTIVE_USER_VRAM_THRESHOLD_MB = 0.1 * 1024;
const SYSTEM_USER_BLACKLIST = new Set(['root', 'gdm']);

/** Ported from the web dashboard: what counts as an active GPU process. */
export function isActiveGpuProcess(process: ProcessInfo): boolean {
  const user = (process.user || '').trim().toLowerCase();
  if (!user || SYSTEM_USER_BLACKLIST.has(user)) return false;
  if (process.gpu_index === null || process.gpu_index === undefined) return false;
  return (process.vram_used_mb ?? 0) >= ACTIVE_USER_VRAM_THRESHOLD_MB;
}

/** Extract the display host from an agent URL (host:port), falling back to the raw URL. */
export function hostFromUrl(rawUrl: string): string {
  try {
    const parsed = new URL(rawUrl);
    return parsed.host || rawUrl;
  } catch {
    return rawUrl;
  }
}

/**
 * Normalize an Agent URL for duplicate detection (web parity): lowercase
 * protocol + host, re-bracket IPv6 hosts, drop a bare root path.
 */
export function normalizeAgentUrl(raw: string): string | null {
  let parsed: URL;
  try {
    parsed = new URL(raw.trim());
  } catch {
    return null;
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return null;
  if (!parsed.hostname) return null;

  const host = parsed.hostname.toLowerCase();
  const displayHost = host.includes(':') ? `[${host}]` : host;
  const userInfo = parsed.username
    ? `${encodeURIComponent(parsed.username)}${parsed.password ? `:${encodeURIComponent(parsed.password)}` : ''}@`
    : '';
  const path = parsed.pathname === '/' ? '' : parsed.pathname;

  return `${parsed.protocol}//${userInfo}${displayHost}${parsed.port ? `:${parsed.port}` : ''}${path}${parsed.search}${parsed.hash}`;
}

export function gpuVramPercent(gpu: GpuInfo): number {
  if (gpu.memory_total > 0) {
    return Math.min(100, Math.max(0, (gpu.memory_used / gpu.memory_total) * 100));
  }
  return Math.min(100, Math.max(0, gpu.memory_utilization || 0));
}

/** Client-side GPU summary — the Gateway's gpuSummary takes precedence when present. */
export function computeGpuSummary(data: SystemMetrics | null | undefined): NodeGPUSummary {
  const gpus = data?.gpus ?? [];
  const processes = data?.processes ?? [];
  const activeIndexes = new Set<number>(
    processes.filter(isActiveGpuProcess).map((p) => p.gpu_index as number),
  );

  const gpuCount = gpus.length;
  let busyCount = 0;
  let utilSum = 0;
  let vramPercentSum = 0;
  let totalMemoryUsed = 0;
  let totalMemory = 0;
  let totalPower = 0;

  for (const gpu of gpus) {
    const vramPercent = gpuVramPercent(gpu);
    const busy = gpu.utilization >= 55 || vramPercent >= 60 || activeIndexes.has(gpu.id);
    if (busy) busyCount += 1;
    utilSum += gpu.utilization;
    vramPercentSum += vramPercent;
    totalMemoryUsed += gpu.memory_used;
    totalMemory += gpu.memory_total;
    totalPower += gpu.power_draw;
  }

  const avgUtilization = gpuCount > 0 ? utilSum / gpuCount : null;
  const avgMemoryPercent = gpuCount > 0 ? vramPercentSum / gpuCount : null;
  const gpuPressure =
    avgUtilization !== null && avgMemoryPercent !== null
      ? Math.min(100, Math.max(0, avgUtilization * 0.6 + avgMemoryPercent * 0.4))
      : 0;

  return {
    gpuCount,
    busyGpuCount: busyCount,
    idleGpuCount: gpuCount - busyCount,
    avgUtilization,
    avgMemoryPercent,
    totalMemoryUsed,
    totalMemory,
    totalPower,
    gpuPressure,
    busyRatio: gpuCount > 0 ? busyCount / gpuCount : 0,
  };
}

/**
 * Availability score formula ported verbatim from the web dashboard.
 * Only used as a fallback — the Gateway score wins when present.
 */
export function computeAvailabilityScore(input: {
  status: string;
  activeUserCount: number;
  cpuPercent: number | null;
  ramPercent: number | null;
  gpuSummary: NodeGPUSummary;
  dataAgeSec: number | null;
}): number {
  if (input.status !== 'online') return 0;

  let score = 100;
  score -= Math.min(30, input.activeUserCount * 8);
  score -= Math.min(24, (input.cpuPercent ?? null) !== null ? (input.cpuPercent as number) * 0.24 : 6);
  score -= Math.min(20, (input.ramPercent ?? null) !== null ? (input.ramPercent as number) * 0.2 : 6);
  score -= Math.min(22, input.gpuSummary.gpuPressure * 0.22);
  score -= Math.min(16, input.gpuSummary.busyRatio * 16);

  const gpuCount = input.gpuSummary.gpuCount;
  const avgPowerPerGpu = gpuCount > 0 ? input.gpuSummary.totalPower / gpuCount : 0;
  score -= Math.min(10, avgPowerPerGpu * 0.08);
  score += Math.min(12, input.gpuSummary.idleGpuCount * 3);
  if (gpuCount === 0) score -= 8;

  const dataAge = input.dataAgeSec;
  if (dataAge !== null && dataAge > 18) score -= 18;
  else if (dataAge !== null && dataAge > 8) score -= 8;

  return Math.round(Math.min(100, Math.max(0, score)));
}

export function getAvailabilityTier(status: string, score: number): string {
  if (status !== 'online') return 'offline';
  if (score >= 80) return 'highlyAvailable';
  if (score >= 60) return 'available';
  if (score >= 35) return 'busy';
  return 'saturated';
}

/** Enriched node model used across the dashboard UI. */
export interface EnrichedNode extends NodeOverview {
  online: boolean;
  hostDisplay: string;
  cpuPercent: number | null;
  ramPercent: number | null;
  networkDown: number;
  networkUp: number;
  effectiveActiveUsers: string[];
  effectiveUserCount: number;
  effectiveGpuSummary: NodeGPUSummary;
  effectiveAvailabilityScore: number;
  effectiveAvailabilityTier: string;
  effectiveDataAgeSec: number | null;
  activeProcesses: ProcessInfo[];
}

export function enrichNode(node: NodeOverview, nowMs: number): EnrichedNode {
  const data = node.data ?? null;
  const processes = data?.processes ?? [];

  const activeUsers =
    node.activeUsers ?? Array.from(new Set(processes.filter(isActiveGpuProcess).map((p) => p.user)));
  const activeUserCount = node.activeUserCount ?? activeUsers.length;

  const serverSummary = node.gpuSummary;
  const clientSummary = computeGpuSummary(data);
  const effectiveGpuSummary: NodeGPUSummary = {
    ...clientSummary,
    ...serverSummary,
    avgUtilization:
      serverSummary?.avgUtilization ?? clientSummary.avgUtilization,
    avgMemoryPercent:
      serverSummary?.avgMemoryPercent ?? clientSummary.avgMemoryPercent,
  };

  const fallbackDataAge =
    node.collectedAtUnix > 0 ? Math.max(0, (nowMs - node.collectedAtUnix) / 1000) : null;
  const effectiveDataAgeSec = node.dataAgeSec ?? fallbackDataAge;

  const effectiveAvailabilityScore =
    node.availabilityScore ??
    computeAvailabilityScore({
      status: node.status,
      activeUserCount,
      cpuPercent: data ? data.cpu_usage : null,
      ramPercent: data ? data.ram_percent : null,
      gpuSummary: effectiveGpuSummary,
      dataAgeSec: effectiveDataAgeSec,
    });
  const effectiveAvailabilityTier =
    node.availabilityTier ?? getAvailabilityTier(node.status, effectiveAvailabilityScore);

  return {
    ...node,
    online: node.status === 'online',
    hostDisplay: (data?.ip_address && data.ip_address.length > 0
      ? data.ip_address
      : hostFromUrl(node.url)) as string,
    cpuPercent: data ? data.cpu_usage : null,
    ramPercent: data ? data.ram_percent : null,
    networkDown: node.metrics?.downSpeed ?? 0,
    networkUp: node.metrics?.upSpeed ?? 0,
    effectiveActiveUsers: activeUsers,
    effectiveUserCount: activeUserCount,
    effectiveGpuSummary,
    effectiveAvailabilityScore,
    effectiveAvailabilityTier,
    effectiveDataAgeSec,
    activeProcesses: processes.filter(isActiveGpuProcess),
  };
}

/** Web sort semantics: online nodes first, then mode-specific ordering. */
export function sortNodes(nodes: EnrichedNode[], mode: SortMode): EnrichedNode[] {
  const onlineFirst = (a: EnrichedNode, b: EnrichedNode): number =>
    Number(b.online) - Number(a.online);

  const sorted = [...nodes];
  sorted.sort((a, b) => {
    const online = onlineFirst(a, b);
    if (online !== 0) return online;

    switch (mode) {
      case 'availability': {
        if (b.effectiveAvailabilityScore !== a.effectiveAvailabilityScore) {
          return b.effectiveAvailabilityScore - a.effectiveAvailabilityScore;
        }
        return a.name.localeCompare(b.name);
      }
      case 'name': {
        return a.name.localeCompare(b.name);
      }
      case 'cpu': {
        // Least loaded first; nodes without data go last.
        const aCpu = a.online ? a.cpuPercent : null;
        const bCpu = b.online ? b.cpuPercent : null;
        if (aCpu === null && bCpu === null) return a.name.localeCompare(b.name);
        if (aCpu === null) return 1;
        if (bCpu === null) return -1;
        if (aCpu !== bCpu) return aCpu - bCpu;
        return a.name.localeCompare(b.name);
      }
      case 'users': {
        if (a.effectiveUserCount !== b.effectiveUserCount) {
          return a.effectiveUserCount - b.effectiveUserCount;
        }
        return a.name.localeCompare(b.name);
      }
      default:
        return 0;
    }
  });
  return sorted;
}

export function matchesFilter(node: EnrichedNode, query: string): boolean {
  if (!query) return true;
  const haystack = `${node.name} ${node.url} ${node.hostDisplay}`.toLowerCase();
  return haystack.includes(query.toLowerCase());
}

export type GpuLoadTier = 'idle' | 'light' | 'busy' | 'full';

export function getGpuLoadTier(gpu: GpuInfo): GpuLoadTier {
  const load = Math.max(gpu.utilization, gpuVramPercent(gpu));
  if (load < 30) return 'idle';
  if (load < 60) return 'light';
  if (load < 85) return 'busy';
  return 'full';
}

export interface UserVramSummary {
  user: string;
  totalVramMb: number;
  processCount: number;
}

/** Per-user VRAM totals from one node's active processes, best first. */
export function summarizeNodeUsers(node: EnrichedNode, limit = 8): UserVramSummary[] {
  const byUser = new Map<string, UserVramSummary>();
  for (const process of node.activeProcesses) {
    const key = process.user || 'unknown';
    const entry = byUser.get(key) ?? { user: key, totalVramMb: 0, processCount: 0 };
    entry.totalVramMb += process.vram_used_mb ?? 0;
    entry.processCount += 1;
    byUser.set(key, entry);
  }
  return Array.from(byUser.values())
    .sort((a, b) => b.totalVramMb - a.totalVramMb)
    .slice(0, limit);
}

export interface DrawerUserGroup {
  user: string;
  totalVramMb: number;
  gpuCount: number;
  processes: Array<{ serverName: string; gpuIndex: number | null; pid: number; vramUsedMb: number }>;
}

/** Cross-node active user grouping for the GPU Users drawer. */
export function collectDrawerGroups(nodes: EnrichedNode[]): DrawerUserGroup[] {
  const byUser = new Map<string, DrawerUserGroup>();
  for (const node of nodes) {
    if (!node.online) continue;
    for (const process of node.activeProcesses) {
      const key = process.user || 'unknown';
      const group = byUser.get(key) ?? {
        user: key,
        totalVramMb: 0,
        gpuCount: 0,
        processes: [],
      };
      group.totalVramMb += process.vram_used_mb ?? 0;
      group.gpuCount += 1;
      group.processes.push({
        serverName: node.name,
        gpuIndex: process.gpu_index ?? null,
        pid: process.pid,
        vramUsedMb: process.vram_used_mb ?? 0,
      });
      byUser.set(key, group);
    }
  }
  return Array.from(byUser.values())
    .filter((group) => group.totalVramMb >= ACTIVE_USER_VRAM_THRESHOLD_MB)
    .sort((a, b) => b.totalVramMb - a.totalVramMb);
}

/** Tier pill and score-bar palettes ported from the web dashboard. */
export const TIER_PILL_STYLES: Record<string, string> = {
  highlyAvailable: 'bg-[#dcefe5] text-[#2f655a] border-[#c8dfd4]',
  available: 'bg-[#deece8] text-[#345f59] border-[#cadfd9]',
  busy: 'bg-[#f0e9dd] text-[#6c5a3f] border-[#e0d2bc]',
  saturated: 'bg-[#f2e1de] text-[#7a4740] border-[#e3c9c4]',
  offline: 'bg-[#ebe5dc] text-[#6e665d] border-[#dad0c4]',
};

export const TIER_BAR_COLORS: Record<string, string> = {
  highlyAvailable: '#4f8f82',
  available: '#5f9189',
  busy: '#b38b54',
  saturated: '#ad6158',
  offline: '#9a948b',
};

export const GPU_TIER_STYLES: Record<GpuLoadTier, string> = {
  idle: 'bg-[#dcefe5] text-[#2f655a] border-[#c8dfd4]',
  light: 'bg-[#e5efe9] text-[#335f56] border-[#cfdfd7]',
  busy: 'bg-[#efe7da] text-[#6a583e] border-[#dfd0ba]',
  full: 'bg-[#f2e1de] text-[#7a4740] border-[#e3c9c4]',
};
