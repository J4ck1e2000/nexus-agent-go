import { useEffect, useRef, useState } from 'react';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import type { EnrichedNode } from '../../lib/node-logic';
import type { TerminalExitEvent, TerminalOutputEvent, TerminalSessionInfo } from '../../../electron/types/ipc';
import { useLanguage } from '../../hooks/useLanguage';
import AppModal from '../common/AppModal';
import { loadLocalTerminalProfile, saveLocalTerminalProfile } from '../../lib/terminal-profile';
import { splitTerminalInput } from '../../lib/terminal-input';

export default function SshTerminalDialog({ node, userId, onClose }: { node: EnrichedNode; userId: number; onClose: () => void }) {
  const { t } = useLanguage();
  const savedProfile = loadLocalTerminalProfile(userId, node.id);
  const [targetHost, setTargetHost] = useState(savedProfile?.targetHost ?? node.sshHost ?? '');
  const [sshUser, setSshUser] = useState(savedProfile?.sshUser ?? '');
  const [useSshConfig, setUseSshConfig] = useState(savedProfile?.useSshConfig ?? false);
  const [session, setSession] = useState<TerminalSessionInfo | null>(null);
  const [connectionEnded, setConnectionEnded] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const terminalContainer = useRef<HTMLDivElement | null>(null);
  const terminal = useRef<Terminal | null>(null);
  const sessionRef = useRef<string | null>(null);
  const connectedRef = useRef(false);
  const mountedRef = useRef(true);

  useEffect(() => {
    saveLocalTerminalProfile(userId, node.id, { targetHost, sshUser, useSshConfig });
  }, [node.id, sshUser, targetHost, useSshConfig, userId]);

  useEffect(() => {
    const outputListener = window.nexus.terminal.onOutput((event: TerminalOutputEvent) => {
      if (event.sessionId !== sessionRef.current) return;
      terminal.current?.write(event.data);
    });
    const exitListener = window.nexus.terminal.onExit((event: TerminalExitEvent) => {
      if (event.sessionId !== sessionRef.current) return;
      connectedRef.current = false;
      setConnectionEnded(true);
      setConnecting(false);
      terminal.current?.write(`\r\n\x1b[90m${t('terminal.exited', { code: event.exitCode ?? '—' })}\x1b[0m\r\n`);
    });
    return () => {
      outputListener();
      exitListener();
    };
  }, [t]);

  useEffect(() => {
    if (!session || !terminalContainer.current) return;
    const instance = new Terminal({
      cursorBlink: true,
      fontFamily: 'Consolas, "Cascadia Mono", "SFMono-Regular", Menlo, monospace',
      fontSize: 13,
      lineHeight: 1.25,
      scrollback: 5000,
      theme: { background: '#101114', foreground: '#e7e8ea', cursor: '#79aaff', selectionBackground: '#45658a88' },
    });
    const fit = new FitAddon();
    instance.loadAddon(fit);
    instance.open(terminalContainer.current);
    terminal.current = instance;
    fit.fit();

    let disposed = false;
    let inputQueue = Promise.resolve();
    let fitFrame: number | null = null;
    const sessionError = () => { if (!disposed) setError(t('terminal.sessionFailed')); };
    const input = instance.onData((data) => {
      inputQueue = inputQueue.then(async () => {
        for (const chunk of splitTerminalInput(data)) {
          if (disposed || sessionRef.current !== session.sessionId || !connectedRef.current) return;
          const result = await window.nexus.terminal.write(session.sessionId, chunk);
          if (!result.ok) { sessionError(); return; }
        }
      }).catch(sessionError);
    });
    const fitAndResize = () => {
      if (fitFrame !== null) cancelAnimationFrame(fitFrame);
      fitFrame = requestAnimationFrame(() => {
        fitFrame = null;
        if (disposed) return;
        fit.fit();
        if (connectedRef.current) void window.nexus.terminal.resize(session.sessionId, instance.cols, instance.rows)
          .then((result) => { if (!result.ok) sessionError(); }).catch(sessionError);
      });
    };
    const resize = new ResizeObserver(fitAndResize);
    resize.observe(terminalContainer.current);
    instance.focus();
    void window.nexus.terminal.attach(session.sessionId).then((result) => {
      if (disposed) return;
      if (result.ok) fitAndResize();
      else {
        sessionError();
        connectedRef.current = false;
        setConnectionEnded(true);
        void window.nexus.terminal.close(session.sessionId);
      }
    }).catch(sessionError);
    void document.fonts?.ready.then(() => { if (!disposed) fitAndResize(); });
    return () => {
      disposed = true;
      if (fitFrame !== null) cancelAnimationFrame(fitFrame);
      input.dispose();
      resize.disconnect();
      instance.dispose();
      terminal.current = null;
    };
  }, [session]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      const activeSession = sessionRef.current;
      if (activeSession) void window.nexus.terminal.close(activeSession);
      sessionRef.current = null;
      connectedRef.current = false;
    };
  }, []);

  async function connect() {
    if (!node.sshHost || !node.sshPort) {
      setError(t('terminal.notConfigured'));
      return;
    }
    setConnecting(true);
    setError(null);
    const result = await window.nexus.terminal.start({
      nodeId: node.id,
      targetHost: targetHost.trim(),
      sshUser: sshUser.trim(),
      useSshConfig,
      cols: 100,
      rows: 30,
    }).catch(() => ({ ok: false as const, error: { code: 'gateway_error' as const, detail: 'terminal_ssh_start_failed', message: '' } }));
    if (!mountedRef.current) {
      if (result.ok) void window.nexus.terminal.close(result.data.sessionId);
      return;
    }
    setConnecting(false);
    if (!result.ok) {
      const normalizedError = result.error.message.toLowerCase();
      const message = result.error.detail === 'terminal_ssh_missing' || normalizedError.includes('openssh was not found') || (normalizedError.includes('spawn') && normalizedError.includes('enoent'))
        ? t('terminal.sshMissing')
        : result.error.detail === 'terminal_alias_mismatch' || normalizedError.includes('not this node')
          ? t('terminal.aliasMismatch')
          : result.error.detail === 'terminal_ssh_config_failed' || normalizedError.includes('read local openssh config')
            ? `${t('terminal.configFailed')} ${result.error.message}`
            : `${t('terminal.startFailed')} ${result.error.message}`;
      setError(message);
      return;
    }
    sessionRef.current = result.data.sessionId;
    connectedRef.current = true;
    setConnectionEnded(false);
    if (result.data.sshUser) setSshUser(result.data.sshUser);
    setSession(result.data);
  }

  function closeSession() {
    const activeSession = sessionRef.current;
    sessionRef.current = null;
    connectedRef.current = false;
    setSession(null);
    setError(null);
    setConnectionEnded(false);
    setConnecting(false);
    if (activeSession) void window.nexus.terminal.close(activeSession);
  }

  function closeDialog() {
    closeSession();
    onClose();
  }

  return (
    <AppModal open onClose={closeDialog} disableClose={connecting} title={t('terminal.title')} subtitle={`${node.name} · ${t('terminal.subtitle')}`} maxWidth="max-w-5xl">
      <>
      {!session ? (
        <div className="flex flex-col gap-4">
          <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('terminal.targetHost')}
            <input className="input-field font-mono" value={targetHost} onChange={(event) => setTargetHost(event.target.value)} maxLength={255} disabled={connecting} />
          </label>
          {!useSshConfig && <label className="flex flex-col gap-1.5 text-xs text-muted">
            {t('terminal.username')}
            <input className="input-field font-mono" value={sshUser} onChange={(event) => setSshUser(event.target.value)} maxLength={64} disabled={connecting} autoComplete="off" />
          </label>}
          <label className="flex items-center gap-2 text-xs text-muted">
            <input type="checkbox" checked={useSshConfig} onChange={(event) => setUseSshConfig(event.target.checked)} disabled={connecting} />
            {t('terminal.useSshConfig')}
          </label>
          <p className="text-xs text-muted">{useSshConfig ? t('terminal.useSshConfigHint') : t('terminal.directHint')}</p>
          <div className="flex justify-end gap-2">
            <button type="button" className="muted-button" onClick={closeDialog} disabled={connecting}>{t('action.cancel')}</button>
            <button type="button" className="primary-button" onClick={() => void connect()} disabled={connecting || !targetHost.trim() || (!useSshConfig && !sshUser.trim())}>
              {connecting ? t('terminal.connecting') : t('terminal.connect')}
            </button>
          </div>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <div className="flex items-center justify-between gap-3 text-xs text-muted">
            <span className="truncate font-mono">{session.sshUser || 'OpenSSH config'}@{session.targetHost}</span>
            <button type="button" className="muted-button" onClick={closeSession}>{t(connectionEnded ? 'terminal.reconnect' : 'terminal.disconnect')}</button>
          </div>
          <div ref={terminalContainer} className="h-[58vh] min-h-[320px] overflow-hidden rounded-xl border border-[#24262a] bg-[#101114]" />
          <p className="text-[11px] text-muted">{t('terminal.inputHint')}</p>
        </div>
      )}
      {error && <p className="mt-3 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800" role="alert">{error}</p>}
      </>
    </AppModal>
  );
}
