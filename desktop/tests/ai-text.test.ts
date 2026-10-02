import { describe, expect, it } from 'vitest';
import { localizeAIError, normalizeAIPayload, sanitizeAIText } from '../src/lib/ai-text';
import { createTranslator } from '../src/i18n';

describe('sanitizeAIText', () => {
  it('passes plain text through', () => {
    expect(sanitizeAIText('Node A is idle.')).toBe('Node A is idle.');
  });

  it('unwraps a fenced answer', () => {
    expect(sanitizeAIText('```text\nhello\n```')).toBe('hello');
  });

  it('unwraps a JSON string payload', () => {
    expect(sanitizeAIText('"{\\"answer\\": \\"final text\\"}"')).toBe('final text');
  });

  it('extracts the answer field from a JSON object', () => {
    expect(sanitizeAIText('{"answer": "42 GPUs idle", "mode": "agent"}')).toBe('42 GPUs idle');
  });

  it('caps recursion depth', () => {
    const deep = '"'.repeat(1) + '{"answer": "x"}' + '"'.repeat(1);
    expect(typeof sanitizeAIText(deep)).toBe('string');
  });
});

describe('normalizeAIPayload', () => {
  it('reads snake_case gateway fields', () => {
    const patch = normalizeAIPayload({
      reasoning_summary: 'checked nodes',
      mode: 'agent',
      related_nodes: ['node-a', 'node-b'],
      tool_calls: [{ name: 'nodes_overview', args: {} }],
      warnings: ['data is stale'],
    });
    expect(patch.reasoningSummary).toBe('checked nodes');
    expect(patch.mode).toBe('agent');
    expect(patch.relatedNodes).toEqual(['node-a', 'node-b']);
    expect(patch.toolCalls).toHaveLength(1);
    expect(patch.warnings).toEqual(['data is stale']);
  });

  it('accepts camelCase aliases and ignores junk tool calls', () => {
    const patch = normalizeAIPayload({
      reasoningSummary: 'r',
      relatedNodes: ['n1'],
      toolCalls: [{ name: 'x' }, null, 'nope'],
      warnings: 'not-an-array',
    });
    expect(patch.reasoningSummary).toBe('r');
    expect(patch.relatedNodes).toEqual(['n1']);
    expect(patch.toolCalls).toEqual([{ name: 'x', args: {} }]);
    expect(patch.warnings).toBeUndefined();
  });

  it('returns an empty patch for non-objects', () => {
    expect(normalizeAIPayload(null)).toEqual({});
    expect(normalizeAIPayload('text')).toEqual({});
  });
});

describe('localizeAIError', () => {
  const en = createTranslator('en');
  const zh = createTranslator('zh');

  it('maps known gateway codes', () => {
    expect(localizeAIError('ai_disabled', en)).toBe(en('errors.ai_disabled'));
    expect(localizeAIError('ai_unavailable', zh)).toBe(zh('errors.ai_unavailable'));
  });

  it('treats aborted as a stop, not an error', () => {
    expect(localizeAIError('aborted', en)).toBe(en('ai.stopped'));
  });

  it('falls back to the generic message with the code', () => {
    expect(localizeAIError('weird_code', en)).toBe('AI query failed: weird_code');
  });
});
