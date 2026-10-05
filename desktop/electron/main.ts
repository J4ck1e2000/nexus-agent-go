import { BrowserWindow, app, safeStorage } from 'electron';
import path from 'node:path';

import { registerIpcHandlers } from './ipc';
import { normalizeGatewayUrl } from './lib/validate';
import { DEFAULT_GATEWAY_URL, SettingsStore } from './services/settings-store';
import { GatewayClient } from './services/gateway';
import { TokenStore, type TokenCipher } from './services/token-store';
import { LocalSshTerminalManager } from './services/local-ssh-terminal';

function createSafeStorageCipher(): TokenCipher {
  return {
    isEncryptionAvailable: () => safeStorage.isEncryptionAvailable(),
    encryptString: (plainText) => safeStorage.encryptString(plainText),
    decryptString: (encrypted) => safeStorage.decryptString(encrypted),
  };
}

function createMainWindow(terminals: LocalSshTerminalManager): BrowserWindow {
  const win = new BrowserWindow({
    width: 1440,
    height: 900,
    minWidth: 1100,
    minHeight: 700,
    title: 'Nexus Desktop',
    show: false,
    backgroundColor: '#ece9e3',
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  win.once('ready-to-show', () => {
    win.show();
  });
  win.on('closed', () => terminals.closeAll());

  // The Forge Vite plugin injects MAIN_WINDOW_VITE_DEV_SERVER_URL in dev mode.
  const devServerURL =
    typeof MAIN_WINDOW_VITE_DEV_SERVER_URL === 'undefined'
      ? undefined
      : MAIN_WINDOW_VITE_DEV_SERVER_URL;
  if (devServerURL) {
    void win.loadURL(devServerURL);
  } else {
    void win.loadFile(path.join(__dirname, `../renderer/${MAIN_WINDOW_VITE_NAME}/index.html`));
  }

  return win;
}

app.whenReady().then(() => {
  const userDataDir = app.getPath('userData');
  const settings = new SettingsStore(path.join(userDataDir, 'settings.json'));
  const tokens = new TokenStore({ dir: userDataDir, cipher: createSafeStorageCipher() });

  const notify = (channel: string, payload: unknown): void => {
    for (const win of BrowserWindow.getAllWindows()) {
      win.webContents.send(channel, payload);
    }
  };

  const gateway = new GatewayClient(
    () => normalizeGatewayUrl(settings.load().gatewayUrl) ?? DEFAULT_GATEWAY_URL,
    () => tokens.load(),
    { onUnauthorized: () => notify('auth:expired', null) },
  );

  const terminals = new LocalSshTerminalManager(notify);

  const disposeIpc = registerIpcHandlers({ gateway, tokens, settings, notify, terminals });

  createMainWindow(terminals);

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createMainWindow(terminals);
    }
  });

  app.on('before-quit', () => {
    disposeIpc();
    terminals.closeAll();
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});
