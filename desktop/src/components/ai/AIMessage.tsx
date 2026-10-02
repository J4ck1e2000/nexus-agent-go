import type { AIMessage } from '../../hooks/useAIStream';
import { sanitizeAIText } from '../../lib/ai-text';
import { useLanguage } from '../../hooks/useLanguage';
import AIThinking from './AIThinking';

/** One conversation bubble (user = dark right, assistant = light left). */
export default function AIMessageBubble({
  message,
  thinkingLabel,
}: {
  message: AIMessage;
  thinkingLabel?: string | null;
}) {
  const { t } = useLanguage();

  if (message.role === 'user') {
    return (
      <div className="flex justify-end">
        <div className="max-w-[85%] rounded-2xl rounded-br-md bg-ink-strong px-4 py-2.5 text-sm leading-relaxed text-[#f8f4ed]">
          <p className="whitespace-pre-wrap break-words">{message.content}</p>
        </div>
      </div>
    );
  }

  const showThinking = message.status === 'thinking' && !message.content;
  const hasDetails = Boolean(
    message.reasoningSummary ||
      (message.relatedNodes.length > 0) ||
      (message.toolCalls.length > 0) ||
      (message.warnings.length > 0) ||
      message.mode,
  );

  return (
    <div className="flex justify-start">
      <div className="max-w-[88%] rounded-2xl rounded-bl-md border border-line bg-panel-soft px-4 py-2.5 text-sm leading-relaxed text-ink">
        {showThinking ? (
          <AIThinking label={thinkingLabel || t('ai.statusThinking')} />
        ) : (
          <p className="whitespace-pre-wrap break-words">
            {sanitizeAIText(message.content)}
            {message.status === 'streaming' && (
              <span className="ml-0.5 inline-block h-3.5 w-[2px] animate-pulse bg-accent align-middle" aria-hidden="true" />
            )}
          </p>
        )}

        {message.status === 'error' && !message.content && (
          <p className="mt-1 text-xs text-danger">{t('ai.failedTitle')}</p>
        )}

        {hasDetails && (
          <details className="mt-2 rounded-xl bg-panel px-3 py-2 text-xs">
            <summary className="cursor-pointer select-none font-medium text-muted">
              {t('ai.detailsTitle')}
            </summary>
            <dl className="mt-2 flex flex-col gap-1.5">
              {message.reasoningSummary && (
                <Detail label={t('ai.detailReasoning')} value={message.reasoningSummary} />
              )}
              {message.mode && <Detail label={t('ai.detailMode')} value={message.mode} mono />}
              {message.relatedNodes.length > 0 && (
                <Detail label={t('ai.detailNodes')} value={message.relatedNodes.join(', ')} mono />
              )}
              {message.toolCalls.length > 0 && (
                <Detail
                  label={t('ai.detailTools')}
                  value={message.toolCalls.map((call) => call.name).join(', ')}
                  mono
                />
              )}
              {message.warnings.length > 0 && (
                <div>
                  <dt className="text-[10px] uppercase tracking-wide text-muted">
                    {t('ai.detailWarnings')}
                  </dt>
                  {message.warnings.map((warning, index) => (
                    <dd key={index} className="mt-0.5 text-danger">
                      {warning}
                    </dd>
                  ))}
                </div>
              )}
            </dl>
          </details>
        )}
      </div>
    </div>
  );
}

function Detail({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <dt className="text-[10px] uppercase tracking-wide text-muted">{label}</dt>
      <dd className={`mt-0.5 break-words text-ink ${mono ? 'font-mono text-[11px]' : ''}`}>{value}</dd>
    </div>
  );
}
