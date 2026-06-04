import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../../pve/www',
    emptyOutDir: true,
    assetsDir: 'assets'
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    proxy: {
      '/api2': 'http://127.0.0.1:18008'
    }
  }
});
