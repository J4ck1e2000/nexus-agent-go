import { describe, expect, it } from 'vitest';
import type { NodeOverview, SystemMetrics } from '../electron/types/ipc';
import {
  computeAvailabilityScore,
  computeGpuSummary,
  enrichNode,
  getAvailabilityTier,
  getGpuLoadTier,
  hostFromUrl,
  isActiveGpuProcess,
  matchesFilter,
  normalizeAgentUrl,
  sortNodes,
} from '../src/lib/node-logic';
import { formatAgeShort, formatSpeed } from '../src/lib/format';

function gpu(partial: Partial<SystemMetrics['gpus'][number]>): SystemMetrics['gpus'][number] {
  return {
    id: 0,
    name: 'NVIDIA GeForce RTX 4090',
    temperature: 50,
    fan_speed: 40,
    power_draw: 120,
    utilization: 10,
    memory_total: 24,
    memory_used: 2,
    memory_utilization: 8,
    ...partial,
  };
}

function metrics(partial: Partial<SystemMetrics> = {}): SystemMetrics {
  return {
    hostname: 'node-a',
    ip_address: '10.0.0.2',
    os: 'linux',
    uptime_seconds: 100,
    uptime_human: '1m',
    cpu_model: 'test',
    cpu_usage: 20,
    cpu_cores: 8,
    ram_total: 64,
    ram_used: 16,
    ram_percent: 25,
    net_sent_mb: 0.5,
    net_recv_mb: 2,
    gpus: [],
    processes: [],
    ...partial,
  };
}

function overview(partial: Partial<NodeOverview> = {}): NodeOverview {
  return {
    id: 1,
    name: 'node-a',
    url: 'http://10.0.0.2:8005',
    status: 'online',
    data: metrics(),
    metrics: { upSpeed: 1, downSpeed: 2 },
    lastSeenAt: Date.now(),
    lastPolledAtUnix: 0,
    collectedAtUnix: Date.now() - 1000,
    activeUsers: ['alice'],
    activeUserCount: 1,
    gpuSummary: {
      gpuCount: 0,
      busyGpuCount: 0,
      idleGpuCount: 0,
      avgUtilization: null,
      avgMemoryPercent: null,
      totalMemoryUsed: 0,
      totalMemory: 0,
      totalPower: 0,
      gpuPressure: 0,
      busyRatio: 0,
    },
    availabilityScore: 88,
    availabilityTier: 'highlyAvailable',
    dataAgeSec: 1,
    ...partial,
  };
}

describe('isActiveGpuProcess', () => {
  it('requires a real user, a GPU index and >= 0.1 GB VRAM', () => {
    expect(
      isActiveGpuProcess({ pid: 1, user: 'alice', command: 'python', cpu_percent: 0, memory_percent: 0, gpu_index: 0, vram_used_mb: 1024 }),
    ).toBe(true);
    expect(
      isActiveGpuProcess({ pid: 1, user: 'alice', command: 'python', cpu_percent: 0, memory_percent: 0, gpu_index: 0, vram_used_mb: 100 }),
    ).toBe(false);
    expect(
      isActiveGpuProcess({ pid: 1, user: 'root', command: 'x', cpu_percent: 0, memory_percent: 0, gpu_index: 0, vram_used_mb: 1024 }),
    ).toBe(false);
    expect(
      isActiveGpuProcess({ pid: 1, user: 'alice', command: 'x', cpu_percent: 0, memory_percent: 0, gpu_index: null, vram_used_mb: 1024 }),
    ).toBe(false);
  });
});

describe('computeGpuSummary', () => {
  it('counts busy GPUs by utilization, VRAM and active processes', () => {
    const data = metrics({
      gpus: [
        gpu({ id: 0, utilization: 90, memory_used: 20, memory_total: 24 }),
        gpu({ id: 1, utilization: 5, memory_used: 1, memory_total: 24 }),
        gpu({ id: 2, utilization: 5, memory_used: 1, memory_total: 24 }),
      ],
      processes: [
        { pid: 9, user: 'bob', command: 'train', cpu_percent: 0, memory_percent: 0, gpu_index: 2, vram_used_mb: 2048 },
      ],
    });
    const summary = computeGpuSummary(data);
    expect(summary.gpuCount).toBe(3);
    expect(summary.busyGpuCount).toBe(2);
    expect(summary.idleGpuCount).toBe(1);
    expect(summary.totalMemory).toBeCloseTo(72);
    expect(summary.totalPower).toBeCloseTo(360);
  });
});

