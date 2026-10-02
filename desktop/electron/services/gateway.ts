import { createSSEParser } from '../lib/sse';
import { GatewayError, codeFromStatus, gatewayError } from './errors';

const DEFAULT_TIMEOUT_MS = 10_000;
export const CONNECTION_TEST_TIMEOUT_MS = 5_000;

export interface GatewayRequestOptions {
  body?: unknown;
  signal?: AbortSignal;
  timeoutMs?: number;
  /** Set false for public endpoints (login/register/version). */
  auth?: boolean;
  /** Override the configured base URL (used by Test Connection). */
  baseURL?: string;
}

export type SSEEventCallback = (eventName: string, data: unknown) => void;

/**
 * The single HTTP client for the Gateway. Lives in the main process so the
 * JWT never reaches the renderer and SSE streams do not depend on renderer
 * fetch semantics.
 */
export class GatewayClient {
  constructor(
    private readonly getBaseURL: () => string,
    private readonly getToken: () => string | null,
    private readonly hooks: { onUnauthorized?: () => void } = {},
  ) {}

  async request<T>(method: string, path: string, options: GatewayRequestOptions = {}): Promise<T> {
    const url = `${options.baseURL ?? this.getBaseURL()}${path}`;
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (options.auth !== false) {
      const token = this.getToken();
      if (token) {
        headers.Authorization = `Bearer ${token}`;
      }
    }
    if (options.body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }

    const timeoutSignal = AbortSignal.timeout(options.timeoutMs ?? DEFAULT_TIMEOUT_MS);
    const signal = options.signal
      ? AbortSignal.any([options.signal, timeoutSignal])
      : timeoutSignal;

    let response: Response;
    try {
      response = await fetch(url, {
        method,
        headers,
        body: options.body === undefined ? undefined : JSON.stringify(options.body),
        signal,
      });
    } catch (err) {
      throw this.mapTransportError(err, options.signal, timeoutSignal, url);
    }

    if (!response.ok) {
      const detail = await readErrorDetail(response);
      if (response.status === 401) {
        this.hooks.onUnauthorized?.();
      }
      throw new GatewayError({
        code: codeFromStatus(response.status),
        status: response.status,
        detail,
        message: `Gateway returned ${response.status}${detail ? ` (${detail})` : ''}`,
      });
    }

    try {
      return (await response.json()) as T;
    } catch {
      throw gatewayError({
        code: 'invalid_response',
        status: response.status,
        message: 'Gateway returned a non-JSON response',
      });
    }
  }

  /**
   * POST an SSE request and forward every parsed event to `onEvent`.
   * Resolves after the stream ends; rejects if no terminal event arrived.
   */
  async streamEvents(
    path: string,
    body: unknown,
    signal: AbortSignal,
    onEvent: SSEEventCallback,
  ): Promise<void> {
    const url = `${this.getBaseURL()}${path}`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream, application/json',
    };
    const token = this.getToken();
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }

    let response: Response;
    try {
      response = await fetch(url, { method: 'POST', headers, body: JSON.stringify(body), signal });
    } catch (err) {
      throw this.mapTransportError(err, signal, null, url);
    }

    if (!response.ok) {
      const detail = await readErrorDetail(response);
      if (response.status === 401) {
        this.hooks.onUnauthorized?.();
      }
      throw new GatewayError({
        code: codeFromStatus(response.status),
        status: response.status,
        detail,
        message: `Gateway returned ${response.status}${detail ? ` (${detail})` : ''}`,
      });
    }

    const contentType = response.headers.get('content-type') ?? '';
    if (contentType.includes('application/json')) {
      // Gateway answered with a plain JSON body instead of SSE.
      try {
        const payload: unknown = await response.json();
        onEvent('done', payload);
        return;
      } catch {
        throw gatewayError({
          code: 'invalid_response',
          status: response.status,
          message: 'Gateway returned a malformed JSON body',
        });
      }
    }

    if (!response.body) {
      throw gatewayError({
        code: 'invalid_response',
        status: response.status,
        message: 'Gateway stream has no body',
      });
    }

    let sawTerminalEvent = false;
    const feed = createSSEParser((message) => {
      let data: unknown = message.data;
      try {
        data = JSON.parse(message.data);
      } catch {
        // Non-JSON payload: forward the raw string.
      }
      if (message.event === 'done' || message.event === 'error') {
        sawTerminalEvent = true;
      }
      onEvent(message.event, data);
    });

    const reader = response.body.getReader();
    const decoder = new TextDecoder('utf-8');
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        feed(decoder.decode(value, { stream: true }));
      }
      feed(decoder.decode());
    } catch (err) {
      if (signal.aborted) {
        throw gatewayError({ code: 'aborted', message: 'Stream aborted' });
      }
      if (err instanceof GatewayError) throw err;
      throw gatewayError({ code: 'network', message: 'Gateway stream was interrupted' });
    }

    if (!sawTerminalEvent) {
      throw gatewayError({
        code: 'invalid_response',
        message: 'Gateway stream ended without a terminal event',
      });
    }
  }

  private mapTransportError(
    err: unknown,
    callerSignal: AbortSignal | undefined,
    timeoutSignal: AbortSignal | null,
    url: string,
  ): GatewayError {
    if (callerSignal?.aborted) {
      return gatewayError({ code: 'aborted', message: 'Request aborted' });
    }
    if (timeoutSignal?.aborted) {
      return gatewayError({ code: 'timeout', message: `Request to ${url} timed out` });
    }
    if (err instanceof GatewayError) {
      return err;
    }
    return gatewayError({ code: 'network', message: `Cannot reach gateway at ${url}` });
  }
}

async function readErrorDetail(response: Response): Promise<string | undefined> {
  try {
    const payload: unknown = await response.json();
    if (payload !== null && typeof payload === 'object') {
      const code = (payload as Record<string, unknown>).error;
      if (typeof code === 'string' && code.trim()) {
        return code.trim();
      }
    }
  } catch {
    // Body was not JSON; fall through.
  }
  return undefined;
}
