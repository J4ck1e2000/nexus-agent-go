import { useCallback, useEffect, useState } from 'react';
import type { AgentConfig, NexusError, NexusResult, TestSSHResult } from '../../electron/types/ipc';
import type { ResolvedCollectorType } from '../../electron/lib/node-payload';

export interface AddNodeInput {
  name: string;
  collectorType: ResolvedCollectorType;
  /** Agent mode: agent base URL. */
  url?: string;
  /** SSH mode: connection target (credentials live on the Gateway only). */
  sshHost?: string;
  sshPort?: number;
  sshUser?: string;
}

export interface TestSSHInput {
  sshHost: string;
  sshPort: number;
  sshUser: string;
}

export interface UseNodeConfigsResult {
  configs: AgentConfig[];
  loading: boolean;
  error: NexusError | null;
  reload: () => Promise<void>;
  addNode: (payload: AddNodeInput) => Promise<NexusResult<AgentConfig>>;
  removeNode: (id: number) => Promise<NexusResult<null>>;
  testSSH: (payload: TestSSHInput) => Promise<NexusResult<TestSSHResult>>;
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
    async (payload: AddNodeInput): Promise<NexusResult<AgentConfig>> => {
      const result = await window.nexus.nodes.add({
        name: payload.name,
        collector_type: payload.collectorType,
        url: payload.url,
        ssh_host: payload.sshHost,
        ssh_port: payload.sshPort,
        ssh_user: payload.sshUser,
        ssh_auth_type: payload.collectorType === 'ssh' ? 'key' : undefined,
      });
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

  const testSSH = useCallback(
    async (payload: TestSSHInput): Promise<NexusResult<TestSSHResult>> =>
      window.nexus.nodes.testSSH({
        ssh_host: payload.sshHost,
        ssh_port: payload.sshPort,
        ssh_user: payload.sshUser,
      }),
    [],
  );

  return { configs, loading, error, reload, addNode, removeNode, testSSH };
}
