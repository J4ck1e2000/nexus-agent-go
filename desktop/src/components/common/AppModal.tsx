import { useEffect, type ReactNode } from 'react';

/**
 * Centered modal with backdrop, Escape-to-close and body scroll lock.
 * `disableClose` blocks every dismissal path (Esc, backdrop, close button) —
 * used while a submit inside the dialog is in flight.
 */
export default function AppModal({
  open,
  onClose,
  title,
  subtitle,
  children,
  maxWidth = 'max-w-lg',
  disableClose = false,
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  subtitle?: ReactNode;
  children: ReactNode;
  maxWidth?: string;
  disableClose?: boolean;
}) {
  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape' && !disableClose) {
        event.preventDefault();
        onClose();
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open, onClose, disableClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-[80] flex items-center justify-center p-4" role="dialog" aria-modal="true">
      <div
        className="absolute inset-0 bg-[#2f2922]/25 backdrop-blur-[2px]"
        onClick={disableClose ? undefined : onClose}
        aria-hidden="true"
      />
      <div className={`soft-panel relative flex max-h-[86vh] w-full ${maxWidth} flex-col p-6`}>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-lg font-semibold tracking-tight text-ink">{title}</h2>
            {subtitle && <p className="mt-1 text-xs text-muted">{subtitle}</p>}
          </div>
          <button
            type="button"
            className={`shrink-0 cursor-pointer rounded-full p-1.5 text-muted transition hover:bg-panel-soft hover:text-ink ${
              disableClose ? 'cursor-not-allowed opacity-50 hover:bg-transparent hover:text-muted' : ''
            }`}
            onClick={disableClose ? undefined : onClose}
            disabled={disableClose}
            aria-label="Close"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M18 6 6 18" />
              <path d="m6 6 12 12" />
            </svg>
          </button>
        </div>
        <div className="custom-scrollbar mt-4 min-h-0 flex-1 overflow-y-auto pr-1">{children}</div>
      </div>
    </div>
  );
}
