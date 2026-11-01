import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// In development the browser talks to this dev server only; /api and /ws are proxied to the Go API,
// so there is no CORS to configure. The API must allow this origin for WebSockets (compose.yaml does).
const api = process.env.API_URL ?? 'http://localhost:8080';

export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    port: 5173,
    proxy: {
      '/api': { target: api, changeOrigin: true },
      '/ws': { target: api, changeOrigin: true, ws: true },
      '/openapi.json': { target: api, changeOrigin: true },
      '/docs': { target: api, changeOrigin: true },
    },
    watch: { usePolling: process.env.CHOKIDAR_USEPOLLING === 'true' },
  },
  test: { environment: 'jsdom', globals: true, setupFiles: ['./src/test/setup.ts'], css: false },
});
