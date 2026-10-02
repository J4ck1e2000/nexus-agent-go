import { useState } from 'react';
import { useAIStream } from '../../hooks/useAIStream';
import { useLanguage } from '../../hooks/useLanguage';
import AIChatWidget from './AIChatWidget';
import FloatingAIChatButton from './FloatingAIChatButton';

/** Composition of the floating AI assistant (launcher + chat widget). */
export default function AIAssistant({ selectedNodeName }: { selectedNodeName: string | null }) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const ai = useAIStream(t);

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
        onSubmit={(query) => void ai.submit(query)}
        onStop={ai.stop}
        onClose={() => setOpen(false)}
        onDismissError={ai.clearError}
      />
      <FloatingAIChatButton
        open={open}
        isStreaming={ai.isStreaming}
        onToggle={() => setOpen((value) => !value)}
      />
    </>
  );
}
