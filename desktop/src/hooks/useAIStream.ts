import { useCallback, useEffect, useRef, useState } from 'react';
import type { ToolCallRecord } from '../../electron/types/ipc';
import { localizeAIError, normalizeAIPayload, type AIMetaPatch } from '../lib/ai-text';
import type { Translator } from '../i18n';

export type AIMessageStatus = 'thinking' | 'streaming' | 'done' | 'error';

export interface AIMessage {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  status: AIMessageStatus;
  reasoningSummary?: string;
  mode?: string;
  relatedNodes: string[];
  toolCalls: ToolCallRecord[];
  warnings: string[];
}

const THINKING_ROTATION_KEYS = ['ai.statusThinking', 'ai.statusTooling', 'ai.statusGenerating'];
const THINKING_ROTATION_MS = 1350;

function createMessageId(): string {
  return `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
}

function createRequestId(): string {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `req-${Date.now()}-${Math.random().toString(16).slice(2, 10)}`;
}

function mergeMeta(message: AIMessage, patch: AIMetaPatch): AIMessage {
  const next: AIMessage = { ...message };
  // Prefer non-empty incoming values (web parity).
  if (patch.reasoningSummary) next.reasoningSummary = patch.reasoningSummary;
  if (patch.mode) next.mode = patch.mode;
  if (patch.relatedNodes && patch.relatedNodes.length > 0) next.relatedNodes = patch.relatedNodes;
  if (patch.toolCalls && patch.toolCalls.length > 0) next.toolCalls = patch.toolCalls;
  if (patch.warnings && patch.warnings.length > 0) next.warnings = patch.warnings;
  return next;
}

function statusLabelText(payload: unknown, t: Translator): string {
  const obj = payload !== null && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
  const phase = typeof obj.phase === 'string' ? obj.phase.trim() : '';
  const message = typeof obj.message === 'string' ? obj.message.trim() : '';
  switch (phase) {
    case 'thinking':
      return t('ai.statusThinking');
    case 'tooling':
      return t('ai.statusTooling');
    case 'generating':
      return t('ai.statusGenerating');
    default:
      break;
  }
  if (message) return message;
  return t('ai.statusFallback');
}

/**
 * AI assistant conversation state: submit queries over IPC, receive SSE
 * events tagged with requestId, render thinking/streaming/done/error states.
 */
export function useAIStream(t: Translator): {
  messages: AIMessage[];
  isStreaming: boolean;
  streamError: string | null;
  thinkingLabel: string | null;
  sessionId: string;
  submit: (query: string) => Promise<void>;
  stop: () => void;
  clearError: () => void;
} {
  const [messages, setMessages] = useState<AIMessage[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [streamError, setStreamError] = useState<string | null>(null);
  const [liveStatusLabel, setLiveStatusLabel] = useState<string | null>(null);
  const [rotationIndex, setRotationIndex] = useState(0);

  const activeRequestIdRef = useRef<string | null>(null);
  const assistantIdRef = useRef<string | null>(null);
  const abortRequestedRef = useRef(false);
  const tRef = useRef(t);
  tRef.current = t;

  const sessionIdRef = useRef(Math.random().toString(16).slice(2, 10));

  const finalizeAssistant = useCallback((status: AIMessageStatus, stoppedText?: string): void => {
    const assistantId = assistantIdRef.current;
    if (assistantId) {
      setMessages((current) =>
        current.map((message) =>
          message.id === assistantId
            ? {
                ...message,
                status: message.content || stoppedText ? status : 'error',
                content: stoppedText && !message.content ? stoppedText : message.content,
              }
            : message,
        ),
      );
    }
  }, []);

  // Live SSE event handling; events for stale requestIds are ignored.
  useEffect(() => {
    const unsubscribe = window.nexus.ai.onEvent((event) => {
      if (event.requestId !== activeRequestIdRef.current) return;
      const assistantId = assistantIdRef.current;
      if (!assistantId) return;
      const translator = tRef.current;

      switch (event.type) {
        case 'status': {
          setLiveStatusLabel(statusLabelText(event.data, translator));
          break;
        }
        case 'delta': {
          const payload = event.data as { text?: unknown } | null;
          const text = typeof payload?.text === 'string' ? payload.text : '';
          if (!text) return;
          setLiveStatusLabel(null);
          setMessages((current) =>
            current.map((message) =>
              message.id === assistantId
                ? { ...message, content: message.content + text, status: 'streaming' }
                : message,
            ),
          );
          break;
        }
        case 'meta': {
          setMessages((current) =>
            current.map((message) =>
              message.id === assistantId ? mergeMeta(message, normalizeAIPayload(event.data)) : message,
            ),
          );
          break;
        }
        case 'done': {
          setLiveStatusLabel(null);
          const patch = normalizeAIPayload(event.data);
          const answer =
            event.data !== null &&
            typeof event.data === 'object' &&
            typeof (event.data as Record<string, unknown>).answer === 'string'
              ? ((event.data as Record<string, unknown>).answer as string)
              : '';
          setMessages((current) =>
            current.map((message) => {
              if (message.id !== assistantId) return message;
              const merged = mergeMeta(message, patch);
              return { ...merged, content: answer || merged.content, status: 'done' as const };
            }),
          );
          setIsStreaming(false);
          activeRequestIdRef.current = null;
          assistantIdRef.current = null;
          break;
        }
        case 'error': {
          setLiveStatusLabel(null);
          const payload = event.data as { error?: unknown } | null;
          const code = typeof payload?.error === 'string' ? payload.error : undefined;
          if (code === 'aborted') {
            if (abortRequestedRef.current) {
              setStreamError(translator('ai.stopped'));
            }
            finalizeAssistant('done');
          } else {
            setStreamError(localizeAIError(code, translator));
            finalizeAssistant('done');
          }
          setIsStreaming(false);
          activeRequestIdRef.current = null;
          assistantIdRef.current = null;
          break;
        }
        default:
          break;
      }
    });
    return unsubscribe;
  }, [finalizeAssistant]);

  // Rotate thinking phrases while waiting for the first status/delta.
  useEffect(() => {
    if (!isStreaming || liveStatusLabel !== null) return;
    const timer = setInterval(() => {
      setRotationIndex((index) => (index + 1) % THINKING_ROTATION_KEYS.length);
    }, THINKING_ROTATION_MS);
    return () => clearInterval(timer);
  }, [isStreaming, liveStatusLabel]);

  const thinkingLabel =
    liveStatusLabel ?? (isStreaming ? THINKING_ROTATION_KEYS[rotationIndex] : null);

  const submit = useCallback(
    async (query: string): Promise<void> => {
      const trimmed = query.trim();
      if (!trimmed || isStreaming) return;

      setStreamError(null);
      setLiveStatusLabel(null);
      abortRequestedRef.current = false;

      const userMessage: AIMessage = {
        id: createMessageId(),
        role: 'user',
        content: trimmed,
        status: 'done',
        relatedNodes: [],
        toolCalls: [],
        warnings: [],
      };
      const assistantMessage: AIMessage = {
        id: createMessageId(),
        role: 'assistant',
        content: '',
        status: 'thinking',
        relatedNodes: [],
        toolCalls: [],
        warnings: [],
      };
      setMessages((current) => [...current, userMessage, assistantMessage]);
      assistantIdRef.current = assistantMessage.id;
      setIsStreaming(true);

      const requestId = createRequestId();
      activeRequestIdRef.current = requestId;

      const result = await window.nexus.ai.start(requestId, trimmed);
      if (!result.ok) {
        setIsStreaming(false);
        activeRequestIdRef.current = null;
        assistantIdRef.current = null;
        setStreamError(localizeAIError(result.error.detail ?? result.error.code, tRef.current));
        setMessages((current) =>
          current.map((message) =>
            message.id === assistantMessage.id ? { ...message, status: 'error' as const } : message,
          ),
        );
      }
    },
    [isStreaming],
  );

  const stop = useCallback((): void => {
    const requestId = activeRequestIdRef.current;
    if (!requestId) return;
    abortRequestedRef.current = true;
    void window.nexus.ai.cancel(requestId);
    setIsStreaming(false);
    setLiveStatusLabel(null);
    setStreamError(tRef.current('ai.stopped'));
    finalizeAssistant('done', tRef.current('ai.stopped'));
    activeRequestIdRef.current = null;
    assistantIdRef.current = null;
  }, [finalizeAssistant]);

  // Cancel any in-flight stream when the assistant unmounts.
  useEffect(() => {
    return () => {
      const requestId = activeRequestIdRef.current;
      if (requestId) {
        void window.nexus.ai.cancel(requestId);
      }
    };
  }, []);

  const clearError = useCallback(() => setStreamError(null), []);

  return {
    messages,
    isStreaming,
    streamError,
    thinkingLabel,
    sessionId: sessionIdRef.current,
    submit,
    stop,
    clearError,
  };
}
