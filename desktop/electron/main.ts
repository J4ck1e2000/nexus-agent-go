import { app, BrowserWindow } from 'electron';
import path from 'node:path';

function createMainWindow(): BrowserWindow {
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
  createMainWindow();

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createMainWindow();
    }
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});
