import { gatewayError } from '../services/errors';
import type { NodeOverview, TerminalSessionInfo, TerminalStartPayload } from '../types/ipc';
import { asIdField, asObject, asStringField, handleEnvelope, type IpcDeps } from './context';

export function registerTerminalIpc(deps: IpcDeps): void {
  handleEnvelope('terminal:start', async (payload) => {
    const input = validateStartPayload(payload);
    const nodes = await deps.gateway.request<NodeOverview[]>('GET', '/api/nodes/overview');
    if (!Array.isArray(nodes)) throw gatewayError({ code: 'invalid_response', message: 'Gateway returned an invalid node list' });
    const node = nodes.find((item) => item.id === input.nodeId);
    if (!node || !node.sshHost || !Number.isInteger(node.sshPort) || (node.sshPort ?? 0) < 1) {
      throw gatewayError({ code: 'not_found', message: 'This node has no direct SSH endpoint' });
    }
    const targetHost = input.targetHost || node.sshHost;
    const started = await deps.terminals.start(node, { ...input, targetHost });
    const session: TerminalSessionInfo = { sessionId: started.sessionId, targetHost, sshUser: started.sshUser };
    return session;
  });

  handleEnvelope('terminal:attach', (payload) => {
    const obj = asObject(payload, 'terminal attach');
    deps.terminals.attach(asSessionId(obj.sessionId));
    return null;
  });

  handleEnvelope('terminal:write', (payload) => {
    const obj = asObject(payload, 'terminal input');
    const sessionId = asSessionId(obj.sessionId);
    const data = asStringField(obj, 'data', { max: 16 * 1024 });
    deps.terminals.write(sessionId, data);
    return null;
  });

  handleEnvelope('terminal:resize', (payload) => {
    const obj = asObject(payload, 'terminal resize');
    const sessionId = asSessionId(obj.sessionId);
    const cols = asDimension(obj.cols, 'cols', 40, 240);
    const rows = asDimension(obj.rows, 'rows', 10, 100);
    deps.terminals.resize(sessionId, cols, rows);
    return null;
  });

  handleEnvelope('terminal:close', (payload) => {
    const obj = asObject(payload, 'terminal close');
    deps.terminals.close(asSessionId(obj.sessionId));
    return null;
  });
}

function validateStartPayload(payload: unknown): TerminalStartPayload {
  const obj = asObject(payload, 'terminal start');
  const nodeId = asIdField(obj, 'nodeId');
  const targetHost = asStringField(obj, 'targetHost', { max: 255, optional: true }).trim();
  const sshUser = asStringField(obj, 'sshUser', { max: 64, optional: true }).trim();
  if (typeof obj.useSshConfig !== 'boolean') {
    throw gatewayError({ code: 'invalid_input', message: 'useSshConfig must be a boolean' });
  }
  const cols = asDimension(obj.cols, 'cols', 40, 240);
  const rows = asDimension(obj.rows, 'rows', 10, 100);
  return { nodeId, targetHost, sshUser, useSshConfig: obj.useSshConfig, cols, rows };
}

function asSessionId(value: unknown): string {
  if (typeof value !== 'string' || !/^[0-9a-f-]{36}$/i.test(value)) {
    throw gatewayError({ code: 'invalid_input', message: 'sessionId is invalid' });
  }
  return value;
}

function asDimension(value: unknown, name: string, minimum: number, maximum: number): number {
  if (typeof value !== 'number' || !Number.isInteger(value) || value < minimum || value > maximum) {
    throw gatewayError({ code: 'invalid_input', message: `${name} is invalid` });
  }
  return value;
}
