import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import type { NodeOverview } from '../electron/types/ipc';

const mocks = vi.hoisted(() => ({ spawn: vi.fn(), execFile: vi.fn() }));
vi.mock('node-pty', () => ({ spawn: mocks.spawn }));
vi.mock('node:child_process', () => ({ execFile: mocks.execFile }));
import { LocalSshTerminalManager } from '../electron/services/local-ssh-terminal';

class FakePty {
  write = vi.fn();
  resize = vi.fn();
  kill = vi.fn();
  dataListener: (data: string) => void = () => {};
  exitListener: (event: { exitCode: number; signal?: number }) => void = () => {};
  onData(listener: (data: string) => void) { this.dataListener = listener; return { dispose() {} }; }
  onExit(listener: (event: { exitCode: number; signal?: number }) => void) { this.exitListener = listener; return { dispose() {} }; }
}

const node = { id: 4, sshHost: '10.0.0.4', sshPort: 22 } as NodeOverview;
const options = { targetHost: 'gpu-alias', sshUser: 'personal-user', useSshConfig: false, cols: 100, rows: 30 };
let child: FakePty;
let manager: LocalSshTerminalManager;
let emit: Mock<(channel: string, payload: unknown) => void>;

beforeEach(() => {
  vi.clearAllMocks();
  child = new FakePty();
  mocks.spawn.mockReturnValue(child);
  mocks.execFile.mockImplementation((_file, _args, _options, callback: (error: Error | null, stdout: string, stderr: string) => void) => {
    callback(null, 'hostname 10.0.0.4\nport 22\nuser personal-user\n', '');
  });
  emit = vi.fn<(channel: string, payload: unknown) => void>();
  manager = new LocalSshTerminalManager(emit);
});
afterEach(() => { manager.closeAll(); vi.unstubAllEnvs(); });

describe('local OpenSSH terminal lifecycle', () => {
  it('buffers startup output and an immediate exit until the renderer attaches', async () => {
    const started = await manager.start(node, options);
    child.dataListener('Permission denied.\r\n');
    child.exitListener({ exitCode: 255 });
    expect(emit).not.toHaveBeenCalled();
    manager.attach(started.sessionId);
    expect(emit.mock.calls.map((call) => call[0])).toEqual(['terminal:output', 'terminal:exit']);
    expect(emit.mock.calls[0][1]).toMatchObject({ sessionId: started.sessionId, data: 'Permission denied.\r\n' });
    expect(() => manager.write(started.sessionId, 'input')).toThrow('no longer active');
  });

  it('uses the personal Linux account and supports input, resize and explicit cleanup', async () => {
    const started = await manager.start(node, options);
    expect(mocks.spawn.mock.calls[0][1]).toEqual(expect.arrayContaining(['-l', 'personal-user', '-p', '22', '--', 'gpu-alias']));
    manager.attach(started.sessionId);
    manager.write(started.sessionId, 'echo hello\r');
    manager.resize(started.sessionId, 120, 40);
    expect(child.write).toHaveBeenCalledWith('echo hello\r');
    expect(child.resize).toHaveBeenCalledWith(120, 40);
    manager.close(started.sessionId);
    expect(child.kill).toHaveBeenCalledOnce();
  });

  it('rejects an alias that resolves to another server before starting a PTY', async () => {
    mocks.execFile.mockImplementation((_file, _args, _options, callback: (error: Error | null, stdout: string, stderr: string) => void) => {
      callback(null, 'hostname 10.0.0.99\nport 22\nuser personal-user\n', '');
    });
    await expect(manager.start(node, options)).rejects.toThrow('not this node');
    expect(mocks.spawn).not.toHaveBeenCalled();
  });

  it('lets OpenSSH config choose the username instead of overriding it with the form value', async () => {
    await manager.start(node, { ...options, useSshConfig: true });
    const args = mocks.spawn.mock.calls[0][1] as string[];
    expect(args).not.toContain('-l');
    expect(args).not.toContain('-p');
  });

  it('uses the same Windows-compatible environment for config checks and the SSH PTY', async () => {
    vi.stubEnv('ProgramData', 'C:\\ProgramData');
    vi.stubEnv('TERM', 'dumb');
    vi.stubEnv('AI_API_KEY', 'test-secret');
    await manager.start(node, options);
    const configEnvironment = mocks.execFile.mock.calls[0][2].env;
    const terminalEnvironment = mocks.spawn.mock.calls[0][2].env;
    expect(configEnvironment).toEqual(terminalEnvironment);
    expect(Object.entries(configEnvironment).find(([name]) => name.toUpperCase() === 'PROGRAMDATA')?.[1]).toBe('C:\\ProgramData');
    expect(configEnvironment.TERM).toBe('xterm-256color');
    expect(configEnvironment.AI_API_KEY).toBeUndefined();
  });

  it('keeps an actionable OpenSSH diagnostic when config parsing fails', async () => {
    mocks.execFile.mockImplementation((_file, _args, _options, callback) => {
      callback(Object.assign(new Error('Command failed'), { code: 255 }), '', 'Bad configuration option: bogus\r\n');
    });
    await expect(manager.start(node, options)).rejects.toMatchObject({
      detail: 'terminal_ssh_config_failed', message: 'Bad configuration option: bogus',
    });
    expect(mocks.spawn).not.toHaveBeenCalled();
  });

  it('distinguishes a missing SSH executable from a silent nonzero config exit', async () => {
    mocks.execFile.mockImplementation((_file, _args, _options, callback) => {
      callback(Object.assign(new Error('Command failed'), { code: 'ENOENT' }), '', '');
    });
    await expect(manager.start(node, options)).rejects.toMatchObject({ detail: 'terminal_ssh_missing' });
    mocks.execFile.mockImplementation((_file, _args, _options, callback) => {
      callback(Object.assign(new Error('Command failed'), { code: 255 }), '', '');
    });
    await expect(manager.start(node, options)).rejects.toMatchObject({
      detail: 'terminal_ssh_config_failed', message: 'OpenSSH exited with code 255',
    });
  });
});
