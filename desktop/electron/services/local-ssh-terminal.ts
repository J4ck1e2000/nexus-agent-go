import { execFile } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import * as pty from 'node-pty';
import { gatewayError } from './errors';
import { openSSHEnvironment } from './open-ssh-environment';
import type { TerminalExitEvent, TerminalOutputEvent } from '../types/ipc';
import type { NodeOverview } from '../types/ipc';

const MAX_TERMINAL_SESSIONS = 8;
const MAX_TERMINAL_INPUT_BYTES = 16 * 1024;
const MAX_PENDING_OUTPUT_BYTES = 256 * 1024;

export interface LocalTerminalStartOptions {
  targetHost: string;
  sshUser: string;
  useSshConfig: boolean;
  cols: number;
  rows: number;
}

type Session = {
  process: pty.IPty;
  attached: boolean;
  pendingOutput: string[];
  pendingBytes: number;
  exitEvent?: TerminalExitEvent;
  attachTimer: ReturnType<typeof setTimeout>;
};

export interface LocalTerminalSessionStart {
  sessionId: string;
  sshUser: string;
}

/** Runs the user's own OpenSSH client so passwords, keys and ssh-agent stay local. */
export class LocalSshTerminalManager {
  private readonly sessions = new Map<string, Session>();

  constructor(
    private readonly emit: (channel: string, payload: unknown) => void,
  ) {}

  async start(node: NodeOverview, options: LocalTerminalStartOptions): Promise<LocalTerminalSessionStart> {
    if (!node.sshHost || !Number.isInteger(node.sshPort) || (node.sshPort ?? 0) < 1 || (node.sshPort ?? 0) > 65535) {
      throw new Error('This node does not have a direct SSH target');
    }
    if (this.sessions.size >= MAX_TERMINAL_SESSIONS) {
      throw new Error('Too many local SSH terminal sessions are open');
    }

    const targetHost = options.targetHost.trim() || node.sshHost;
    if (!isSafeSSHHostOrAlias(targetHost)) throw new Error('Invalid SSH host or local SSH alias');
    const username = options.useSshConfig ? '' : options.sshUser.trim();
    if (username && !isSafeSSHUsername(username)) throw new Error('Invalid SSH username');
    if (!options.useSshConfig && !username) throw new Error('SSH username is required for direct connection');

    const cols = clampDimension(options.cols, 40, 240, 100);
    const rows = clampDimension(options.rows, 10, 100, 30);
    const connectionOptions: string[] = [];
    if (!options.useSshConfig) connectionOptions.push('-p', String(node.sshPort));
    if (username) connectionOptions.push('-l', username);
    const executable = process.platform === 'win32' ? 'ssh.exe' : 'ssh';
    const effective = await readEffectiveOpenSSHConfig(executable, connectionOptions, targetHost);
    if (normalizeHost(effective.host) !== normalizeHost(node.sshHost) || effective.port !== node.sshPort) {
      throw gatewayError({ code: 'invalid_input', detail: 'terminal_alias_mismatch', message: `Local SSH configuration resolves to ${effective.host}:${effective.port}, not this node's configured endpoint` });
    }
    if (this.sessions.size >= MAX_TERMINAL_SESSIONS) throw new Error('Too many local SSH terminal sessions are open');

    const args = ['-tt', '-o', 'ConnectTimeout=12', '-o', 'ServerAliveInterval=15', '-o', 'ServerAliveCountMax=3', ...connectionOptions, '--', targetHost];

    const sessionId = randomUUID();
    let child: pty.IPty;
    try {
      child = pty.spawn(executable, args, {
        name: 'xterm-256color',
        cols,
        rows,
        cwd: process.env.USERPROFILE || process.env.HOME || process.cwd(),
        env: openSSHEnvironment(),
        useConpty: true,
        // Bundled ConPTY avoids the system backend's console-enumeration race
        // when closing a Windows SSH process. Forge ships this DLL unpacked.
        useConptyDll: process.platform === 'win32',
      });
    } catch (error) {
      throw gatewayError({ code: 'gateway_error', detail: 'terminal_ssh_start_failed', message: safeMessage(error) });
    }

    const session: Session = {
      process: child, attached: false, pendingOutput: [], pendingBytes: 0,
      attachTimer: setTimeout(() => this.close(sessionId), 30_000),
    };
    this.sessions.set(sessionId, session);
    child.onData((data) => {
      if (session.attached) this.emit('terminal:output', { sessionId, data } satisfies TerminalOutputEvent);
      else {
        session.pendingOutput.push(data);
        session.pendingBytes += Buffer.byteLength(data, 'utf8');
        while (session.pendingBytes > MAX_PENDING_OUTPUT_BYTES && session.pendingOutput.length > 1) {
          session.pendingBytes -= Buffer.byteLength(session.pendingOutput.shift()!, 'utf8');
        }
        if (session.pendingBytes > MAX_PENDING_OUTPUT_BYTES) {
          session.pendingOutput = [session.pendingOutput[0].slice(-MAX_PENDING_OUTPUT_BYTES / 4)];
          session.pendingBytes = Buffer.byteLength(session.pendingOutput[0], 'utf8');
        }
      }
    });
    child.onExit(({ exitCode, signal }) => {
      session.exitEvent = { sessionId, exitCode, signal };
      if (session.attached) {
        clearTimeout(session.attachTimer);
        this.sessions.delete(sessionId);
        this.emit('terminal:exit', session.exitEvent);
      }
    });
    return { sessionId, sshUser: effective.user || username };
  }

