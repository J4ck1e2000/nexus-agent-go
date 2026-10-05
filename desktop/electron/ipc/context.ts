import { ipcMain } from 'electron';

import { toErrorPayload } from '../services/errors';
import { gatewayError } from '../services/errors';
import type { GatewayClient } from '../services/gateway';
import type { SettingsStore } from '../services/settings-store';
import type { TokenStore } from '../services/token-store';
import type { LocalSshTerminalManager } from '../services/local-ssh-terminal';
import type { NexusResult, UserRecord, UserRole } from '../types/ipc';

/** Shared dependencies handed to every IPC registrar. */
export interface IpcDeps {
  gateway: GatewayClient;
  tokens: TokenStore;
  settings: SettingsStore;
  notify: (channel: string, payload: unknown) => void;
  terminals: LocalSshTerminalManager;
}

type Handler<T> = (payload: unknown) => Promise<T> | T;

/** Wrap a handler so every IPC call resolves to a NexusResult envelope. */
export function handleEnvelope<T>(channel: string, handler: Handler<T>): void {
  ipcMain.handle(channel, async (_event, payload: unknown): Promise<NexusResult<T>> => {
    try {
      return { ok: true, data: await handler(payload) };
    } catch (err) {
      return { ok: false, error: toErrorPayload(err) };
    }
  });
}

// ---------------------------------------------------------------------------
// Runtime payload validation (renderer input is untrusted)
// ---------------------------------------------------------------------------

export function asObject(value: unknown, label = 'payload'): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw gatewayError({ code: 'invalid_input', message: `${label} must be an object` });
  }
  return value as Record<string, unknown>;
}

export function asStringField(
  obj: Record<string, unknown>,
  key: string,
  options: { max: number; optional?: boolean },
): string {
  const raw = obj[key];
  if (raw === undefined || raw === null) {
    if (options.optional) return '';
    throw gatewayError({ code: 'invalid_input', message: `${key} is required` });
  }
  if (typeof raw !== 'string') {
    throw gatewayError({ code: 'invalid_input', message: `${key} must be a string` });
  }
  if (raw.length > options.max) {
    throw gatewayError({ code: 'invalid_input', message: `${key} is too long` });
  }
  return raw;
}

export function asIdField(obj: Record<string, unknown>, key: string): number {
  const raw = obj[key];
  if (typeof raw !== 'number' || !Number.isInteger(raw) || raw <= 0) {
    throw gatewayError({ code: 'invalid_input', message: `${key} must be a positive integer` });
  }
  return raw;
}

// ---------------------------------------------------------------------------
// Response sanitization (gateway output is also untrusted)
// ---------------------------------------------------------------------------

export function sanitizeUser(value: unknown): UserRecord {
  const obj = asObject(value, 'user');
  const id = obj.id;
  if (typeof id !== 'number' || !Number.isInteger(id) || id <= 0) {
    throw gatewayError({ code: 'invalid_response', message: 'User payload has an invalid id' });
  }
  const username = obj.username;
  if (typeof username !== 'string' || username.length === 0 || username.length > 128) {
    throw gatewayError({ code: 'invalid_response', message: 'User payload has an invalid username' });
  }
  const role: UserRole = obj.role === 'admin' ? 'admin' : 'user';
  const record: UserRecord = { id, username, role };
  if (typeof obj.created_at === 'string') record.created_at = obj.created_at;
  if (typeof obj.updated_at === 'string') record.updated_at = obj.updated_at;
  return record;
}

export function sanitizeUserList(value: unknown): UserRecord[] {
  if (!Array.isArray(value)) {
    throw gatewayError({ code: 'invalid_response', message: 'Expected a user list' });
  }
  return value.map(sanitizeUser);
}
