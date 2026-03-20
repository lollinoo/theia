import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
export default defineConfig(function (_a) {
    var mode = _a.mode;
    var env = loadEnv(mode, process.cwd(), '');
    var apiTarget = env.VITE_API_URL || 'http://backend:8080';
    var wsTarget = apiTarget.replace(/^http/i, 'ws');
    return {
        plugins: [react()],
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
