/// <reference types="vite/client" />
import type {
  CanvasDiagnosticEvent,
  CanvasDiagnosticsExport,
  CanvasDiagnosticsSnapshot,
} from './components/canvas/canvasDiagnostics';
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
    __THEIA_CANVAS_DIAGNOSTICS__?: () => CanvasDiagnosticsSnapshot;
    __THEIA_CANVAS_DIAGNOSTICS_EXPORT__?: () => CanvasDiagnosticsExport;
    __THEIA_CANVAS_DIAGNOSTICS_CLEAR_EVENTS__?: () => void;
    __THEIA_CANVAS_DIAGNOSTIC_EVENTS__?: CanvasDiagnosticEvent[];
    __THEIA_CANVAS_FORCE_REFRESH__?: () => void;
  }
}

export {};
