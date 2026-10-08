import { beforeEach, describe, expect, it } from 'vitest';
import type { EnrichedNode } from '../src/lib/node-logic';
import type { IdleReservationFilters, NodeHistorySample } from '../electron/types/ipc';
import { DEFAULT_IDLE_GPU_FILTERS, hasStableIdleWindow, loadIdleGpuFilters, rankIdleGpuCandidates, saveIdleGpuFilters } from '../src/lib/idle-gpu';

const filters: IdleReservationFilters = {
  ...DEFAULT_IDLE_GPU_FILTERS,
  minFreeVramGb: 40,
  minFreeSystemMemoryGb: 32,
  idleDurationMinutes: 5,
};

describe('idle GPU discovery', () => {
  beforeEach(() => Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: memoryStorage() }));

  it('ranks GPUs that satisfy current resource, process and stable-idle filters', () => {
    const now = 1_800_000_000;
    const gpuNode = node({ utilization: 4, memoryUsed: 2, processCount: 0 });
    const history = { 4: [sample(now - 300), sample(now - 150), sample(now)] };
    const result = rankIdleGpuCandidates([gpuNode], history, filters, now);
    expect(result).toHaveLength(1);
    expect(result[0].matches).toBe(true);
    expect(result[0].freeVramGb).toBe(78);
    expect(result[0].freeSystemMemoryGb).toBe(128);
  });

  it('rejects a GPU that has an active GPU process when empty-only is requested', () => {
    const now = 1_800_000_000;
    const gpuNode = node({ utilization: 4, memoryUsed: 2, processCount: 1, processes: [{
      pid: 42, user: 'worker', command: 'train', cpu_percent: 1, memory_percent: 1, gpu_index: 0, vram_used_mb: 2048,
    }] });
    const result = rankIdleGpuCandidates([gpuNode], { 4: [sample(now - 300), sample(now)] }, filters, now);
    expect(result[0].matches).toBe(false);
    expect(result[0].activeGpuProcessCount).toBe(1);
  });

  it('requires complete history coverage for a duration filter', () => {
    const now = 1_800_000_000;
    expect(hasStableIdleWindow([sample(now - 100), sample(now)], 0, filters, now, 256)).toBe(false);
    expect(hasStableIdleWindow([sample(now - 300), sample(now - 150), sample(now)], 0, filters, now, 256)).toBe(true);
  });

  it('rejects offline or resource-deficient observations within the sustained window', () => {
    const now = 1_800_000_000;
    const points = Array.from({ length: 21 }, (_, index) => sample(now - 300 + index * 15));
    points[10] = { ...points[10], status: 'offline' };
    expect(hasStableIdleWindow(points, 0, filters, now, 256)).toBe(false);
    points[10] = { ...sample(now - 150), ramPercent: 99 };
    expect(hasStableIdleWindow(points, 0, filters, now, 256)).toBe(false);
    points[10] = sample(now - 150);
    points[10].gpus![0].memoryUsed = 60;
    expect(hasStableIdleWindow(points, 0, filters, now, 256)).toBe(false);
  });

  it('supports CPU-model, utilization and partial free-memory criteria', () => {
    const gpuNode = node({ utilization: 20, memoryUsed: 30, processCount: 0 });
    const current = { ...DEFAULT_IDLE_GPU_FILTERS, minFreeVramGb: 40, maxGpuUtilization: 25, cpuModel: 'CPU', processPolicy: 'any' as const };
    expect(rankIdleGpuCandidates([gpuNode], {}, current, 1_800_000_000)[0].matches).toBe(true);
    expect(rankIdleGpuCandidates([gpuNode], {}, { ...current, maxGpuUtilization: 10 }, 1_800_000_000)[0].matches).toBe(false);
    expect(rankIdleGpuCandidates([gpuNode], {}, { ...current, cpuModel: 'Other CPU' }, 1_800_000_000)).toEqual([]);
  });

  it('persists and normalizes idle filters', () => {
    const value = { ...DEFAULT_IDLE_GPU_FILTERS, minFreeVramGb: 48, nodeIds: [4] };
    saveIdleGpuFilters(value);
    expect(loadIdleGpuFilters()).toEqual(value);
    localStorage.setItem('nexus.idleGpuFilters.v1', JSON.stringify({ minFreeVramGb: -3, idleDurationMinutes: 99 }));
    expect(loadIdleGpuFilters()).toMatchObject({ minFreeVramGb: 0, idleDurationMinutes: 0 });
  });
});

