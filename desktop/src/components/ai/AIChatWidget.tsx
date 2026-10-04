import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import type { AIMessage } from '../../hooks/useAIStream';
import type { ConversationSummary } from '../../../electron/types/ipc';
import { useLanguage } from '../../hooks/useLanguage';
import AIMessageBubble from './AIMessage';

/**
 * Floating AI chat panel: header (history / new chat / close), streaming
 * message list or conversation history, quick prompts, details blocks, error
 * box and Enter-to-send input with Stop support.
 */
export default function AIChatWidget({
  open,
  messages,
  isStreaming,
  streamError,
  thinkingLabel,
  sessionId,
  selectedNodeName,
  conversations,
  activeConversationId,
  historyOpen,
  historyError,
  onToggleHistory,
  onSelectConversation,
  onDeleteConversation,
  onNewChat,
  onSubmit,
  onStop,
  onClose,
  onDismissError,
}: {
  open: boolean;
  messages: AIMessage[];
  isStreaming: boolean;
  streamError: string | null;
  thinkingLabel: string | null;
  sessionId: string;
  selectedNodeName: string | null;
  conversations: ConversationSummary[];
  activeConversationId: number | null;
  historyOpen: boolean;
  historyError: string | null;
  onToggleHistory: () => void;
  onSelectConversation: (id: number) => void;
  onDeleteConversation: (conversation: ConversationSummary) => void;
  onNewChat: () => void;
  onSubmit: (query: string) => void;
  onStop: () => void;
  onClose: () => void;
  onDismissError: () => void;
}) {
  const { t, language } = useLanguage();
  const [draft, setDraft] = useState('');
  const listRef = useRef<HTMLDivElement | null>(null);
  const nearBottomRef = useRef(true);

  // Node-specific prompts only make sense with a node selected; fall back to
  // fleet-wide prompts instead of referencing a hard-coded node name.
  const quickPrompts = selectedNodeName
    ? [
        t('ai.quick1'),
        t('ai.quick2'),
        t('ai.quick3', { node: selectedNodeName }),
        t('ai.quick4', { node: selectedNodeName }),
      ]
    : [t('ai.quick1'), t('ai.quick2'), t('ai.quickAll1'), t('ai.quickAll2')];

  // Esc closes the panel — unless a modal/dialog overlay is open, which
  // consumes Esc first (one press closes one layer).
  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key !== 'Escape') return;
      if (document.querySelector('[data-nexus-overlay="open"]')) return;
      event.preventDefault();
      onClose();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [open, onClose]);

  // Auto-scroll only when the user is near the bottom (web parity: 64px).
  useEffect(() => {
    const list = listRef.current;
    if (!list || !open) return;
    if (nearBottomRef.current) {
      list.scrollTop = list.scrollHeight;
    }
  }, [messages, thinkingLabel, open, historyOpen]);

  const handleScroll = (): void => {
    const list = listRef.current;
    if (!list) return;
    nearBottomRef.current = list.scrollHeight - list.scrollTop - list.clientHeight < 64;
  };

  const send = (value?: string): void => {
    const query = (value ?? draft).trim();
    if (!query || isStreaming) return;
    setDraft('');
    onSubmit(query);
  };

  const handleKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>): void => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      send();
    }
  };

  const formatTime = (value: string | undefined): string => {
    if (!value) return '';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    return date.toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  return (
    <div
      className={`fixed bottom-[86px] right-5 z-[70] flex w-[min(420px,calc(100vw-20px))] origin-bottom-right flex-col overflow-hidden rounded-3xl border border-line bg-panel shadow-soft transition-all duration-200 ${
        open ? 'pointer-events-auto scale-100 opacity-100' : 'pointer-events-none scale-95 opacity-0'
      }`}
      style={{ height: 'min(640px, calc(100vh - 110px))' }}
      role="dialog"
      aria-label={t('ai.title')}
      aria-hidden={!open}
    >
      <header className="flex items-start justify-between gap-2 border-b border-line px-5 py-4">
        <div>
          <h2 className="text-base font-semibold tracking-tight text-ink">{t('ai.title')}</h2>
          <p className="mt-0.5 text-[11px] text-muted">
            {isStreaming ? t('ai.subtitleStreaming') : t('ai.subtitleIdle')}
          </p>
        </div>
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            className={`cursor-pointer rounded-full p-1.5 transition hover:bg-panel-soft hover:text-ink ${
              historyOpen ? 'bg-panel-soft text-ink' : 'text-muted'
            }`}
            onClick={onToggleHistory}
            aria-label={t('ai.historyTitle')}
            title={t('ai.historyTitle')}
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
              <path d="M3 3v5h5" />
              <path d="M12 7v5l4 2" />
            </svg>
          </button>
          <button
            type="button"
            className="cursor-pointer rounded-full p-1.5 text-muted transition hover:bg-panel-soft hover:text-ink"
            onClick={onNewChat}
            aria-label={t('ai.newChat')}
            title={t('ai.newChat')}
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
              <path d="M12 7v6" />
              <path d="M9 10h6" />
            </svg>
          </button>
          <button
            type="button"
            className="cursor-pointer rounded-full p-1.5 text-muted transition hover:bg-panel-soft hover:text-ink"
            onClick={onClose}
            aria-label={t('action.close')}
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M18 6 6 18" />
              <path d="m6 6 12 12" />
            </svg>
          </button>
        </div>
      </header>

      <div
        ref={listRef}
        onScroll={handleScroll}
        className="custom-scrollbar flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-5 py-4"
      >
        {historyOpen ? (
          <div className="flex flex-col gap-2">
            {historyError && <p className="text-xs text-[#683833]">{historyError}</p>}
            {conversations.length === 0 ? (
              <p className="py-6 text-center text-xs text-muted">{t('ai.historyEmpty')}</p>
            ) : (
              conversations.map((conversation) => (
                <div
                  key={conversation.id}
                  className={`group flex items-center gap-2 rounded-xl px-3 py-2.5 transition ${
                    conversation.id === activeConversationId ? 'soft-panel-subtle' : 'hover:bg-panel-soft'
                  }`}
                >
                  <button
                    type="button"
                    className="min-w-0 flex-1 cursor-pointer text-left"
                    onClick={() => onSelectConversation(conversation.id)}
                  >
                    <p className="truncate text-xs font-medium text-ink">
                      {conversation.title || t('ai.untitledConversation')}
                    </p>
                    {formatTime(conversation.updated_at) && (
                      <p className="mt-0.5 text-[10px] text-muted">
                        {formatTime(conversation.updated_at)}
                      </p>
                    )}
                  </button>
                  <button
                    type="button"
                    className="shrink-0 cursor-pointer rounded-full p-1 text-muted opacity-0 transition group-hover:opacity-100 hover:bg-[#f2e3df] hover:text-[#683833] focus-visible:opacity-100"
                    onClick={() => onDeleteConversation(conversation)}
                    aria-label={t('ai.deleteConversation')}
                    title={t('ai.deleteConversation')}
                  >
                    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                      <path d="M3 6h18" />
                      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
                      <path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                      <path d="M10 11v6" />
                      <path d="M14 11v6" />
                    </svg>
                  </button>
                </div>
              ))
            )}
          </div>
        ) : messages.length === 0 ? (
          <div className="flex flex-col gap-3">
            <div className="soft-panel-subtle p-4">
              <p className="text-sm font-semibold text-ink">{t('ai.introTitle')}</p>
              <p className="mt-1 text-xs leading-relaxed text-muted">{t('ai.introBody')}</p>
            </div>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              {quickPrompts.map((prompt) => (
                <button
                  key={prompt}
                  type="button"
                  className="soft-panel-subtle cursor-pointer px-3.5 py-2.5 text-left text-xs text-muted transition hover:-translate-y-px hover:text-ink"
                  onClick={() => send(prompt)}
                >
                  {prompt}
                </button>
              ))}
            </div>
          </div>
        ) : (
          messages.map((message, index) => (
            <AIMessageBubble
              key={message.id}
              message={message}
              thinkingLabel={
                isStreaming && index === messages.length - 1 && message.status === 'thinking'
                  ? thinkingLabel
                  : null
              }
            />
          ))
        )}
      </div>

      {streamError && !historyOpen && (
        <div className="mx-5 mb-2 flex items-start justify-between gap-2 rounded-xl bg-[#f2e3df] px-3.5 py-2.5 text-xs text-[#683833]">
          <span className="leading-relaxed">{streamError}</span>
          <button
            type="button"
            className="shrink-0 cursor-pointer opacity-70 transition hover:opacity-100"
            onClick={onDismissError}
            aria-label={t('action.close')}
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M18 6 6 18" />
              <path d="m6 6 12 12" />
            </svg>
          </button>
        </div>
      )}

      <footer className="border-t border-line px-5 py-3.5">
        <textarea
          className="input-field min-h-[76px] resize-none py-3"
          value={draft}
          placeholder={t('ai.inputPlaceholder')}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={handleKeyDown}
          disabled={isStreaming}
          aria-label={t('ai.inputPlaceholder')}
        />
        <div className="mt-2 flex items-center justify-between">
          <span className="font-mono text-[10px] text-muted">
            {t('ai.sessionLabel')}: {sessionId}
          </span>
          {isStreaming ? (
            <button type="button" className="outlined-danger-button" onClick={onStop}>
              <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <rect x="6" y="6" width="12" height="12" rx="2" />
              </svg>
              {t('ai.stop')}
            </button>
          ) : (
            <button
              type="button"
              className="primary-button px-4 py-2"
              onClick={() => send()}
              disabled={!draft.trim()}
            >
              {t('ai.send')}
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="m22 2-7 20-4-9-9-4Z" />
                <path d="M22 2 11 13" />
              </svg>
            </button>
          )}
        </div>
      </footer>
    </div>
  );
}
