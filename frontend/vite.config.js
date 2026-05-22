import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

function manualChunks(id) {
    var normalizedId = id.replace(/\\/g, '/');
    if (!normalizedId.includes('/node_modules/'))
        return undefined;
    if (normalizedId.includes('/node_modules/react/') ||
        normalizedId.includes('/node_modules/react-dom/') ||
        normalizedId.includes('/node_modules/scheduler/')) {
        return 'react-vendor';
    }
    if (normalizedId.includes('/node_modules/@xyflow/')) {
        return 'reactflow-vendor';
    }
    if (normalizedId.includes('/node_modules/d3-')) {
        return 'd3-vendor';
    }
    return 'vendor';
}

export default defineConfig(function (_a) {
    var mode = _a.mode;
    var env = loadEnv(mode, process.cwd(), '');
    var apiTarget = env.VITE_API_URL || 'http://backend:8080';
    var wsTarget = apiTarget.replace(/^http/i, 'ws');
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
                    xfwd: true,
                    ws: true,
                },
                '/api': {
                    target: apiTarget,
                    changeOrigin: true,
                    xfwd: true,
                    ws: true,
                },
            },
        },
        build: {
            rollupOptions: {
                output: {
                    manualChunks: manualChunks,
                },
            },
        },
    };
});
