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
    asar: true,
  },
  rebuildConfig: {},
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
