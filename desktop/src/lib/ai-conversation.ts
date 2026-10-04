import type { ConversationMessage } from '../../electron/types/ipc';
import type { AIMessage } from '../hooks/useAIStream';
import { normalizeAIPayload } from './ai-text';

/**
 * Pure helpers for the conversation-history feature: extracting the active
 * conversation id from SSE payloads and mapping stored messages back into
 * renderer message shapes.
 */

/** Extract `conversation_id` (snake_case or camelCase) from a meta/done payload. */
export function extractConversationId(payload: unknown): number | null {
  if (payload === null || typeof payload !== 'object') return null;
  const obj = payload as Record<string, unknown>;
  const raw = obj.conversation_id ?? obj.conversationId;
  if (typeof raw !== 'number' || !Number.isInteger(raw) || raw <= 0) return null;
  return raw;
}

/** Map one stored gateway message into the renderer bubble shape (status done). */
export function mapConversationMessage(message: ConversationMessage, index: number): AIMessage {
  const patch = normalizeAIPayload(message.meta);
  return {
    id: `conv-${message.id}-${index}`,
    role: message.role,
    content: message.content,
    status: 'done',
    reasoningSummary: patch.reasoningSummary,
    mode: patch.mode,
    relatedNodes: patch.relatedNodes ?? [],
    toolCalls: patch.toolCalls ?? [],
    warnings: patch.warnings ?? [],
  };
}

/** Map a whole stored conversation; gateway returns messages in chronological order. */
export function mapConversationMessages(messages: ConversationMessage[]): AIMessage[] {
  return messages.map(mapConversationMessage);
}
