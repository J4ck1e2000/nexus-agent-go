import { gatewayError } from '../services/errors';
import type { ConversationMessage, ConversationSummary } from '../types/ipc';

const MAX_TITLE_LENGTH = 256;
const MAX_MESSAGE_LENGTH = 128 * 1024;

/** Runtime shape of one stored message's meta payload (snake_case gateway contract). */
function sanitizeMessageMeta(value: unknown): Record<string, unknown> | null {
  if (value === null || value === undefined) return null;
  if (typeof value !== 'object' || Array.isArray(value)) {
    throw gatewayError({ code: 'invalid_response', message: 'Conversation message meta must be an object' });
  }
  return value as Record<string, unknown>;
}

function sanitizeTimestamp(value: unknown): string | undefined {
  return typeof value === 'string' && value.length > 0 && value.length <= 64 ? value : undefined;
}

export function sanitizeConversationSummary(value: unknown): ConversationSummary {
  const obj = value !== null && typeof value === 'object' ? (value as Record<string, unknown>) : {};
  const id = obj.id;
  if (typeof id !== 'number' || !Number.isInteger(id) || id <= 0) {
    throw gatewayError({ code: 'invalid_response', message: 'Conversation payload has an invalid id' });
  }
  const rawTitle = typeof obj.title === 'string' ? obj.title : '';
  const summary: ConversationSummary = {
    id,
    title: rawTitle.slice(0, MAX_TITLE_LENGTH),
  };
  const createdAt = sanitizeTimestamp(obj.created_at);
  if (createdAt) summary.created_at = createdAt;
  const updatedAt = sanitizeTimestamp(obj.updated_at);
  if (updatedAt) summary.updated_at = updatedAt;
  return summary;
}

export function sanitizeConversationList(value: unknown): ConversationSummary[] {
  const list = value !== null && typeof value === 'object' ? (value as Record<string, unknown>).conversations : value;
  if (!Array.isArray(list)) {
    throw gatewayError({ code: 'invalid_response', message: 'Expected a conversation list' });
  }
  return list.map(sanitizeConversationSummary);
}

export function sanitizeConversationMessage(value: unknown): ConversationMessage {
  const obj = value !== null && typeof value === 'object' ? (value as Record<string, unknown>) : {};
  const id = obj.id;
  if (typeof id !== 'number' || !Number.isInteger(id) || id <= 0) {
    throw gatewayError({ code: 'invalid_response', message: 'Conversation message has an invalid id' });
  }
  if (obj.role !== 'user' && obj.role !== 'assistant') {
    throw gatewayError({ code: 'invalid_response', message: 'Conversation message has an invalid role' });
  }
  if (typeof obj.content !== 'string' || obj.content.length > MAX_MESSAGE_LENGTH) {
    throw gatewayError({ code: 'invalid_response', message: 'Conversation message has invalid content' });
  }
  const message: ConversationMessage = {
    id,
    role: obj.role,
    content: obj.content,
    meta: sanitizeMessageMeta(obj.meta),
  };
  const createdAt = sanitizeTimestamp(obj.created_at);
  if (createdAt) message.created_at = createdAt;
  return message;
}

export function sanitizeConversationMessageList(value: unknown): ConversationMessage[] {
  const list = value !== null && typeof value === 'object' ? (value as Record<string, unknown>).messages : value;
  if (!Array.isArray(list)) {
    throw gatewayError({ code: 'invalid_response', message: 'Expected a conversation message list' });
  }
  return list.map(sanitizeConversationMessage);
}
