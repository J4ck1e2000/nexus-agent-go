import type { NodeHistoryResponse, NodeOverview, TestSSHResult } from '../types/ipc';
import { execFile } from 'node:child_process';
import { homedir } from 'node:os';
import path from 'node:path';
import { handleEnvelope, type IpcDeps } from './context';
import { GatewayError, gatewayError } from '../services/errors';
import {
  buildAddNodeBody,
  buildTestSSHBody,
  sanitizeAgentConfig,
  sanitizeAgentConfigList,
} from '../lib/node-payload';

/** Request timeout for the SSH connection test (covers connect + one collect). */
const TEST_SSH_TIMEOUT_MS = 20_000;

const SSH_ERROR_DETAILS = new Set([
  'ssh_connect_failed',
  'ssh_auth_failed',
  'ssh_password_auth_failed',
  'ssh_bootstrap_failed',
  'ssh_enrollment_required',
  'insecure_transport',
  'ssh_host_key_failed',
  'ssh_command_timeout',
  'ssh_command_failed',
  'ssh_output_too_large',
  'ssh_snapshot_invalid',
  'ssh_not_configured',
  'encrypted_private_key_not_supported',
  'ssh_metrics_failed',
]);

type SSHHostKeyHint = { marker: string; public_key: string };

function isLoopbackHost(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, '');
  return host === 'localhost' || host.endsWith('.localhost') || host === '::1' || /^127(?:\.\d{1,3}){3}$/.test(host);
}

function canSendBootstrapSecretOver(url: string): boolean {
  try {
    const parsed = new URL(url);
    if (parsed.protocol === 'https:') return true;
    return parsed.protocol === 'http:' && isLoopbackHost(parsed.hostname);
  } catch {
    return false;
  }
}

function readSSHKeygenOutput(file: string, host: string, port: number): Promise<string> {
  const target = port === 22 ? host : '[' + host + ']:' + String(port);
  return new Promise((resolve) => {
    execFile('ssh-keygen', ['-F', target, '-f', file], { timeout: 2000, maxBuffer: 256 * 1024, windowsHide: true }, (error, stdout) => {
      resolve(error ? '' : String(stdout));
    });
  });
}

async function readLocalTrustedHostKeys(host: string, port: number): Promise<SSHHostKeyHint[]> {
  const file = path.join(homedir(), '.ssh', 'known_hosts');
  const output = await readSSHKeygenOutput(file, host, port);
  const hints: SSHHostKeyHint[] = [];
  for (const line of output.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const fields = trimmed.split(/\s+/);
    let marker = '';
    if (fields[0].startsWith('@')) marker = fields.shift() ?? '';
    if (fields.length < 3) continue;
    fields.shift(); // host pattern (ssh-keygen has already matched this target)
    const keyType = fields.shift();
    const keyData = fields.shift();
    if (!keyType || !keyData) continue;
    hints.push({ marker, public_key: keyType + ' ' + keyData });
    if (hints.length >= 16) break;
  }
  return hints;
}
export function registerNodesIpc(deps: IpcDeps): void {
  handleEnvelope('nodes:overview', () => deps.gateway.request<NodeOverview[]>('GET', '/api/nodes/overview'));

  handleEnvelope('nodes:history', (payload) => {
    const obj = asIdObject(payload);
    const rawFrom = (payload as { fromUnix?: unknown }).fromUnix;
    if (typeof rawFrom !== 'number' || !Number.isSafeInteger(rawFrom) || rawFrom < 0) {
      throw gatewayError({ code: 'invalid_input', message: 'fromUnix must be a non-negative integer' });
    }
    const stepSeconds = (payload as { stepSeconds?: unknown }).stepSeconds;
    if (typeof stepSeconds !== 'number' || !Number.isInteger(stepSeconds) || stepSeconds < 5 || stepSeconds > 86_400) {
      throw gatewayError({ code: 'invalid_input', message: 'stepSeconds is invalid' });
    }
    const aggregation = (payload as { aggregation?: unknown }).aggregation;
    if (aggregation !== 'average' && aggregation !== 'peak') {
      throw gatewayError({ code: 'invalid_input', message: 'aggregation is invalid' });
    }
    return deps.gateway.request<NodeHistoryResponse>('GET', `/api/nodes/${obj.id}/history?from=${rawFrom}&step_seconds=${stepSeconds}&aggregation=${aggregation}`);
  });

  handleEnvelope('nodes:list', () =>
    deps.gateway
      .request<unknown[]>('GET', '/api/config')
      .then((list) => sanitizeAgentConfigList(list)),
  );

  handleEnvelope('nodes:add', async (payload) => {
    const body = buildAddNodeBody(payload);
    if (body.collector_type === 'ssh') {
      if (!body.ssh_host || !body.ssh_port || !body.ssh_user) {
        throw gatewayError({ code: 'invalid_input', message: 'SSH enrollment fields are incomplete' });
      }
      const gatewayUrl = deps.settings.load().gatewayUrl;
      if (body.ssh_password && !canSendBootstrapSecretOver(gatewayUrl)) {
        throw gatewayError({
          code: 'insecure_transport',
          message: 'SSH bootstrap credentials require HTTPS for a remote Gateway.',
        });
      }
      const trusted_host_keys = await readLocalTrustedHostKeys(body.ssh_host, body.ssh_port);
      const enrollmentBody = {
        name: body.name,
        ssh_host: body.ssh_host,
        ssh_port: body.ssh_port,
        ssh_user: body.ssh_user,
        ssh_password: body.ssh_password,
        trusted_host_keys,
      };
      const enrolled = await deps.gateway.request<unknown>('POST', '/api/config/enroll-ssh', { body: enrollmentBody, timeoutMs: 60_000 });
      return sanitizeAgentConfig(enrolled);
    }
    const created = await deps.gateway.request<unknown>('POST', '/api/config', { body });
    return sanitizeAgentConfig(created);
  });

  handleEnvelope('nodes:test-ssh', async (payload) => {
    const body = buildTestSSHBody(payload);
    try {
      return await deps.gateway.request<TestSSHResult>('POST', '/api/config/test-ssh', {
        body,
        timeoutMs: TEST_SSH_TIMEOUT_MS,
      });
    } catch (err) {
      // Surface the gateway's stable ssh_* error code as the error code so the
      // renderer can localize it; other failures keep their normalized code.
      if (err instanceof GatewayError && err.detail && SSH_ERROR_DETAILS.has(err.detail)) {
        throw gatewayError({
          code: 'gateway_error',
          status: err.status,
          detail: err.detail,
          message: `SSH connection test failed: ${err.detail}`,
        });
      }
      throw err;
    }
  });

  handleEnvelope('nodes:remove', (payload) => {
    const obj = asIdObject(payload);
    return deps.gateway
      .request<unknown>('DELETE', `/api/config/${obj.id}`)
      .then(() => null);
  });
}

function asIdObject(payload: unknown): { id: number } {
  const id = (payload as { id?: unknown } | null | undefined)?.id;
  if (typeof id !== 'number' || !Number.isInteger(id) || id <= 0) {
    throw gatewayError({ code: 'invalid_input', message: 'id must be a positive integer' });
  }
  return { id };
}