describe('availability', () => {
  it('scores offline nodes as zero', () => {
    expect(
      computeAvailabilityScore({
        status: 'offline',
        activeUserCount: 0,
        cpuPercent: 0,
        ramPercent: 0,
        gpuSummary: computeGpuSummary(null),
        dataAgeSec: 1,
      }),
    ).toBe(0);
  });

  it('maps scores to tiers like the web dashboard', () => {
    expect(getAvailabilityTier('online', 85)).toBe('highlyAvailable');
    expect(getAvailabilityTier('online', 70)).toBe('available');
    expect(getAvailabilityTier('online', 40)).toBe('busy');
    expect(getAvailabilityTier('online', 10)).toBe('saturated');
    expect(getAvailabilityTier('offline', 100)).toBe('offline');
  });
});

describe('enrichNode', () => {
  it('prefers the gateway payload and falls back to client derivations', () => {
    const node = enrichNode(overview({ activeUsers: null, activeUserCount: null }), Date.now());
    expect(node.hostDisplay).toBe('10.0.0.2');
    expect(node.effectiveActiveUsers).toEqual([]);
    expect(node.effectiveUserCount).toBe(0);
    expect(node.effectiveAvailabilityScore).toBe(88);
  });

  it('falls back to URL host when no IP is reported', () => {
    const node = enrichNode(overview({ data: metrics({ ip_address: '' }) }), Date.now());
    expect(node.hostDisplay).toBe('10.0.0.2:8005');
  });
});

describe('sorting and filtering', () => {
  it('sorts by availability score descending with online nodes first', () => {
    const a = enrichNode(overview({ id: 1, name: 'a', availabilityScore: 50 }), 0);
    const b = enrichNode(overview({ id: 2, name: 'b', availabilityScore: 90 }), 0);
    const c = enrichNode(overview({ id: 3, name: 'c', status: 'offline', availabilityScore: 0 }), 0);
    const sorted = sortNodes([c, a, b], 'availability');
    expect(sorted.map((node) => node.id)).toEqual([2, 1, 3]);
  });

  it('sorts by CPU ascending (least loaded first) and pushes data-less nodes last', () => {
    const a = enrichNode(overview({ id: 1, name: 'a', data: metrics({ cpu_usage: 80 }) }), 0);
    const b = enrichNode(overview({ id: 2, name: 'b', data: metrics({ cpu_usage: 10 }) }), 0);
    const c = enrichNode(overview({ id: 3, name: 'c', status: 'offline', data: null }), 0);
    const sorted = sortNodes([a, c, b], 'cpu');
    expect(sorted.map((node) => node.id)).toEqual([2, 1, 3]);
  });

  it('matches search across name, url and host', () => {
    const node = enrichNode(overview(), 0);
    expect(matchesFilter(node, 'node-a')).toBe(true);
    expect(matchesFilter(node, '8005')).toBe(true);
    expect(matchesFilter(node, 'nope')).toBe(false);
    expect(matchesFilter(node, '')).toBe(true);
  });
});

describe('normalizeAgentUrl', () => {
  it('normalizes hosts for duplicate detection', () => {
    expect(normalizeAgentUrl('HTTP://GPU-01.Local:8005')).toBe('http://gpu-01.local:8005');
    expect(normalizeAgentUrl('http://gpu-01.local:8005/')).toBe('http://gpu-01.local:8005');
    expect(normalizeAgentUrl('bogus')).toBeNull();
  });
});

describe('GPU tiers and formatting', () => {
  it('maps load to tiers', () => {
    expect(getGpuLoadTier(gpu({ utilization: 5, memory_used: 1, memory_total: 24 }))).toBe('idle');
    expect(getGpuLoadTier(gpu({ utilization: 40, memory_used: 8, memory_total: 24 }))).toBe('light');
    expect(getGpuLoadTier(gpu({ utilization: 70, memory_used: 18, memory_total: 24 }))).toBe('busy');
    expect(getGpuLoadTier(gpu({ utilization: 95, memory_used: 23, memory_total: 24 }))).toBe('full');
  });

  it('formats network speeds like the web dashboard', () => {
    expect(formatSpeed(0)).toBe('0 KB/s');
    expect(formatSpeed(0.5)).toBe('512 KB/s');
    expect(formatSpeed(12.345)).toBe('12.35 MB/s');
  });

  it('formats data age compactly', () => {
    expect(formatAgeShort(0.2)).toBe('<1s');
    expect(formatAgeShort(42)).toBe('42s');
    expect(formatAgeShort(125)).toBe('2m 5s');
    expect(formatAgeShort(null)).toBe('N/A');
  });
});

describe('hostFromUrl', () => {
  it('extracts host:port and tolerates junk', () => {
    expect(hostFromUrl('http://1.2.3.4:8005')).toBe('1.2.3.4:8005');
    expect(hostFromUrl('junk')).toBe('junk');
  });
});
