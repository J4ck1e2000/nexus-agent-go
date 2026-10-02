import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { SettingsStore } from '../electron/services/settings-store';

describe('SettingsStore', () => {
  let file: string;

  beforeEach(() => {
    file = path.join(mkdtempSync(path.join(tmpdir(), 'nexus-settings-')), 'settings.json');
  });

  afterEach(() => {
    rmSync(path.dirname(file), { recursive: true, force: true });
  });

  it('defaults to the local gateway when no file exists', () => {
    const store = new SettingsStore(file);
    expect(store.load()).toEqual({ gatewayUrl: 'http://127.0.0.1:3000' });
  });

  it('saves and reloads a valid gateway URL', () => {
    const store = new SettingsStore(file);
    const saved = store.save({ gatewayUrl: 'http://10.0.0.8:3000/' });
    expect(saved.gatewayUrl).toBe('http://10.0.0.8:3000');
    expect(store.load().gatewayUrl).toBe('http://10.0.0.8:3000');
  });

  it('rejects invalid URLs on save', () => {
    const store = new SettingsStore(file);
    expect(() => store.save({ gatewayUrl: 'not a url' })).toThrowError();
  });

  it('falls back to defaults for a corrupt settings file', () => {
    const store = new SettingsStore(file);
    store.save({ gatewayUrl: 'http://10.0.0.8:3000' });
    const { writeFileSync } = require('node:fs') as typeof import('node:fs');
    writeFileSync(file, '{not json');
    expect(store.load()).toEqual({ gatewayUrl: 'http://127.0.0.1:3000' });
  });
});
