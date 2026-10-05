import { defineConfig } from 'vite';

// The Electron Forge Vite plugin provides the main-process build preset
// (CJS output to `.vite/build/main.cjs`, node builtins + electron externalized).
// node-pty must retain its package directory: native modules and ConPTY worker
// scripts are resolved relative to that directory in both dev and ASAR builds.
export default defineConfig({
  build: { rollupOptions: { external: ['node-pty'] } },
});
