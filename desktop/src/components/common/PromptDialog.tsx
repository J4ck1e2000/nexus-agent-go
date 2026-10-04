import { useEffect, useRef, useState, type FormEvent } from 'react';
import AppModal from './AppModal';
import { useLanguage } from '../../hooks/useLanguage';

/** Single-value prompt dialog with validation (used for password resets). */
export default function PromptDialog({
  open,
  title,
  description,
  placeholder,
  inputType = 'text',
  maxLength,
  minLength,
  required = false,
  confirmLabel,
  busy = false,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  title: string;
  description: string;
  placeholder?: string;
  inputType?: 'text' | 'password';
  maxLength?: number;
  minLength?: number;
  required?: boolean;
  confirmLabel: string;
  busy?: boolean;
  onSubmit: (value: string) => void;
  onCancel: () => void;
}) {
  const { t } = useLanguage();
  const [value, setValue] = useState('');
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (open) {
      setValue('');
      setError(null);
      const timer = setTimeout(() => inputRef.current?.focus(), 30);
      return () => clearTimeout(timer);
    }
    return undefined;
  }, [open]);

  const submit = (event?: FormEvent<HTMLFormElement>): void => {
    event?.preventDefault();
    const trimmed = value;
    if (required && !trimmed) {
      setError(t('dialog.requiredField'));
      return;
    }
    if (minLength !== undefined && trimmed.length > 0 && trimmed.length < minLength) {
      setError(t('dialog.passwordTooShort', { min: minLength }));
      return;
    }
    onSubmit(trimmed);
  };

  return (
    <AppModal open={open} onClose={onCancel} title={title} maxWidth="max-w-md" disableClose={busy}>
      <p className="text-sm leading-relaxed text-muted">{description}</p>
      <form
        className="mt-4 flex flex-col gap-3"
        onSubmit={(event) => submit(event)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && !busy) submit();
        }}
      >
        <input
          ref={inputRef}
          type={inputType}
          className="input-field"
          value={value}
          placeholder={placeholder}
          maxLength={maxLength}
          onChange={(event) => {
            setValue(event.target.value);
            setError(null);
          }}
        />
        {error && <p className="text-xs text-danger">{error}</p>}
        <div className="flex justify-end gap-2">
          <button type="button" className="muted-button" onClick={onCancel} disabled={busy}>
            {t('action.cancel')}
          </button>
          <button type="submit" className="primary-button" disabled={busy}>
            {busy ? t('action.submitting') : confirmLabel}
          </button>
        </div>
      </form>
    </AppModal>
  );
}
