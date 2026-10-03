// Pure node payload validation/sanitization, free of electron imports so
// vitest can exercise it. Wired into the nodes IPC registrar.
import type { AddNodePayload, AgentConfig } from '../types/ipc';
import { gatewayError } from '../services/errors';
import {
  isValidNodeName,
  isValidNodeUrl,
  isValidSSHHost,
  isValidSSHPort,
  isValidSSHUser,
} from './validate';

export type ResolvedCollectorType = 'ssh' | 'agent';

function invalid(message: string): Error {
  return gatewayError({ code: 'invalid_input', message });
}

function asRecord(value: unknown, label: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw invalid(`${label} must be an object`);
  }
  return value as Record<string, unknown>;
}

function optionalString(obj: Record<string, unknown>, key: string): string | undefined {
  const raw = obj[key];
  if (raw === undefined || raw === null) return undefined;
  if (typeof raw !== 'string') throw invalid(`${key} must be a string`);
  return raw;
}

/** Resolve collector_type with the Gateway default: absent/agent -> "agent". */
export function resolveCollectorType(raw: unknown): ResolvedCollectorType {
  if (raw === undefined || raw === null || raw === '') return 'agent';
  if (raw === 'ssh') return 'ssh';
  if (raw === 'agent') return 'agent';
  throw invalid('collector_type must be "ssh" or "agent"');
}

/**
 * Sanitize a gateway AgentConfig payload. SSH nodes omit `url`, agent nodes
 * omit the ssh_* fields; anything unexpected is rejected.
 */
export function sanitizeAgentConfig(value: unknown): AgentConfig {
  const obj = asRecord(value, 'agent config');
  const id = obj.id;
  if (typeof id !== 'number' || !Number.isInteger(id) || id <= 0) {
    throw invalid('Node payload has an invalid id');
  }
  if (typeof obj.name !== 'string' || obj.name.length === 0) {
    throw invalid('Node payload has an invalid name');
  }

  const collectorType = resolveCollectorType(obj.collector_type);
  const config: AgentConfig = { id, name: obj.name, collector_type: collectorType };

  if (collectorType === 'agent') {
    const url = optionalString(obj, 'url');
    if (url === undefined) {
      throw invalid('Agent node payload is missing url');
    }
    config.url = url;
    return config;
  }

  const host = optionalString(obj, 'ssh_host');
  const port = obj.ssh_port;
  const user = optionalString(obj, 'ssh_user');
  const authType = optionalString(obj, 'ssh_auth_type');
  if (host === undefined || typeof port !== 'number' || user === undefined) {
    throw invalid('SSH node payload is missing ssh fields');
  }
  if (authType !== undefined && authType !== 'key') {
    throw invalid('Node payload has an invalid ssh_auth_type');
  }
  config.ssh_host = host;
  config.ssh_port = port;
  config.ssh_user = user;
  if (authType !== undefined) config.ssh_auth_type = 'key';
  return config;
}

export function sanitizeAgentConfigList(value: unknown): AgentConfig[] {
  if (!Array.isArray(value)) {
    throw invalid('Expected a node config list');
  }
  return value.map(sanitizeAgentConfig);
}

/**
 * Validate renderer-supplied Add Node input and build the Gateway payload.
 * Mirrors the Gateway rules: agent mode needs a URL, ssh mode needs
 * host/port/user; credentials are never part of the payload.
 */
export function buildAddNodeBody(payload: unknown): AddNodePayload {
  const obj = asRecord(payload, 'add node payload');

  const name = optionalString(obj, 'name')?.trim() ?? '';
  if (!isValidNodeName(name)) {
    throw invalid('Node name is required');
  }

  const collectorType = resolveCollectorType(obj.collector_type);

  if (collectorType === 'agent') {
    const url = optionalString(obj, 'url')?.trim() ?? '';
    if (!isValidNodeUrl(url)) {
      throw invalid('Node URL must be a valid http(s) URL');
    }
    return { name, collector_type: 'agent', url };
  }

  const host = optionalString(obj, 'ssh_host')?.trim() ?? '';
  const port = obj.ssh_port;
  const user = optionalString(obj, 'ssh_user')?.trim() ?? '';
  if (!isValidSSHHost(host)) {
    throw invalid('SSH host is required');
  }
  if (!isValidSSHPort(port)) {
    throw invalid('SSH port must be between 1 and 65535');
  }
  if (!isValidSSHUser(user)) {
    throw invalid('SSH user must be 1-64 characters (letters, digits, . _ -)');
  }
  return { name, collector_type: 'ssh', ssh_host: host, ssh_port: port, ssh_user: user, ssh_auth_type: 'key' };
}

/**
 * Validate renderer-supplied SSH test input (no name needed).
 */
export function buildTestSSHBody(payload: unknown): { ssh_host: string; ssh_port: number; ssh_user: string } {
  const obj = asRecord(payload, 'test ssh payload');

  const host = optionalString(obj, 'ssh_host')?.trim() ?? '';
  const port = obj.ssh_port;
  const user = optionalString(obj, 'ssh_user')?.trim() ?? '';
  if (!isValidSSHHost(host)) {
    throw invalid('SSH host is required');
  }
  if (!isValidSSHPort(port)) {
    throw invalid('SSH port must be between 1 and 65535');
  }
  if (!isValidSSHUser(user)) {
    throw invalid('SSH user must be 1-64 characters (letters, digits, . _ -)');
  }
  return { ssh_host: host, ssh_port: port, ssh_user: user };
}
