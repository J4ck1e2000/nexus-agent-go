import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// Renderer config. The Forge Vite plugin merges its own preset on top of this
// (base './', outDir `.vite/renderer/main_window`), so don't set those here.
export default defineConfig({
  plugins: [react(), tailwindcss()],
});
