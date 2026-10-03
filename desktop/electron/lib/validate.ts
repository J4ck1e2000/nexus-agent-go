import { gatewayError } from '../services/errors';

/**
 * Normalize a user-provided Gateway URL.
 * Returns null when the value is not a usable http(s) base URL.
 */
export function normalizeGatewayUrl(raw: unknown): string | null {
  if (typeof raw !== 'string') return null;
  const trimmed = raw.trim();
  if (!trimmed || trimmed.length > 2048) return null;

  let url: URL;
  try {
    url = new URL(trimmed);
  } catch {
    return null;
  }

  if (url.protocol !== 'http:' && url.protocol !== 'https:') return null;
  if (url.username || url.password) return null;
  if (url.hash) return null;

  let normalized = url.toString();
  while (normalized.length > 0 && normalized.endsWith('/')) {
    normalized = normalized.slice(0, -1);
  }
  return normalized;
}

export function requireGatewayUrl(raw: unknown): string {
  const normalized = normalizeGatewayUrl(raw);
  if (!normalized) {
    throw gatewayError({ code: 'invalid_input', message: 'Invalid gateway URL' });
  }
  return normalized;
}

const USERNAME_PATTERN = /^[A-Za-z0-9._-]+$/;

/** Mirrors the Gateway rules: 3-64 chars of [A-Za-z0-9._-]. */
export function isValidUsername(raw: unknown): raw is string {
  return (
    typeof raw === 'string' &&
    raw.length >= 3 &&
    raw.length <= 64 &&
    USERNAME_PATTERN.test(raw)
  );
}

export function isValidPassword(raw: unknown): raw is string {
  return typeof raw === 'string' && raw.length >= 6 && raw.length <= 256;
}

export function isValidRequestId(raw: unknown): raw is string {
  return typeof raw === 'string' && /^[A-Za-z0-9_-]{8,64}$/.test(raw);
}

export function isValidNodeName(raw: unknown): raw is string {
  return typeof raw === 'string' && raw.trim().length >= 1 && raw.trim().length <= 128;
}

export function isValidNodeUrl(raw: unknown): raw is string {
  if (typeof raw !== 'string') return false;
  const normalized = normalizeGatewayUrl(raw);
  return normalized !== null;
}

/** SSH host: non-empty, reasonable length (hostname / IPv4 / IPv6). */
export function isValidSSHHost(raw: unknown): raw is string {
  return typeof raw === 'string' && raw.trim().length >= 1 && raw.trim().length <= 255;
}

/** SSH port: 1..65535 integer. */
export function isValidSSHPort(raw: unknown): raw is number {
  return typeof raw === 'number' && Number.isInteger(raw) && raw >= 1 && raw <= 65535;
}

const SSH_USER_PATTERN = /^[a-zA-Z0-9._-]+$/;

/** Mirrors the Gateway rules: 1-64 chars of [a-zA-Z0-9._-]. */
export function isValidSSHUser(raw: unknown): raw is string {
  return (
    typeof raw === 'string' &&
    raw.length >= 1 &&
    raw.length <= 64 &&
    SSH_USER_PATTERN.test(raw)
  );
}

/** Validate a positive integer id used in path params. */
export function isValidId(raw: unknown): raw is number {
  return typeof raw === 'number' && Number.isInteger(raw) && raw > 0;
}

const MAX_QUERY_LENGTH = 8000;

export function sanitizeAIQuery(raw: unknown): string {
  if (typeof raw !== 'string') {
    throw gatewayError({ code: 'invalid_input', message: 'Query must be a string' });
  }
  const trimmed = raw.trim();
  if (!trimmed) {
    throw gatewayError({ code: 'invalid_input', message: 'Query must not be empty' });
  }
  if (trimmed.length > MAX_QUERY_LENGTH) {
    throw gatewayError({ code: 'invalid_input', message: 'Query is too long' });
  }
  return trimmed;
}
