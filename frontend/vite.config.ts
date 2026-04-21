import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiTarget = env.VITE_API_URL || 'http://backend:8080';
  const wsTarget = apiTarget.replace(/^http/i, 'ws');

  return {
    plugins: [react(), tailwindcss()],
    define: {
      __APP_VERSION__: JSON.stringify(process.env.VERSION || 'dev'),
    },
    server: {
      host: '0.0.0.0',
      port: 3000,
      proxy: {
        '/api/v1/ws': {
          target: wsTarget,
          changeOrigin: true,
          ws: true,
        },
        '/api': {
          target: apiTarget,
          changeOrigin: true,
          ws: true,
        },
      },
    },
  };
});
