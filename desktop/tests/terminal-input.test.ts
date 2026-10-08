import { describe, expect, it } from 'vitest';
import { splitTerminalInput } from '../src/lib/terminal-input';

describe('terminal input chunks', () => {
  it('keeps keystrokes and control sequences intact', () => {
    expect(splitTerminalInput('\x03')).toEqual(['\x03']);
    expect(splitTerminalInput('\x1b[200~probe\r\n\x1b[201~')).toEqual(['\x1b[200~probe\r\n\x1b[201~']);
    expect(splitTerminalInput('')).toEqual([]);
  });
  it('preserves Unicode and order for a paste larger than the IPC byte limit', () => {
    const data = 'abc中文😀\r\n'.repeat(4000);
    const chunks = splitTerminalInput(data);
    expect(chunks.length).toBeGreaterThan(1);
    expect(chunks.join('')).toBe(data);
    for (const chunk of chunks) {
      expect(new TextEncoder().encode(chunk).length).toBeLessThanOrEqual(16 * 1024);
      expect(chunk).not.toContain('\ufffd');
      expect(chunk).not.toMatch(/^[\udc00-\udfff]|[\ud800-\udbff]$/);
    }
  });
});
