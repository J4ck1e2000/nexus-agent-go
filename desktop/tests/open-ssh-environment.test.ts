import { describe, expect, it } from 'vitest';
import { openSSHEnvironment } from '../electron/services/open-ssh-environment';

describe('OpenSSH process environment', () => {
  it('preserves Windows system config paths, user identity and SSH Agent', () => {
    const result = openSSHEnvironment({
      ProgramData: 'C:\\ProgramData', SystemRoot: 'C:\\Windows',
      USERPROFILE: 'C:\\Users\\alice', USERNAME: 'alice', USERDOMAIN: 'WORKSTATION',
      SSH_AUTH_SOCK: 'local-agent', PATH: 'client-path', LC_MESSAGES: 'en_US.UTF-8',
    });
    expect(result).toMatchObject({
      ProgramData: 'C:\\ProgramData', SystemRoot: 'C:\\Windows',
      USERPROFILE: 'C:\\Users\\alice', USERNAME: 'alice', USERDOMAIN: 'WORKSTATION',
      SSH_AUTH_SOCK: 'local-agent', PATH: 'client-path', LC_MESSAGES: 'en_US.UTF-8',
    });
  });

  it('does not inherit app credentials or Electron process settings', () => {
    const result = openSSHEnvironment({
      AI_API_KEY: 'test-secret', JWT_SECRET: 'test-secret', MYSQL_ROOT_PASSWORD: 'test-secret',
      PI_RUNTIME_TOKEN: 'test-secret', NODE_OPTIONS: '--inspect', ELECTRON_RUN_AS_NODE: '1',
    });
    expect(result).toEqual({ TERM: 'xterm-256color' });
  });

  it('advertises xterm instead of the GUI terminal type', () => {
    expect(openSSHEnvironment({ TERM: 'dumb' }).TERM).toBe('xterm-256color');
    expect(openSSHEnvironment({}).TERM).toBe('xterm-256color');
  });
});
