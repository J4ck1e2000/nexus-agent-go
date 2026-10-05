import { useCallback, useEffect, useState } from 'react';
import type { EnrichedNode } from '../lib/node-logic';
import type { NodeHistoryResponse, NexusError } from '../../electron/types/ipc';

export function useNodeHistory(nodeId: number | null, days: number, enabled: boolean) {
  const [history, setHistory] = useState<NodeHistoryResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<NexusError | null>(null);
  const [refreshToken, setRefreshToken] = useState(0);
  const refresh = useCallback(() => setRefreshToken((value) => value + 1), []);

  useEffect(() => {
    setHistory(null);
    setError(null);
    setLoading(false);
    if (!enabled || nodeId === null) return;
    let disposed = false;
    let inFlight = false;
    const load = async () => {
      if (disposed || inFlight) return;
      inFlight = true;
      setLoading(true);
      try {
        const fromUnix = Math.floor(Date.now() / 1000) - Math.max(1, Math.min(90, days)) * 86_400;
        const result = await window.nexus.nodes.history(nodeId, fromUnix, 3600, 'average');
        if (disposed) return;
        if (result.ok && result.data.nodeId === nodeId) {
          setHistory(result.data);
          setError(null);
        } else if (!result.ok) setError(result.error);
      } catch {
        if (!disposed) setError({ code: 'network', message: 'History request was interrupted' });
      } finally {
        inFlight = false;
        if (!disposed) setLoading(false);
      }
    };
    void load();
    const timer = setInterval(() => void load(), 5 * 60_000);
    return () => { disposed = true; clearInterval(timer); };
  }, [days, enabled, nodeId, refreshToken]);

  return { history, loading, error, refresh };
}

export function useFleetIdleHistory(nodes: EnrichedNode[], enabled: boolean) {
  const [historyByNode, setHistoryByNode] = useState<Record<number, NodeHistoryResponse['samples']>>({});
  const [loading, setLoading] = useState(false);
  const [refreshToken, setRefreshToken] = useState(0);
  const nodeKey = nodes.filter((node) => node.online).map((node) => node.id).sort((a, b) => a - b).join(',');
  const refresh = useCallback(() => setRefreshToken((value) => value + 1), []);

  useEffect(() => {
    setLoading(false);
    if (!enabled) return;
    const nodeIds = nodeKey ? nodeKey.split(',').map(Number) : [];
    if (nodeIds.length === 0) { setHistoryByNode({}); return; }
    let disposed = false;
    let inFlight = false;
    const load = async () => {
      if (disposed || inFlight) return;
      inFlight = true;
      setLoading(true);
      const next: Record<number, NodeHistoryResponse['samples']> = {};
      const fromUnix = Math.floor(Date.now() / 1000) - 70 * 60;
      let index = 0;
      const worker = async () => {
        while (!disposed && index < nodeIds.length) {
          const id = nodeIds[index++];
          try {
            const result = await window.nexus.nodes.history(id, fromUnix, 15, 'peak');
            if (result.ok && result.data.nodeId === id) next[id] = result.data.samples;
          } catch { /* Missing history must fail the sustained-idle condition. */ }
        }
      };
      try {
        await Promise.all(Array.from({ length: Math.min(8, nodeIds.length) }, worker));
        if (!disposed) setHistoryByNode(next);
      } finally {
        inFlight = false;
        if (!disposed) setLoading(false);
      }
    };
    void load();
    const timer = setInterval(() => void load(), 30_000);
    return () => { disposed = true; clearInterval(timer); };
  }, [enabled, nodeKey, refreshToken]);

  return { historyByNode, loading, refresh };
}
