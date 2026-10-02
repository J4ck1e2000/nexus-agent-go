import { useCallback, useEffect, useState } from 'react';
import type { AgentConfig, NexusError, NexusResult } from '../../electron/types/ipc';

export interface UseNodeConfigsResult {
  configs: AgentConfig[];
  loading: boolean;
  error: NexusError | null;
  reload: () => Promise<void>;
  addNode: (payload: { name: string; url: string }) => Promise<NexusResult<AgentConfig>>;
  removeNode: (id: number) => Promise<NexusResult<null>>;
}

/** Node configuration list (GET/POST/DELETE /api/config) for admins. */
export function useNodeConfigs(enabled: boolean): UseNodeConfigsResult {
  const [configs, setConfigs] = useState<AgentConfig[]>([]);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState<NexusError | null>(null);

  const reload = useCallback(async (): Promise<void> => {
    const result = await window.nexus.nodes.list();
    if (result.ok) {
      setConfigs(result.data);
      setError(null);
    } else {
      setError(result.error);
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    if (!enabled) return;
    void reload();
  }, [enabled, reload]);

  const addNode = useCallback(
    async (payload: { name: string; url: string }): Promise<NexusResult<AgentConfig>> => {
      const result = await window.nexus.nodes.add(payload);
      if (result.ok) {
        await reload();
      }
      return result;
    },
    [reload],
  );

  const removeNode = useCallback(
    async (id: number): Promise<NexusResult<null>> => {
      const result = await window.nexus.nodes.remove(id);
      if (result.ok) {
        await reload();
      }
      return result;
    },
    [reload],
  );

  return { configs, loading, error, reload, addNode, removeNode };
}
