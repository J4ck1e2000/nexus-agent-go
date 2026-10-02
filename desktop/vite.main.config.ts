import { defineConfig } from 'vite';

// The Electron Forge Vite plugin provides the main-process build preset
// (CJS output to `.vite/build/main.cjs`, node builtins + electron externalized).
// Keep this file minimal so packaging conventions stay owned by the plugin.
export default defineConfig({});
