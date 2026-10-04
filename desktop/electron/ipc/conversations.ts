import { asIdField, handleEnvelope, type IpcDeps } from './context';
import { gatewayError } from '../services/errors';
import {
  sanitizeConversationList,
  sanitizeConversationMessageList,
  sanitizeConversationSummary,
} from '../lib/conversation-payload';

/**
 * Registers conversation-history IPC (list/create/load/delete). The JWT stays
 * in the main process; the renderer only sees sanitized payloads.
 */
export function registerConversationsIpc(deps: IpcDeps): void {
  handleEnvelope('conversations:list', () =>
    deps.gateway
      .request<unknown>('GET', '/api/ai/conversations?limit=200&offset=0')
      .then((payload) => sanitizeConversationList(payload)),
  );

  handleEnvelope('conversations:create', () =>
    deps.gateway
      .request<unknown>('POST', '/api/ai/conversations', { body: {} })
      .then((payload) => sanitizeConversationSummary(payload)),
  );

  handleEnvelope('conversations:messages', (payload) => {
    const id = asIdField(asPayloadObject(payload), 'id');
    return deps.gateway
      .request<unknown>('GET', `/api/ai/conversations/${id}/messages`)
      .then((data) => sanitizeConversationMessageList(data));
  });

  handleEnvelope('conversations:remove', (payload) => {
    const id = asIdField(asPayloadObject(payload), 'id');
    return deps.gateway
      .request<unknown>('DELETE', `/api/ai/conversations/${id}`)
      .then(() => null);
  });
}

function asPayloadObject(payload: unknown): Record<string, unknown> {
  if (payload === null || typeof payload !== 'object' || Array.isArray(payload)) {
    throw gatewayError({ code: 'invalid_input', message: 'payload must be an object' });
  }
  return payload as Record<string, unknown>;
}
