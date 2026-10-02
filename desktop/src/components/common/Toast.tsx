import type { ToastItem, ToastTone } from '../../context/ToastContext';
import { useToastContext } from '../../context/ToastContext';

const TONE_STYLES: Record<ToastTone, string> = {
  info: 'bg-[#f3eee6] border-[#d8cfc4] text-[#3c3833]',
  success: 'bg-[#e4efe9] border-[#c8ddd5] text-[#255049]',
  warning: 'bg-[#f1e7df] border-[#dfd0c5] text-[#5f4a42]',
  error: 'bg-[#f2e3df] border-[#dfc8c3] text-[#683833]',
};

export default function ToastStack() {
  const { toasts, dismissToast } = useToastContext();

  if (toasts.length === 0) return null;

  return (
    <div className="fixed top-4 right-4 z-[100] flex w-[min(92vw,360px)] flex-col gap-2">
      {toasts.map((toast: ToastItem) => (
        <div
          key={toast.id}
          role="status"
          className={`flex items-start justify-between gap-3 rounded-2xl border px-4 py-3 text-sm shadow-soft ${TONE_STYLES[toast.tone]}`}
        >
          <span className="leading-snug">{toast.message}</span>
          <button
            type="button"
            onClick={() => dismissToast(toast.id)}
            aria-label="Dismiss"
            className="mt-0.5 shrink-0 cursor-pointer opacity-60 transition hover:opacity-100"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M18 6 6 18" />
              <path d="m6 6 12 12" />
            </svg>
          </button>
        </div>
      ))}
    </div>
  );
}
