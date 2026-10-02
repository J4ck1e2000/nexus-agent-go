import { useState, type FormEvent } from 'react';
import type { AgentConfig } from '../../../electron/types/ipc';
import AppModal from '../common/AppModal';
import { useLanguage } from '../../hooks/useLanguage';
import { normalizeAgentUrl } from '../../lib/node-logic';
import { useToastContext } from '../../context/ToastContext';

/** Admin "Add Node" dialog: name + agent URL with duplicate detection. */
export default function AddNodeDialog({
  open,
  configs,
  busy,
  onClose,
  onCreate,
}: {
  open: boolean;
  configs: AgentConfig[];
  busy: boolean;
  onClose: () => void;
  onCreate: (payload: { name: string; url: string }) => Promise<boolean>;
}) {
  const { t } = useLanguage();
  const { showToast } = useToastContext();
  const [name, setName] = useState('');
  const [url, setUrl] = useState('http://');
  const [error, setError] = useState<string | null>(null);

  const reset = (): void => {
    setName('');
    setUrl('http://');
    setError(null);
  };

  const submit = async (event?: FormEvent<HTMLFormElement>): Promise<void> => {
    event?.preventDefault();
    setError(null);

    const trimmedName = name.trim();
    const trimmedUrl = url.trim();
    if (!trimmedName) {
      setError(t('dialog.usernameRequired'));
      return;
    }
    if (!trimmedUrl) {
      setError(t('dialog.passwordRequired'));
      return;
    }
    const normalized = normalizeAgentUrl(trimmedUrl);
    if (!normalized) {
      setError(t('desktop.settings.invalidUrl'));
      return;
    }
    const existing = new Set(
      configs.map((config) => normalizeAgentUrl(config.url)).filter((value): value is string => value !== null),
    );
    if (existing.has(normalized)) {
      showToast(t('notify.duplicateNodeUrl'), 'warning');
      setError(t('notify.duplicateNodeUrl'));
      return;
    }

    const ok = await onCreate({ name: trimmedName, url: trimmedUrl });
    if (ok) {
      reset();
    }
  };

  return (
    <AppModal
      open={open}
      onClose={() => {
        reset();
        onClose();
      }}
      title={t('modal.addNode')}
      subtitle={t('modal.configuration')}
      maxWidth="max-w-md"
      disableEscape={busy}
    >
      <form className="flex flex-col gap-3" onSubmit={(event) => void submit(event)}>
        <label className="flex flex-col gap-1.5">
          <span className="text-xs font-medium text-muted">{t('modal.nodeName')}</span>
          <input
            type="text"
            className="input-field"
            value={name}
            placeholder={t('modal.nodeNamePlaceholder')}
            disabled={busy}
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-xs font-medium text-muted">{t('modal.agentUrl')}</span>
          <input
            type="text"
            className="input-field font-mono"
            value={url}
            placeholder={t('modal.agentUrlPlaceholder')}
            disabled={busy}
            onChange={(event) => setUrl(event.target.value)}
          />
        </label>
        {error && <p className="text-xs text-danger">{error}</p>}
        <div className="mt-1 flex justify-end gap-2">
          <button
            type="button"
            className="muted-button"
            disabled={busy}
            onClick={() => {
              reset();
              onClose();
            }}
          >
            {t('action.cancel')}
          </button>
          <button type="submit" className="primary-button" disabled={busy}>
            {busy ? t('action.submitting') : t('action.connectAgent')}
          </button>
        </div>
      </form>
    </AppModal>
  );
}
