import type { ForgeConfig } from '@electron-forge/shared-types';
import { MakerSquirrel } from '@electron-forge/maker-squirrel';
import { MakerZIP } from '@electron-forge/maker-zip';
import { MakerDMG } from '@electron-forge/maker-dmg';
import { MakerDeb } from '@electron-forge/maker-deb';
import { MakerRpm } from '@electron-forge/maker-rpm';
import { VitePlugin } from '@electron-forge/plugin-vite';

const config: ForgeConfig = {
  packagerConfig: {
    name: 'Nexus Desktop',
    executableName: 'nexus-desktop',
    asar: { unpack: '**/node_modules/node-pty/**' },
    // Forge's Vite plugin normally ships only .vite. Include the one external
    // runtime package whose native binaries and worker scripts cannot bundle.
    ignore: (file) => Boolean(file) && !(
      file === '/package.json' || file.startsWith('/.vite')
      || file === '/node_modules' || file === '/node_modules/node-pty'
      || file.startsWith('/node_modules/node-pty/')
    ),
  },
  // node-pty 1.1 ships Node-API prebuilds; the loopback smoke test verifies them
  // against our Electron version. Preserve those binaries instead of invoking
  // node-gyp inside the pruned package (where build-only headers are absent).
  rebuildConfig: { ignoreModules: ['node-pty'] },
  makers: [
    new MakerZIP({}, ['darwin', 'linux', 'win32']),
    new MakerDMG({}, ['darwin']),
    new MakerSquirrel({}, ['win32']),
    new MakerDeb(
      {
        options: {
          maintainer: 'Nexus <nexus@example.com>',
        },
      },
      ['linux'],
    ),
    new MakerRpm({}, ['linux']),
  ],
  plugins: [
    new VitePlugin({
      hotRestart: true,
      build: [
        {
          entry: 'electron/main.ts',
          config: 'vite.main.config.ts',
          target: 'main',
        },
        {
          entry: 'electron/preload.ts',
          config: 'vite.preload.config.ts',
          target: 'preload',
        },
      ],
      renderer: [
        {
          name: 'main_window',
          config: 'vite.renderer.config.ts',
        },
      ],
    }),
  ],
};

export default config;
