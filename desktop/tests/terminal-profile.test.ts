import { beforeEach, describe, expect, it } from 'vitest';
import { loadLocalTerminalProfile, saveLocalTerminalProfile } from '../src/lib/terminal-profile';

describe('local SSH terminal profiles', () => {
  beforeEach(() => Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: memoryStorage() }));

  it('keeps SSH usernames separate for each platform user and node', () => {
    saveLocalTerminalProfile(10, 4, { targetHost: 'gpu-a', sshUser: 'alice', useSshConfig: false });
    saveLocalTerminalProfile(11, 4, { targetHost: 'gpu-a', sshUser: 'bob', useSshConfig: false });
    expect(loadLocalTerminalProfile(10, 4)?.sshUser).toBe('alice');
    expect(loadLocalTerminalProfile(11, 4)?.sshUser).toBe('bob');
    expect(loadLocalTerminalProfile(10, 5)).toBeNull();
  });
});

function memoryStorage(): Storage {
  const values = new Map<string, string>();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => { values.set(String(key), String(value)); },
    removeItem: (key) => { values.delete(key); },
    clear: () => values.clear(),
    key: (index) => [...values.keys()][index] ?? null,
    get length() { return values.size; },
  };
}
