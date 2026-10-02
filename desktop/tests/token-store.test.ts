import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { TokenStore, type TokenCipher } from '../electron/services/token-store';

function fakeCipher(available: boolean): TokenCipher & { lastPlaintext: string | null } {
  return {
    lastPlaintext: null,
    isEncryptionAvailable: () => available,
    encryptString(plain: string) {
      this.lastPlaintext = plain;
      return Buffer.from(`enc:${Buffer.from(plain).toString('base64')}`);
    },
    decryptString(encrypted: Buffer) {
      const text = encrypted.toString('utf8');
      if (!text.startsWith('enc:')) throw new Error('not encrypted by this cipher');
      return Buffer.from(text.slice(4), 'base64').toString('utf8');
    },
  };
}

describe('TokenStore', () => {
  let dir: string;

  beforeEach(() => {
    dir = mkdtempSync(path.join(tmpdir(), 'nexus-token-'));
  });

  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it('round-trips a token through the encrypted path', () => {
    const cipher = fakeCipher(true);
    const store = new TokenStore({ dir, cipher });
    store.save('jwt-value');
    expect(cipher.lastPlaintext).toBe('jwt-value');
    expect(store.load()).toBe('jwt-value');
  });

  it('falls back to plaintext when the OS keyring is unavailable', () => {
    const cipher = fakeCipher(false);
    const store = new TokenStore({ dir, cipher });
    store.save('plain-jwt');
    expect(store.load()).toBe('plain-jwt');
  });

  it('returns null after clear', () => {
    const store = new TokenStore({ dir, cipher: fakeCipher(true) });
    store.save('jwt');
    store.clear();
    expect(store.load()).toBeNull();
    expect(() => store.clear()).not.toThrow();
  });

  it('treats an empty token as a clear', () => {
    const store = new TokenStore({ dir, cipher: fakeCipher(true) });
    store.save('jwt');
    store.save('   ');
    expect(store.load()).toBeNull();
  });

  it('returns null for a corrupted file instead of throwing', () => {
    const store = new TokenStore({ dir, cipher: fakeCipher(true) });
    store.save('jwt');
    const { writeFileSync } = require('node:fs') as typeof import('node:fs');
    writeFileSync(path.join(dir, 'nexus-token.bin'), Buffer.from([9, 1, 2, 3]));
    expect(store.load()).toBeNull();
  });

  it('returns null when no token was ever saved', () => {
    const store = new TokenStore({ dir, cipher: fakeCipher(true) });
    expect(store.load()).toBeNull();
  });
});
