import { useCallback, useEffect, useState } from 'react';
import type { ConversationSummary } from '../../../electron/types/ipc';
import { useAIStream } from '../../hooks/useAIStream';
import { useLanguage } from '../../hooks/useLanguage';
import { localizedError } from '../../lib/errors';
import ConfirmDialog from '../common/ConfirmDialog';
import AIChatWidget from './AIChatWidget';
import FloatingAIChatButton from './FloatingAIChatButton';

/** Composition of the floating AI assistant (launcher + chat widget + history). */
export default function AIAssistant({ selectedNodeName }: { selectedNodeName: string | null }) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const ai = useAIStream(t);

  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ConversationSummary | null>(null);
  const [deleting, setDeleting] = useState(false);

  const refreshConversations = useCallback(async (): Promise<void> => {
    const result = await window.nexus.conversations.list();
    if (result.ok) {
      setConversations(result.data);
      setListError(null);
    } else {
      setListError(localizedError(result.error, t));
    }
  }, [t]);

  // Load the history list when the panel opens, and refresh it after a
  // streaming exchange completes so the newest conversation/title shows up.
  useEffect(() => {
    if (!open || ai.isStreaming) return;
    void refreshConversations();
  }, [open, historyOpen, ai.isStreaming, refreshConversations]);

  const handleSelectConversation = useCallback(
    (id: number): void => {
      void ai.openConversation(id).then((loaded) => {
        if (loaded) setHistoryOpen(false);
      });
    },
    [ai],
  );

  const handleConfirmDelete = useCallback((): void => {
    const target = deleteTarget;
    if (!target || deleting) return;
    setDeleting(true);
    void window.nexus.conversations
      .remove(target.id)
      .then(async (result) => {
        setDeleting(false);
        setDeleteTarget(null);
        if (!result.ok) {
          setListError(localizedError(result.error, t));
          return;
        }
        if (ai.conversationId === target.id) {
          ai.newConversation();
        }
        await refreshConversations();
      });
  }, [deleteTarget, deleting, t, ai, refreshConversations]);

  return (
    <>
      <AIChatWidget
        open={open}
        messages={ai.messages}
        isStreaming={ai.isStreaming}
        streamError={ai.streamError}
        thinkingLabel={ai.thinkingLabel}
        sessionId={ai.sessionId}
        selectedNodeName={selectedNodeName}
        conversations={conversations}
        activeConversationId={ai.conversationId}
        historyOpen={historyOpen}
        historyError={listError}
        onToggleHistory={() => setHistoryOpen((value) => !value)}
        onSelectConversation={handleSelectConversation}
        onDeleteConversation={setDeleteTarget}
        onNewChat={() => {
          ai.newConversation();
          setHistoryOpen(false);
        }}
        onSubmit={(query) => void ai.submit(query)}
        onStop={ai.stop}
        onClose={() => setOpen(false)}
        onDismissError={ai.clearError}
      />
      <ConfirmDialog
        open={deleteTarget !== null}
        title={t('ai.deleteConversationTitle')}
        description={t('ai.deleteConversationDescription', {
          title: deleteTarget?.title || t('ai.untitledConversation'),
        })}
        confirmTone="danger"
        confirmLabel={t('ai.deleteConversation')}
        busy={deleting}
        onConfirm={handleConfirmDelete}
        onCancel={() => setDeleteTarget(null)}
      />
      <FloatingAIChatButton
        open={open}
        isStreaming={ai.isStreaming}
        onToggle={() => setOpen((value) => !value)}
      />
    </>
  );
}
