import type { AddNodePayload, AgentConfig, NodeOverview } from '../types/ipc';
import { asIdField, asObject, asStringField, handleEnvelope, type IpcDeps } from './context';
import { isValidNodeName, isValidNodeUrl } from '../lib/validate';
import { gatewayError } from '../services/errors';

function sanitizeAgentConfig(value: unknown): AgentConfig {
  const obj = asObject(value, 'agent config');
  if (typeof obj.id !== 'number' || !Number.isInteger(obj.id)) {
    throw gatewayError({ code: 'invalid_response', message: 'Node payload has an invalid id' });
  }
  if (typeof obj.name !== 'string' || typeof obj.url !== 'string') {
    throw gatewayError({ code: 'invalid_response', message: 'Node payload has invalid fields' });
  }
  return { id: obj.id, name: obj.name, url: obj.url };
}

export function registerNodesIpc(deps: IpcDeps): void {
  handleEnvelope('nodes:overview', () => deps.gateway.request<NodeOverview[]>('GET', '/api/nodes/overview'));

  handleEnvelope('nodes:list', () =>
    deps.gateway.request<unknown[]>('GET', '/api/config').then((list) => {
      if (!Array.isArray(list)) {
        throw gatewayError({ code: 'invalid_response', message: 'Expected a node config list' });
      }
      return list.map(sanitizeAgentConfig);
    }),
  );

  handleEnvelope('nodes:add', async (payload) => {
    const obj = asObject(payload);
    const name = asStringField(obj, 'name', { max: 128 }).trim();
    const url = asStringField(obj, 'url', { max: 2048 }).trim();
    if (!isValidNodeName(name)) {
      throw gatewayError({ code: 'invalid_input', message: 'Node name is required' });
    }
    if (!isValidNodeUrl(url)) {
      throw gatewayError({ code: 'invalid_input', message: 'Node URL must be a valid http(s) URL' });
    }
    const body: AddNodePayload = { name, url };
    const created = await deps.gateway.request<unknown>('POST', '/api/config', { body });
    return sanitizeAgentConfig(created);
  });

  handleEnvelope('nodes:remove', (payload) => {
    const obj = asObject(payload);
    const id = asIdField(obj, 'id');
    return deps.gateway
      .request<unknown>('DELETE', `/api/config/${id}`)
      .then(() => null);
  });
}
