import { useCallback, useEffect, useRef, useState } from 'react';
import type { NexusError } from '../../electron/types/ipc';
import { enrichNode, type EnrichedNode } from '../lib/node-logic';

const POLL_INTERVAL_MS = 2000;

export interface UseNodesResult {
  nodes: EnrichedNode[];
  loading: boolean;
  error: NexusError | null;
  lastSyncedAt: Date | null;
  refresh: () => void;
}

/**
 * Polls GET /api/nodes/overview every 2 seconds (matching the web dashboard)
 * and enriches raw gateway payloads for display. The interval is cleared on
 * unmount and overlapping requests are skipped.
 */
export function useNodes(enabled: boolean): UseNodesResult {
  const [nodes, setNodes] = useState<EnrichedNode[]>([]);
  const [error, setError] = useState<NexusError | null>(null);
  const [lastSyncedAt, setLastSyncedAt] = useState<Date | null>(null);
  const [loading, setLoading] = useState(enabled);

  const inFlightRef = useRef(false);
  const disposedRef = useRef(false);

  const fetchAll = useCallback(async (): Promise<void> => {
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    try {
      const result = await window.nexus.nodes.overview();
      if (disposedRef.current) return;
      if (result.ok) {
        const nowMs = Date.now();
        setNodes(result.data.map((node) => enrichNode(node, nowMs)));
        setError(null);
        // Only a successful sync may advance the "synced" timestamp; on
        // failure the UI keeps showing when data was last actually fresh.
        setLastSyncedAt(new Date());
      } else {
        // Poll errors stay silent in the UI (web behavior); keep stale data.
        setError(result.error);
      }
    } finally {
      inFlightRef.current = false;
      if (!disposedRef.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    disposedRef.current = false;
    setLoading(true);
    void fetchAll();
    const timer = setInterval(() => {
      void fetchAll();
    }, POLL_INTERVAL_MS);
    return () => {
      disposedRef.current = true;
      clearInterval(timer);
    };
  }, [enabled, fetchAll]);

  const refresh = useCallback(() => {
    void fetchAll();
  }, [fetchAll]);

  return { nodes, loading, error, lastSyncedAt, refresh };
}
