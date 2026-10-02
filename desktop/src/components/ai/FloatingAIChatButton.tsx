import { useLanguage } from '../../hooks/useLanguage';

/** Floating launcher with gradient, pulse while streaming and a nudge bubble. */
export default function FloatingAIChatButton({
  open,
  isStreaming,
  onToggle,
}: {
  open: boolean;
  isStreaming: boolean;
  onToggle: () => void;
}) {
  const { t } = useLanguage();

  return (
    <div className="fixed bottom-5 right-5 z-[70] flex flex-col items-end gap-2.5">
      {!open && !isStreaming && (
        <div className="animate-[aiFloat_3s_ease-in-out_infinite] rounded-2xl rounded-br-sm border border-line bg-panel px-4 py-2.5 text-xs text-muted shadow-soft">
          {t('ai.nudge')}
        </div>
      )}
      <button
        type="button"
        title={t('ai.launcherTitle')}
        aria-label={t('ai.launcherTitle')}
        onClick={onToggle}
        className={`flex h-[60px] w-[60px] cursor-pointer items-center justify-center rounded-full bg-gradient-to-br from-[#26231f] via-[#3b3530] to-[#5f9189] text-[#f8f4ed] shadow-button transition hover:-translate-y-0.5 ${
          isStreaming ? 'animate-[aiPulse_1.5s_ease-in-out_infinite]' : ''
        }`}
      >
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M9.9 3.6 11.4 8a2 2 0 0 0 1.3 1.3l4.4 1.5a1 1 0 0 1 0 1.9l-4.4 1.5a2 2 0 0 0-1.3 1.3L9.9 19a1 1 0 0 1-1.9 0l-1.5-4.4a2 2 0 0 0-1.3-1.3L0.8 11.7a1 1 0 0 1 0-1.9l4.4-1.5a2 2 0 0 0 1.3-1.3L8 3.6a1 1 0 0 1 1.9 0Z" transform="translate(3.5 0.7)" />
        </svg>
      </button>
    </div>
  );
}
