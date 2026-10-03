import type { NodeOverview, TestSSHResult } from '../types/ipc';
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
  'ssh_host_key_failed',
  'ssh_command_timeout',
  'ssh_command_failed',
  'ssh_output_too_large',
  'ssh_snapshot_invalid',
  'ssh_not_configured',
  'encrypted_private_key_not_supported',
  'ssh_metrics_failed',
]);

export function registerNodesIpc(deps: IpcDeps): void {
  handleEnvelope('nodes:overview', () => deps.gateway.request<NodeOverview[]>('GET', '/api/nodes/overview'));

  handleEnvelope('nodes:list', () =>
    deps.gateway
      .request<unknown[]>('GET', '/api/config')
      .then((list) => sanitizeAgentConfigList(list)),
  );

  handleEnvelope('nodes:add', async (payload) => {
    const body = buildAddNodeBody(payload);
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
