import { isValidRequestId, sanitizeAIQuery } from '../lib/validate';
import { gatewayError, toErrorPayload } from '../services/errors';
import type { AIStreamEventType } from '../types/ipc';
import { asObject, handleEnvelope, type IpcDeps } from './context';

const AI_EVENT_CHANNEL = 'ai:event';

/**
 * Registers AI streaming IPC. The SSE request lives entirely in the main
 * process; the renderer only sees typed events tagged with its requestId.
 */
export function registerAIIpc(deps: IpcDeps): () => void {
  const activeRequests = new Map<string, AbortController>();

  handleEnvelope('ai:start', (payload) => {
    const obj = asObject(payload);
    const requestId = obj.requestId;
    if (!isValidRequestId(requestId)) {
      throw gatewayError({ code: 'invalid_input', message: 'requestId must match [A-Za-z0-9_-]{8,64}' });
    }
    const query = sanitizeAIQuery(obj.query);
    if (activeRequests.has(requestId)) {
      throw gatewayError({ code: 'invalid_input', message: 'requestId is already streaming' });
    }

    const controller = new AbortController();
    activeRequests.set(requestId, controller);
    void runAIStream(deps, requestId, query, controller).finally(() => {
      activeRequests.delete(requestId);
    });
    return null;
  });

  handleEnvelope('ai:cancel', (payload) => {
    const obj = asObject(payload);
    const requestId = obj.requestId;
    if (!isValidRequestId(requestId)) {
      throw gatewayError({ code: 'invalid_input', message: 'requestId must match [A-Za-z0-9_-]{8,64}' });
    }
    activeRequests.get(requestId)?.abort();
    return null;
  });

  return () => {
    for (const controller of activeRequests.values()) {
      controller.abort();
    }
    activeRequests.clear();
  };
}

async function runAIStream(
  deps: IpcDeps,
  requestId: string,
  query: string,
  controller: AbortController,
): Promise<void> {
  const send = (type: AIStreamEventType, data: unknown): void => {
    deps.notify(AI_EVENT_CHANNEL, { requestId, type, data });
  };

  try {
    await deps.gateway.streamEvents(
      '/api/ai/query',
      { query, stream: true },
      controller.signal,
      (eventName, data) => {
        if (
          eventName === 'start' ||
          eventName === 'status' ||
          eventName === 'delta' ||
          eventName === 'meta' ||
          eventName === 'done' ||
          eventName === 'error'
        ) {
          send(eventName, data);
        }
        // Unknown event names are intentionally dropped.
      },
    );
  } catch (err) {
    const payload = toErrorPayload(err);
    if (payload.code === 'aborted') {
      // The renderer initiated the cancel; still emit so any state resets.
      send('error', { error: 'aborted' });
    } else {
      send('error', { error: payload.detail ?? payload.code, code: payload.code });
    }
  }
}
