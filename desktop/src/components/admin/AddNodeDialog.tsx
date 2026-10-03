import { useState, type FormEvent } from 'react';
import type { AgentConfig, TestSSHResult } from '../../../electron/types/ipc';
import type { ResolvedCollectorType } from '../../../electron/lib/node-payload';
import type { AddNodeInput } from '../../hooks/useNodeConfigs';
import AppModal from '../common/AppModal';
import { useLanguage } from '../../hooks/useLanguage';
import { normalizeAgentUrl } from '../../lib/node-logic';
import { useToastContext } from '../../context/ToastContext';

export type SSHTestOutcome =
  | { ok: true; result: TestSSHResult }
  | { ok: false; message: string };

type TestState =
  | { kind: 'idle' }
  | { kind: 'testing' }
  | { kind: 'ok'; result: TestSSHResult }
  | { kind: 'failed'; message: string };

/** Admin "Add Node" dialog: SSH (agentless, default) or legacy Agent URL. */
export default function AddNodeDialog({
  open,
  configs,
  busy,
  onClose,
  onCreate,
  onTestSSH,
}: {
  open: boolean;
  configs: AgentConfig[];
  busy: boolean;
  onClose: () => void;
  onCreate: (payload: AddNodeInput) => Promise<boolean>;
  onTestSSH: (payload: { sshHost: string; sshPort: number; sshUser: string }) => Promise<SSHTestOutcome>;
}) {
  const { t } = useLanguage();
  const { showToast } = useToastContext();
  const [collectorType, setCollectorType] = useState<ResolvedCollectorType>('ssh');
  const [name, setName] = useState('');
  const [url, setUrl] = useState('http://');
  const [sshHost, setSshHost] = useState('');
  const [sshPort, setSshPort] = useState('22');
  const [sshUser, setSshUser] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [testState, setTestState] = useState<TestState>({ kind: 'idle' });

  const reset = (): void => {
    setCollectorType('ssh');
    setName('');
    setUrl('http://');
    setSshHost('');
    setSshPort('22');
    setSshUser('');
    setError(null);
    setTestState({ kind: 'idle' });
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

      const ok = await onCreate({ name: trimmedName, collectorType: 'agent', url: trimmedUrl });
      if (ok) {
        reset();
      }
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
    const endpoint = `${host.toLowerCase()}:${port}:${user}`;
    const duplicate = configs
      .filter((config) => config.collector_type === 'ssh')
      .some(
        (config) =>
          `${(config.ssh_host ?? '').toLowerCase()}:${config.ssh_port ?? 0}:${config.ssh_user ?? ''}` === endpoint,
      );
    if (duplicate) {
      showToast(t('desktop.nodes.duplicateEndpoint'), 'warning');
      setError(t('desktop.nodes.duplicateEndpoint'));
      return;
    }

    const ok = await onCreate({
      name: trimmedName,
      collectorType: 'ssh',
      sshHost: host,
      sshPort: port,
      sshUser: user,
    });
    if (ok) {
      reset();
    }
  };

  const runTest = async (): Promise<void> => {
    setError(null);
    const host = sshHost.trim();
    const user = sshUser.trim();
    const port = parsedPort();
    if (!host || !user || !Number.isInteger(port) || port < 1 || port > 65535) {
      setError(t('desktop.nodes.testInvalidInput'));
      return;
    }

    setTestState({ kind: 'testing' });
    const outcome = await onTestSSH({ sshHost: host, sshPort: port, sshUser: user });
    setTestState(outcome.ok ? { kind: 'ok', result: outcome.result } : { kind: 'failed', message: outcome.message });
  };

  const testButtonDisabled = busy || testState.kind === 'testing';

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
              setCollectorType(event.target.value === 'ssh' ? 'ssh' : 'agent');
              setError(null);
              setTestState({ kind: 'idle' });
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
            {testState.kind === 'testing' && (
              <p className="text-xs text-muted">{t('desktop.nodes.testing')}</p>
            )}
            {testState.kind === 'ok' && (
              <p className="text-xs text-success">
                {t('desktop.nodes.testOk', {
                  hostname: testState.result.hostname,
                  count: testState.result.gpu_count,
                })}
              </p>
            )}
            {testState.kind === 'failed' && (
              <p className="text-xs text-danger">{testState.message}</p>
            )}
          </>
        )}

        {error && <p className="text-xs text-danger">{error}</p>}
        <div className="mt-1 flex items-center justify-end gap-2">
          {collectorType === 'ssh' && (
            <button
              type="button"
              className="muted-button mr-auto"
              disabled={testButtonDisabled}
              onClick={() => void runTest()}
            >
              {testState.kind === 'testing' ? t('desktop.nodes.testing') : t('desktop.nodes.testConnection')}
            </button>
          )}
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
            {busy ? t('action.submitting') : t('desktop.nodes.addServer')}
          </button>
        </div>
      </form>
    </AppModal>
  );
}