function node(input: { utilization: number; memoryUsed: number; processCount: number; processes?: EnrichedNode['data'] extends infer T ? T extends { processes: infer P } ? P : never : never }): EnrichedNode {
  return {
    id: 4, name: 'gpu-node', url: 'ssh://collector@10.0.0.4:22', status: 'online', online: true,
    sshHost: '10.0.0.4', sshPort: 22, lastPolledAtUnix: 1_800_000_000, collectedAtUnix: 1_800_000_000,
    activeUsers: [], activeUserCount: 0, dataAgeSec: 0, error: '',
    gpuSummary: { gpuCount: 1, busyGpuCount: 0, idleGpuCount: 1, avgUtilization: input.utilization, avgMemoryPercent: 2.5, totalMemoryUsed: input.memoryUsed, totalMemory: 80, totalPower: 0, gpuPressure: 0, busyRatio: 0 },
    availabilityScore: 90, availabilityTier: 'highlyAvailable', effectiveAvailabilityScore: 90, effectiveAvailabilityTier: 'highlyAvailable',
    hostDisplay: '10.0.0.4', cpuPercent: 12, ramPercent: 50, networkDown: 0, networkUp: 0, effectiveActiveUsers: [], effectiveUserCount: 0, effectiveGpuSummary: { gpuCount: 1, busyGpuCount: 0, idleGpuCount: 1, avgUtilization: input.utilization, avgMemoryPercent: 2.5, totalMemoryUsed: input.memoryUsed, totalMemory: 80, totalPower: 0, gpuPressure: 0, busyRatio: 0 }, effectiveDataAgeSec: 0, activeProcesses: [],
    data: {
      hostname: 'gpu-node', ip_address: '10.0.0.4', os: 'linux', uptime_seconds: 1, uptime_human: '1m', cpu_model: 'CPU', cpu_usage: 12, cpu_cores: 32, ram_total: 256, ram_used: 128, ram_percent: 50, net_sent_mb: 0, net_recv_mb: 0,
      gpus: [{ id: 0, name: 'Test GPU', temperature: 52, fan_speed: 0, power_draw: 0, utilization: input.utilization, memory_total: 80, memory_used: input.memoryUsed, memory_utilization: 2.5 }],
      processes: input.processes ?? [],
    },
  };
}

function sample(timestampUnix: number): NodeHistorySample {
  return {
    timestampUnix, nodeId: 4, nodeName: 'gpu-node', status: 'online', availabilityScore: 90, availabilityTier: 'highlyAvailable', cpuUsage: 12, ramPercent: 50,
    gpuSummary: { gpuCount: 1, busyGpuCount: 0, idleGpuCount: 1, avgUtilization: 4, avgMemoryPercent: 2.5, totalMemoryUsed: 2, totalMemory: 80, totalPower: 0, gpuPressure: 0, busyRatio: 0 },
    activeUserCount: 0, gpus: [{ id: 0, name: 'Test GPU', utilization: 4, memoryUsed: 2, memoryTotal: 80, temperature: 52, powerDraw: 0, processCount: 0 }],
  };
}

function memoryStorage(): Storage {
  const values = new Map<string, string>();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => { values.set(String(key), String(value)); },
    removeItem: (key) => { values.delete(key); },
    clear: () => values.clear(),
    key: (index) => [...values.keys()][index] ?? null,
    get length() { return values.size; },
  };
}
