/// <reference types="vite/client" />
import type { CanvasMeasurementRecord } from './components/canvas/canvasInstrumentation';

declare const __APP_VERSION__: string;

declare global {
  interface Window {
    __THEIA_CANVAS_METRICS__?: CanvasMeasurementRecord[];
  }
}

export {};
