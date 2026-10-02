import type { GatewayErrorCode } from '../types/ipc';

export interface GatewayErrorPayload {
  code: GatewayErrorCode;
  status?: number;
  detail?: string;
  message: string;
}

export class GatewayError extends Error {
  readonly code: GatewayErrorCode;
  readonly status?: number;
  readonly detail?: string;

  constructor(payload: GatewayErrorPayload) {
    super(payload.message);
    this.name = 'GatewayError';
    this.code = payload.code;
    this.status = payload.status;
    this.detail = payload.detail;
  }
}

export function gatewayError(payload: GatewayErrorPayload): GatewayError {
  return new GatewayError(payload);
}

/** Map an HTTP status code from the Gateway to a normalized error code. */
export function codeFromStatus(status: number): GatewayErrorCode {
  switch (status) {
    case 400:
      return 'bad_request';
    case 401:
      return 'unauthorized';
    case 403:
      return 'forbidden';
    case 404:
      return 'not_found';
    case 409:
      return 'conflict';
    default:
      return status >= 500 ? 'server_error' : 'gateway_error';
  }
}

/**
 * Normalize any thrown value into a stable error payload that can cross IPC.
 * Messages must stay free of tokens, passwords and other secrets.
 */
export function toErrorPayload(err: unknown): GatewayErrorPayload {
  if (err instanceof GatewayError) {
    return { code: err.code, status: err.status, detail: err.detail, message: err.message };
  }
  if (err instanceof Error) {
    if (err.name === 'AbortError') {
      return { code: 'aborted', message: 'Request aborted' };
    }
    if (err.name === 'TimeoutError') {
      return { code: 'timeout', message: 'Request timed out' };
    }
    if (err.name === 'TypeError') {
      return { code: 'network', message: 'Gateway is unreachable' };
    }
    return { code: 'server_error', message: err.message };
  }
  return { code: 'server_error', message: 'Unknown error' };
}
