import { describe, expect, it } from 'vitest';

import {
  extractConversationId,
  mapConversationMessages,
} from '../src/lib/ai-conversation';
import {
  sanitizeConversationList,
  sanitizeConversationMessageList,
  sanitizeConversationSummary,
} from '../electron/lib/conversation-payload';
import type { ConversationMessage } from '../electron/types/ipc';

describe('extractConversationId', () => {
  it('reads the snake_case gateway field', () => {
    expect(extractConversationId({ conversation_id: 42 })).toBe(42);
  });

  it('accepts the camelCase alias', () => {
    expect(extractConversationId({ conversationId: 7 })).toBe(7);
  });

  it('rejects zero, negatives, fractions and non-objects', () => {
    expect(extractConversationId({ conversation_id: 0 })).toBeNull();
    expect(extractConversationId({ conversation_id: -3 })).toBeNull();
    expect(extractConversationId({ conversation_id: 1.5 })).toBeNull();
    expect(extractConversationId({ conversation_id: '9' })).toBeNull();
    expect(extractConversationId(null)).toBeNull();
    expect(extractConversationId('done')).toBeNull();
    expect(extractConversationId(undefined)).toBeNull();
  });
});

describe('mapConversationMessages', () => {
  it('maps stored messages into renderer bubbles with meta details', () => {
    const stored: ConversationMessage[] = [
      { id: 1, role: 'user', content: '看看集群', meta: null },
      {
        id: 2,
        role: 'assistant',
        content: '集群正常',
        meta: {
          reasoning_summary: '已查询节点概览',
          mode: 'agent',
          related_nodes: ['gpu-01'],
          tool_calls: [{ name: 'get_node_metrics', args: {} }],
          warnings: ['stale data'],
        },
      },
    ];

    const messages = mapConversationMessages(stored);
    expect(messages).toHaveLength(2);

    expect(messages[0]).toMatchObject({
      role: 'user',
      content: '看看集群',
      status: 'done',
      relatedNodes: [],
      toolCalls: [],
      warnings: [],
    });

    expect(messages[1].reasoningSummary).toBe('已查询节点概览');
    expect(messages[1].mode).toBe('agent');
    expect(messages[1].relatedNodes).toEqual(['gpu-01']);
    expect(messages[1].toolCalls).toEqual([{ name: 'get_node_metrics', args: {} }]);
    expect(messages[1].warnings).toEqual(['stale data']);
    expect(messages[0].id).not.toBe(messages[1].id);
  });

  it('keeps chronological order and defaults missing meta fields', () => {
    const stored: ConversationMessage[] = [
      { id: 5, role: 'user', content: 'q', meta: {} },
      { id: 9, role: 'assistant', content: 'a', meta: null },
    ];
    const messages = mapConversationMessages(stored);
    expect(messages[0].content).toBe('q');
    expect(messages[1].content).toBe('a');
    expect(messages[1].relatedNodes).toEqual([]);
  });
});

describe('conversation payload sanitizers (main process)', () => {
  it('sanitizes a conversation summary and list envelopes', () => {
    const summary = sanitizeConversationSummary({
      id: 3,
      title: '帮我看看 GPU',
      user_id: 1,
      extra: 'dropped',
    });
    expect(summary).toEqual({ id: 3, title: '帮我看看 GPU' });

    const list = sanitizeConversationList({
      conversations: [{ id: 1, title: '', created_at: '2026-10-04T00:00:00Z' }, { id: 2 }],
    });
    expect(list).toEqual([
      { id: 1, title: '', created_at: '2026-10-04T00:00:00Z' },
      { id: 2, title: '' },
    ]);

    expect(() => sanitizeConversationSummary({ id: 0 })).toThrow();
    expect(() => sanitizeConversationList({ conversations: 'nope' })).toThrow();
    expect(() => sanitizeConversationList(null)).toThrow();
  });

  it('sanitizes message lists and rejects bad roles or content', () => {
    const messages = sanitizeConversationMessageList({
      messages: [
        { id: 1, role: 'user', content: 'q', meta: null },
        { id: 2, role: 'assistant', content: 'a', meta: { mode: 'agent' } },
      ],
    });
    expect(messages[0].meta).toBeNull();
    expect(messages[1].meta).toEqual({ mode: 'agent' });

    expect(() =>
      sanitizeConversationMessageList({ messages: [{ id: 3, role: 'system', content: 'x' }] }),
    ).toThrow();
    expect(() =>
      sanitizeConversationMessageList({ messages: [{ id: 3, role: 'user', content: 42 }] }),
    ).toThrow();
    expect(() => sanitizeConversationMessageList({ nope: true })).toThrow();
  });
});
