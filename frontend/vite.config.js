import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

function chunkGroupName(id) {
    var normalizedId = id.replace(/\\/g, '/');
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
    if (normalizedId.includes('/node_modules/')) {
        return 'vendor';
    }
    if (normalizedId.includes('/src/components/canvas/')) {
        return 'canvas-core';
    }
    if (normalizedId.includes('/src/components/Canvas.tsx') ||
        normalizedId.includes('/src/components/DeviceCard.tsx') ||
        normalizedId.includes('/src/components/LinkEdge.tsx') ||
        normalizedId.includes('/src/components/LinkLabelLayer.tsx')) {
        return 'canvas-view';
    }
    if (normalizedId.includes('/src/components/dashboard/') ||
        normalizedId.includes('/src/components/Dashboard.tsx')) {
        return 'dashboard-view';
    }
    if (normalizedId.includes('/src/components/settings/') ||
        normalizedId.includes('/src/components/settings-panel/') ||
        normalizedId.includes('/src/components/SettingsPanel.tsx') ||
        normalizedId.includes('/src/components/UserSettingsPage.tsx') ||
        normalizedId.includes('/src/components/SNMPProfileManager.tsx') ||
        normalizedId.includes('/src/components/CredentialProfileManager.tsx') ||
        normalizedId.includes('/src/components/GrafanaDashboardProfileManager.tsx') ||
        normalizedId.includes('/src/components/InstanceBackupManager.tsx')) {
        return 'settings-view';
    }
    if (normalizedId.includes('/src/components/topology-hub/')) {
        return 'topology-hub-view';
    }
    return null;
}

export default defineConfig(function (_a) {
    var mode = _a.mode;
    var env = loadEnv(mode, process.cwd(), '');
    var apiTarget = env.VITE_API_URL || 'http://backend:8080';
    var wsTarget = apiTarget.replace(/^http/i, 'ws');
    return {
        plugins: [react(), tailwindcss()],
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
            rolldownOptions: {
                output: {
                    codeSplitting: {
                        groups: [
                            {
                                name: chunkGroupName,
                            },
                        ],
                    },
                },
            },
        },
    };
});
