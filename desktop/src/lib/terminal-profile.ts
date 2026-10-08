export interface LocalTerminalProfile {
  targetHost: string;
  sshUser: string;
  useSshConfig: boolean;
}

export function loadLocalTerminalProfile(userId: number, nodeId: number): LocalTerminalProfile | null {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(terminalProfileKey(userId, nodeId)) ?? 'null');
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
    const profile = raw as Record<string, unknown>;
    return {
      targetHost: typeof profile.targetHost === 'string' ? profile.targetHost.slice(0, 255) : '',
      sshUser: typeof profile.sshUser === 'string' ? profile.sshUser.slice(0, 64) : '',
      useSshConfig: profile.useSshConfig === true,
    };
  } catch {
    return null;
  }
}

export function saveLocalTerminalProfile(userId: number, nodeId: number, profile: LocalTerminalProfile): void {
  try {
    localStorage.setItem(terminalProfileKey(userId, nodeId), JSON.stringify(profile));
  } catch {
    // These non-secret connection hints are optional convenience settings.
  }
}

function terminalProfileKey(userId: number, nodeId: number): string {
  return `nexus.terminalProfile.v1.${userId}.${nodeId}`;
}
