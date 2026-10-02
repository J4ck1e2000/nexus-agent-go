import AppModal from './AppModal';
import { useLanguage } from '../../hooks/useLanguage';

/** Danger-aware confirmation dialog. */
export default function ConfirmDialog({
  open,
  title,
  description,
  onConfirm,
  onCancel,
  busy = false,
  confirmTone = 'default',
}: {
  open: boolean;
  title: string;
  description: string;
  onConfirm: () => void;
  onCancel: () => void;
  busy?: boolean;
  confirmTone?: 'default' | 'danger';
}) {
  const { t } = useLanguage();

  return (
    <AppModal open={open} onClose={onCancel} title={title} maxWidth="max-w-md" disableEscape={busy}>
      <p className="text-sm leading-relaxed text-muted">{description}</p>
      <div className="mt-5 flex justify-end gap-2">
        <button type="button" className="muted-button" onClick={onCancel} disabled={busy}>
          {t('action.cancel')}
        </button>
        <button
          type="button"
          className={confirmTone === 'danger' ? 'outlined-danger-button' : 'primary-button'}
          onClick={onConfirm}
          disabled={busy}
        >
          {busy ? t('action.submitting') : t('action.confirm')}
        </button>
      </div>
    </AppModal>
  );
}
