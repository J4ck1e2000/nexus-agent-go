import { describe, expect, it } from 'vitest';
import {
  isValidPassword,
  isValidRequestId,
  isValidUsername,
  normalizeGatewayUrl,
  sanitizeAIQuery,
} from '../electron/lib/validate';
import { gatewayError } from '../electron/services/errors';

describe('normalizeGatewayUrl', () => {
  it('accepts http(s) URLs and strips trailing slashes', () => {
    expect(normalizeGatewayUrl('http://127.0.0.1:3000')).toBe('http://127.0.0.1:3000');
    expect(normalizeGatewayUrl('http://127.0.0.1:3000/')).toBe('http://127.0.0.1:3000');
    expect(normalizeGatewayUrl('  https://gw.example.com/// ')).toBe('https://gw.example.com');
  });

  it('keeps explicit paths', () => {
    expect(normalizeGatewayUrl('http://localhost:3000/gateway')).toBe('http://localhost:3000/gateway');
  });

  it('rejects non-http schemes, credentials and garbage', () => {
    expect(normalizeGatewayUrl('ftp://example.com')).toBeNull();
    expect(normalizeGatewayUrl('not a url')).toBeNull();
    expect(normalizeGatewayUrl('')).toBeNull();
    expect(normalizeGatewayUrl(null)).toBeNull();
    expect(normalizeGatewayUrl('http://user:pass@example.com')).toBeNull();
    expect(normalizeGatewayUrl(`http://example.com/${'x'.repeat(3000)}`)).toBeNull();
  });
});

describe('credential validators', () => {
  it('mirrors gateway username rules', () => {
    expect(isValidUsername('abc')).toBe(true);
    expect(isValidUsername('a.b_c-d9')).toBe(true);
    expect(isValidUsername('ab')).toBe(false);
    expect(isValidUsername('bad space')).toBe(false);
    expect(isValidUsername('x'.repeat(65))).toBe(false);
  });

  it('requires passwords of at least 6 characters', () => {
    expect(isValidPassword('123456')).toBe(true);
    expect(isValidPassword('12345')).toBe(false);
  });

  it('accepts requestIds matching the IPC contract', () => {
    expect(isValidRequestId('a1b2c3d4e5')).toBe(true);
    expect(isValidRequestId('3f8a7c6e-1234-4abc-9def-567890abcdef')).toBe(true);
    expect(isValidRequestId('short')).toBe(false);
    expect(isValidRequestId('has space')).toBe(false);
  });
});

describe('sanitizeAIQuery', () => {
  it('trims and accepts reasonable queries', () => {
    expect(sanitizeAIQuery('  哪个节点现在最空闲?  ')).toBe('哪个节点现在最空闲?');
  });

  it('rejects empty or oversized queries', () => {
    expect(() => sanitizeAIQuery('   ')).toThrowError(gatewayError({ code: 'invalid_input', message: '' }).constructor);
    expect(() => sanitizeAIQuery(42)).toThrowError();
    expect(() => sanitizeAIQuery('x'.repeat(8001))).toThrowError();
  });
});
