import { useState, type FormEvent } from 'react';
import type { AgentConfig } from '../../../electron/types/ipc';
import type { ResolvedCollectorType } from '../../../electron/lib/node-payload';
import type { AddNodeInput } from '../../hooks/useNodeConfigs';
import AppModal from '../common/AppModal';
import { useLanguage } from '../../hooks/useLanguage';
import { normalizeAgentUrl } from '../../lib/node-logic';
import { useToastContext } from '../../context/ToastContext';

export type NodeCreateOutcome = { ok: true } | { ok: false; message: string };
/** Admin "Add Node" dialog: SSH (agentless, default) or legacy Agent URL. */
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
  onCreate: (payload: AddNodeInput) => Promise<NodeCreateOutcome>;
}) {
  const { t } = useLanguage();
  const { showToast } = useToastContext();
  const [collectorType, setCollectorType] = useState<ResolvedCollectorType>('ssh');
  const [name, setName] = useState('');
  const [url, setUrl] = useState('http://');
  const [sshHost, setSshHost] = useState('');
  const [sshPort, setSshPort] = useState('22');
  const [sshUser, setSshUser] = useState('');
  const [sshPassword, setSshPassword] = useState('');
  const [error, setError] = useState<string | null>(null);

  const reset = (): void => {
    setCollectorType('ssh');
    setName('');
    setUrl('http://');
    setSshHost('');
    setSshPort('22');
    setSshUser('');
    setSshPassword('');
    setError(null);
  };

  const parsedPort = (): number => Number.parseInt(sshPort.trim(), 10);

  const submit = async (event?: FormEvent<HTMLFormElement>): Promise<void> => {
    event?.preventDefault();
    setError(null);

    const trimmedName = name.trim();
    if (!trimmedName) {
      setError(t('dialog.usernameRequired'));
      return;
    }

    if (collectorType === 'agent') {
      const trimmedUrl = url.trim();
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
        configs
          .filter((config) => (config.collector_type ?? 'agent') === 'agent')
          .map((config) => normalizeAgentUrl(config.url ?? ''))
          .filter((value): value is string => value !== null),
      );
      if (existing.has(normalized)) {
        showToast(t('notify.duplicateNodeUrl'), 'warning');
        setError(t('notify.duplicateNodeUrl'));
        return;
      }

      const outcome = await onCreate({ name: trimmedName, collectorType: 'agent', url: trimmedUrl });
      if (outcome.ok) reset();
      else setError(outcome.message);
      return;
    }

    const host = sshHost.trim();
    const user = sshUser.trim();
    const port = parsedPort();
    if (!host) {
      setError(t('desktop.nodes.hostRequired'));
      return;
    }
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      setError(t('desktop.nodes.invalidPort'));
      return;
    }
    if (!user) {
      setError(t('desktop.nodes.userRequired'));
      return;
    }
    const password = sshPassword;
    setSshPassword('');
    const outcome = await onCreate({
      name: trimmedName,
      collectorType: 'ssh',
      sshHost: host,
      sshPort: port,
      sshUser: user,
      sshPassword: password,
    });
    if (outcome.ok) reset();
    else setError(outcome.message);
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
          <span className="text-xs font-medium text-muted">{t('desktop.nodes.collectorLabel')}</span>
          <select
            className="input-field"
            value={collectorType}
            disabled={busy}
            onChange={(event) => {
              const nextType = event.target.value === 'ssh' ? 'ssh' : 'agent';
              if (nextType === 'agent') setSshPassword('');
              setCollectorType(nextType);
              setError(null);
            }}
          >
            <option value="ssh">{t('desktop.nodes.collectorSSH')}</option>
            <option value="agent">{t('desktop.nodes.collectorAgent')}</option>
          </select>
        </label>

        {collectorType === 'agent' ? (
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
        ) : (
          <>
            <p className="text-xs text-muted">{t('desktop.nodes.sshHint')}</p>
            <div className="grid grid-cols-[1fr_96px] gap-3">
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted">{t('desktop.nodes.hostLabel')}</span>
                <input
                  type="text"
                  className="input-field font-mono"
                  value={sshHost}
                  placeholder={t('desktop.nodes.hostPlaceholder')}
                  disabled={busy}
                  onChange={(event) => setSshHost(event.target.value)}
                />
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted">{t('desktop.nodes.portLabel')}</span>
                <input
                  type="number"
                  min={1}
                  max={65535}
                  className="input-field font-mono"
                  value={sshPort}
                  disabled={busy}
                  onChange={(event) => setSshPort(event.target.value)}
                />
              </label>
            </div>
            <label className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-muted">{t('desktop.nodes.userLabel')}</span>
              <input
                type="text"
                className="input-field font-mono"
                value={sshUser}
                placeholder={t('desktop.nodes.userPlaceholder')}
                disabled={busy}
                onChange={(event) => setSshUser(event.target.value)}
              />
            </label>
            <label className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-muted">{t('desktop.nodes.passwordLabel')}</span>
              <input
                type="password"
                className="input-field font-mono"
                value={sshPassword}
                maxLength={4096}
                autoComplete="new-password"
                disabled={busy}
                onChange={(event) => setSshPassword(event.target.value)}
              />
              <span className="text-xs text-muted">{t('desktop.nodes.passwordHint')}</span>
            </label>
          </>
        )}

        {error && <p className="text-xs text-danger">{error}</p>}
        <div className="mt-1 flex items-center justify-end gap-2">
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
            {busy ? t('desktop.nodes.connecting') : collectorType === 'ssh' ? t('desktop.nodes.connectAndAdd') : t('desktop.nodes.addServer')}
          </button>
        </div>
      </form>
    </AppModal>
  );
}
