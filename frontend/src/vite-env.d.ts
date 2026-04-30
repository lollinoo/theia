/// <reference types="vite/client" />
import type {
  CanvasMetricSample,
  CanvasMetricsExport,
} from './components/canvas/canvasInstrumentation';

declare const __APP_VERSION__: string;

declare global {
  interface Window {
    __THEIA_CANVAS_METRICS__?: CanvasMetricSample[];
    __THEIA_CANVAS_METRICS_EXPORT__?: () => CanvasMetricsExport;
    __THEIA_CANVAS_METRICS_CLEAR__?: () => void;
  }
}

export {};
