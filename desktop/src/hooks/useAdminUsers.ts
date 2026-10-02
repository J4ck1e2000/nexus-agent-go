import { useCallback, useEffect, useState } from 'react';
import type { NexusError, NexusResult, UserRecord, UserRole } from '../../electron/types/ipc';

export interface UseAdminUsersResult {
  users: UserRecord[];
  loading: boolean;
  error: NexusError | null;
  reload: () => Promise<void>;
  createUser: (payload: { username: string; password: string; role: UserRole }) => Promise<NexusResult<UserRecord>>;
  deleteUser: (id: number) => Promise<NexusResult<null>>;
  setRole: (id: number, role: UserRole) => Promise<NexusResult<UserRecord>>;
  resetPassword: (id: number, password: string) => Promise<NexusResult<null>>;
}

/** Admin user management data (GET/POST/DELETE/PATCH /api/admin/users*). */
export function useAdminUsers(enabled: boolean): UseAdminUsersResult {
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState<NexusError | null>(null);

  const reload = useCallback(async (): Promise<void> => {
    const result = await window.nexus.users.list();
    if (result.ok) {
      setUsers(result.data);
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

  const createUser = useCallback(
    async (payload: { username: string; password: string; role: UserRole }) => {
      const result = await window.nexus.users.create(payload);
      if (result.ok) {
        await reload();
      }
      return result;
    },
    [reload],
  );

  const deleteUser = useCallback(
    async (id: number) => {
      const result = await window.nexus.users.remove(id);
      if (result.ok) {
        await reload();
      }
      return result;
    },
    [reload],
  );

  const setRole = useCallback(
    async (id: number, role: UserRole) => {
      const result = await window.nexus.users.setRole(id, role);
      if (result.ok) {
        setUsers((current) =>
          current.map((user) => (user.id === id ? { ...user, role: result.data.role } : user)),
        );
      }
      return result;
    },
    [],
  );

  const resetPassword = useCallback(
    async (id: number, password: string) => window.nexus.users.resetPassword(id, password),
    [],
  );

  return { users, loading, error, reload, createUser, deleteUser, setRole, resetPassword };
}
