const SSH_ENV_ALLOWLIST = new Set([
  'PATH', 'HOME', 'USER', 'USERNAME', 'USERDOMAIN', 'USERDNSDOMAIN',
  'USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'APPDATA', 'LOCALAPPDATA',
  'PROGRAMDATA', 'ALLUSERSPROFILE', 'SYSTEMDRIVE', 'SYSTEMROOT', 'WINDIR',
  'SSH_AUTH_SOCK', 'SSH_AGENT_PID', 'COMSPEC', 'PATHEXT',
  'TEMP', 'TMP', 'TMPDIR', 'LANG', 'LC_ALL',
]);

/** Preserve OpenSSH's OS paths/identity without forwarding application secrets. */
export function openSSHEnvironment(source: NodeJS.ProcessEnv = process.env): NodeJS.ProcessEnv {
  const environment: NodeJS.ProcessEnv = {};
  for (const [name, value] of Object.entries(source)) {
    if (value === undefined) continue;
    const normalized = name.toUpperCase();
    if (SSH_ENV_ALLOWLIST.has(normalized) || normalized.startsWith('LC_')) environment[name] = value;
  }
  // The remote PTY must describe xterm, even if the GUI inherited TERM=dumb.
  environment.TERM = 'xterm-256color';
  return environment;
}
