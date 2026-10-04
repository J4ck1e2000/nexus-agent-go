import AppModal from './AppModal';
import { useLanguage } from '../../hooks/useLanguage';

/** Danger-aware confirmation dialog. The confirm button names the action. */
export default function ConfirmDialog({
  open,
  title,
  description,
  onConfirm,
  onCancel,
  busy = false,
  confirmTone = 'default',
  confirmLabel,
}: {
  open: boolean;
  title: string;
  description: string;
  onConfirm: () => void;
  onCancel: () => void;
  busy?: boolean;
  confirmTone?: 'default' | 'danger';
  /** Verb matching the action (e.g. "Delete node"); defaults to a generic confirm. */
  confirmLabel?: string;
}) {
  const { t } = useLanguage();

  return (
    <AppModal open={open} onClose={onCancel} title={title} maxWidth="max-w-md" disableClose={busy}>
      <p className="text-sm leading-relaxed text-muted">{description}</p>
      <div className="mt-5 flex justify-end gap-2">
        <button type="button" className="muted-button" onClick={onCancel} disabled={busy}>
          {t('action.cancel')}
        </button>
        <button
          type="button"
          className={confirmTone === 'danger' ? 'danger-button' : 'primary-button'}
          onClick={onConfirm}
          disabled={busy}
        >
          {busy ? t('action.submitting') : confirmLabel ?? t('action.confirm')}
        </button>
      </div>
    </AppModal>
  );
}
