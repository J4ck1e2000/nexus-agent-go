import { describe, expect, it } from 'vitest';
import { createSSEParser } from '../electron/lib/sse';

function collect(): { feed: (chunk: string) => void; events: Array<{ event: string; data: string }> } {
  const events: Array<{ event: string; data: string }> = [];
  const feed = createSSEParser((message) => events.push(message));
  return { feed, events };
}

describe('createSSEParser', () => {
  it('parses a basic event with name and JSON payload', () => {
    const { feed, events } = collect();
    feed('event: delta\ndata: {"text":"hello"}\n\n');
    expect(events).toEqual([{ event: 'delta', data: '{"text":"hello"}' }]);
  });

  it('handles events split across chunks', () => {
    const { feed, events } = collect();
    feed('event: st');
    feed('atus\ndata: {"ph');
    feed('ase":"thinking"}\n');
    feed('\n');
    expect(events).toEqual([{ event: 'status', data: '{"phase":"thinking"}' }]);
  });

  it('joins multi-line data fields with newlines', () => {
    const { feed, events } = collect();
    feed('data: line1\ndata: line2\n\n');
    expect(events).toEqual([{ event: 'message', data: 'line1\nline2' }]);
  });

  it('handles CRLF line endings', () => {
    const { feed, events } = collect();
    feed('event: done\r\ndata: {"answer":"ok"}\r\n\r\n');
    expect(events).toEqual([{ event: 'done', data: '{"answer":"ok"}' }]);
  });

  it('ignores comment keep-alive lines and unknown fields', () => {
    const { feed, events } = collect();
    feed(': ping\nid: 42\nevent: status\ndata: {}\nretry: 100\n\n');
    expect(events).toEqual([{ event: 'status', data: '{}' }]);
  });

  it('dispatches multiple events in one chunk', () => {
    const { feed, events } = collect();
    feed('event: a\ndata: 1\n\nevent: b\ndata: 2\n\n');
    expect(events.map((event) => event.event)).toEqual(['a', 'b']);
  });

  it('keeps buffered partial lines waiting for the terminator', () => {
    const { feed, events } = collect();
    feed('event: delta\ndata: {"text":"hi"}\n');
    expect(events).toEqual([]);
    feed('\n');
    expect(events).toHaveLength(1);
  });

  it('emits an event with no data field using an empty payload', () => {
    const { feed, events } = collect();
    feed('event: ping\n\n');
    expect(events).toEqual([{ event: 'ping', data: '' }]);
  });
});