  attach(sessionId: string): void {
    const session = this.sessions.get(sessionId);
    if (!session || session.attached) return;
    session.attached = true;
    clearTimeout(session.attachTimer);
    for (const data of session.pendingOutput) this.emit('terminal:output', { sessionId, data } satisfies TerminalOutputEvent);
    session.pendingOutput = [];
    session.pendingBytes = 0;
    if (session.exitEvent) {
      this.sessions.delete(sessionId);
      this.emit('terminal:exit', session.exitEvent);
    }
  }

  write(sessionId: string, data: string): void {
    const session = this.requireSession(sessionId);
    if (Buffer.byteLength(data, 'utf8') > MAX_TERMINAL_INPUT_BYTES) throw new Error('Terminal input is too large');
    session.process.write(data);
  }

  resize(sessionId: string, cols: number, rows: number): void {
    this.requireSession(sessionId).process.resize(
      clampDimension(cols, 40, 240, 100),
      clampDimension(rows, 10, 100, 30),
    );
  }

  close(sessionId: string): void {
    const session = this.sessions.get(sessionId);
    if (!session) return;
    this.sessions.delete(sessionId);
    clearTimeout(session.attachTimer);
    if (!session.exitEvent) session.process.kill();
  }

  closeAll(): void {
    for (const [sessionId, session] of this.sessions) {
      this.sessions.delete(sessionId);
      clearTimeout(session.attachTimer);
      if (!session.exitEvent) session.process.kill();
    }
  }

  private requireSession(sessionId: string): Session {
    const session = this.sessions.get(sessionId);
    if (!session || session.exitEvent) throw new Error('Terminal session is no longer active');
    return session;
  }
}

function isSafeSSHHostOrAlias(value: string): boolean {
  return value.length <= 255 && value.length > 0 && !value.startsWith('-') && /^[A-Za-z0-9_.:\-\[\]]+$/.test(value);
}

function isSafeSSHUsername(value: string): boolean {
  return value.length <= 64 && !value.startsWith('-') && /^[A-Za-z0-9._-]+$/.test(value);
}

function clampDimension(value: number, minimum: number, maximum: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback;
  return Math.max(minimum, Math.min(maximum, Math.floor(value)));
}

function readEffectiveOpenSSHConfig(executable: string, connectionOptions: string[], targetHost: string): Promise<{ host: string; port: number; user: string }> {
  return new Promise((resolve, reject) => {
    execFile(executable, ['-G', ...connectionOptions, '--', targetHost], {
      timeout: 5000,
      maxBuffer: 64 * 1024,
      windowsHide: true,
      env: openSSHEnvironment(),
    }, (error, stdout, stderr) => {
      if (error) {
        const missing = (error as NodeJS.ErrnoException).code === 'ENOENT';
        reject(gatewayError({
          code: 'gateway_error',
          detail: missing ? 'terminal_ssh_missing' : 'terminal_ssh_config_failed',
          message: missing ? 'OpenSSH was not found on this computer'
            : (stderr?.trim().slice(0, 500) || `OpenSSH exited with code ${error.code ?? 'unknown'}`),
        }));
        return;
      }
      const host = /^hostname\s+(.+)$/im.exec(stdout)?.[1]?.trim();
      const portText = /^port\s+(\d+)$/im.exec(stdout)?.[1];
      const user = /^user\s+(.+)$/im.exec(stdout)?.[1]?.trim() ?? '';
      const port = portText ? Number(portText) : NaN;
      if (!host || !Number.isInteger(port) || port < 1 || port > 65535) {
        reject(gatewayError({ code: 'invalid_response', detail: 'terminal_ssh_config_failed', message: 'Local OpenSSH did not resolve a valid host and port' }));
        return;
      }
      resolve({ host, port, user });
    });
  });
}

function normalizeHost(host: string): string {
  return host.trim().toLowerCase().replace(/^\[|\]$/g, '').replace(/\.$/, '');
}

function safeMessage(error: unknown): string {
  return error instanceof Error ? error.message.slice(0, 300) : 'unknown error';
}
