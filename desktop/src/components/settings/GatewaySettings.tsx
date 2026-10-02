import { useEffect, useState } from 'react';
import { useToastContext } from '../../context/ToastContext';
import { localizedError } from '../../lib/errors';
import { useLanguage } from '../../hooks/useLanguage';

/**
 * Gateway connection settings, shared by the login page (compact panel) and
 * the dedicated settings page. All IO goes through window.nexus.settings.
 */
export default function GatewaySettings({ onSaved }: { onSaved?: () => void }) {
  const { t } = useLanguage();
  const { showToast } = useToastContext();
  const [url, setUrl] = useState('');
  const [loaded, setLoaded] = useState(false);
  const [testing, setTesting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [status, setStatus] = useState<{ tone: 'success' | 'error'; message: string } | null>(null);

  useEffect(() => {
    let disposed = false;
    void window.nexus.settings.get().then((result) => {
      if (disposed) return;
      if (result.ok) {
        setUrl(result.data.gatewayUrl);
      }
      setLoaded(true);
    });
    return () => {
      disposed = true;
    };
  }, []);

  const isValidUrl = (value: string): boolean => {
    try {
      const parsed = new URL(value.trim());
      return parsed.protocol === 'http:' || parsed.protocol === 'https:';
    } catch {
      return false;
    }
  };

  const handleTest = async (): Promise<void> => {
    if (!isValidUrl(url)) {
      setStatus({ tone: 'error', message: t('desktop.settings.invalidUrl') });
      return;
    }
    setTesting(true);
    setStatus(null);
    try {
      const result = await window.nexus.settings.testConnection(url);
      if (result.ok) {
        setStatus({
          tone: 'success',
          message: t('desktop.settings.testOk', { versionName: result.data.versionName || '—' }),
        });
      } else {
        setStatus({
          tone: 'error',
          message: `${t('desktop.settings.testFail')} — ${localizedError(result.error, t)}`,
        });
      }
    } finally {
      setTesting(false);
    }
  };

  const handleSave = async (): Promise<void> => {
    if (!isValidUrl(url)) {
      setStatus({ tone: 'error', message: t('desktop.settings.invalidUrl') });
      return;
    }
    setSaving(true);
    try {
      const result = await window.nexus.settings.update({ gatewayUrl: url.trim() });
      if (result.ok) {
        showToast(t('desktop.settings.saved'), 'success');
        setStatus(null);
        onSaved?.();
      } else {
        setStatus({ tone: 'error', message: localizedError(result.error, t) });
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="soft-panel-subtle p-5">
      <h3 className="text-sm font-semibold text-ink">{t('desktop.settings.gatewaySection')}</h3>
      <p className="mt-1 text-xs text-muted">{t('desktop.settings.gatewayUrlHint')}</p>

      <div className="mt-3 flex flex-col gap-3 sm:flex-row">
        <input
          type="text"
          className="input-field font-mono"
          value={url}
          spellCheck={false}
          disabled={!loaded || testing || saving}
          placeholder={t('desktop.settings.gatewayUrlPlaceholder')}
          onChange={(event) => setUrl(event.target.value)}
          aria-label={t('desktop.settings.gatewayUrlLabel')}
        />
        <div className="flex shrink-0 gap-2">
          <button type="button" className="muted-button" onClick={() => void handleTest()} disabled={testing || saving}>
            {testing ? t('desktop.settings.testing') : t('desktop.settings.testConnection')}
          </button>
          <button type="button" className="primary-button" onClick={() => void handleSave()} disabled={testing || saving}>
            {t('desktop.settings.save')}
          </button>
        </div>
      </div>

      {status && (
        <p
          className={`mt-3 text-xs leading-relaxed ${
            status.tone === 'success' ? 'text-success' : 'text-danger'
          }`}
          role="status"
        >
          {status.message}
        </p>
      )}
    </div>
  );
}
