import type { ToolCallRecord } from '../../electron/types/ipc';
import type { Translator } from '../i18n';

/**
 * AI bubble text sanitizer ported from the web dashboard: unwrap code fences
 * and stringified JSON payloads (bounded recursion), surfacing the answer text.
 */
export function sanitizeAIText(raw: string, depth = 0): string {
  if (depth > 4) return raw;

  let text = raw.trim();
  const fenced = text.match(/^```[a-zA-Z]*\n([\s\S]*?)\n?```$/);
  if (fenced) {
    text = fenced[1].trim();
  }
  text = text.replace(/```json\n([\s\S]*?)```/g, (_match, inner: string) => inner.trim());

  try {
    const parsed: unknown = JSON.parse(text);
    if (typeof parsed === 'string') {
      return sanitizeAIText(parsed, depth + 1);
    }
    if (parsed !== null && typeof parsed === 'object') {
      const obj = parsed as Record<string, unknown>;
      for (const key of ['answer', 'content', 'text', 'message']) {
        const value = obj[key];
        if (typeof value === 'string' && value.trim()) {
          return sanitizeAIText(value, depth + 1);
        }
      }
      for (const value of Object.values(obj)) {
        if (typeof value === 'string' && value.trim()) {
          return sanitizeAIText(value, depth + 1);
        }
      }
    }
  } catch {
    // Not JSON; keep the text as-is.
  }

  return text;
}

const KNOWN_AI_ERROR_CODES = new Set([
  'ai_disabled',
  'invalid_query',
  'ai_query_failed',
  'ai_unavailable',
  'unauthorized',
]);

/** Map an SSE error code to a localized message (web localizeAIError parity). */
export function localizeAIError(code: string | undefined, t: Translator): string {
  if (code === 'aborted') {
    return t('ai.stopped');
  }
  if (code && KNOWN_AI_ERROR_CODES.has(code)) {
    return t(`errors.${code}`);
  }
  return t('ai.errorWithCode', { code: code ?? 'unknown' });
}

export interface AIMetaPatch {
  reasoningSummary?: string;
  mode?: string;
  relatedNodes?: string[];
  toolCalls?: ToolCallRecord[];
  warnings?: string[];
}

function asStringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  return value.filter((item): item is string => typeof item === 'string');
}

function asToolCalls(value: unknown): ToolCallRecord[] | undefined {
  if (!Array.isArray(value)) return undefined;
  return value
    .filter(
      (item): item is ToolCallRecord =>
        item !== null && typeof item === 'object' && typeof (item as ToolCallRecord).name === 'string',
    )
    .map((item) => ({
      name: item.name,
      args:
        item.args && typeof item.args === 'object' && !Array.isArray(item.args)
          ? (item.args as Record<string, unknown>)
          : {},
    }));
}

/**
 * Normalize a `meta` (or `done`) payload, accepting both snake_case gateway
 * fields and their camelCase aliases (web parity).
 */
export function normalizeAIPayload(payload: unknown): AIMetaPatch {
  if (payload === null || typeof payload !== 'object') {
    return {};
  }
  const obj = payload as Record<string, unknown>;
  const patch: AIMetaPatch = {};

  const reasoning = obj.reasoning_summary ?? obj.reasoningSummary;
  if (typeof reasoning === 'string' && reasoning.trim()) {
    patch.reasoningSummary = reasoning;
  }
  if (typeof obj.mode === 'string' && obj.mode.trim()) {
    patch.mode = obj.mode;
  }
  const relatedNodes = asStringArray(obj.related_nodes ?? obj.relatedNodes);
  if (relatedNodes) patch.relatedNodes = relatedNodes;
  const toolCalls = asToolCalls(obj.tool_calls ?? obj.toolCalls);
  if (toolCalls) patch.toolCalls = toolCalls;
  const warnings = asStringArray(obj.warnings);
  if (warnings) patch.warnings = warnings;

  return patch;
}
