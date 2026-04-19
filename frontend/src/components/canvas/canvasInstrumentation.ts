export type CanvasMeasurementName =
  | 'theia:canvas:topology-load'
  | 'theia:canvas:layout'
  | 'theia:canvas:snapshot-apply';

export type CanvasMeasurementTrigger =
  | 'initial_load'
  | 'backend_reconnected'
  | 'topology_changed'
  | 'snapshot'
  | 'manual_refresh';

export interface CanvasMeasurementRecord {
  name: CanvasMeasurementName;
  trigger: CanvasMeasurementTrigger;
  durationMs: number;
  recordedAt: string;
}

const maxCanvasMeasurements = 50;

function pushCanvasMeasurement(record: CanvasMeasurementRecord): void {
  if (typeof window === 'undefined') {
    return;
  }

  const nextBuffer = [...(window.__THEIA_CANVAS_METRICS__ ?? []), record];
  if (nextBuffer.length > maxCanvasMeasurements) {
    nextBuffer.splice(0, nextBuffer.length - maxCanvasMeasurements);
  }
  window.__THEIA_CANVAS_METRICS__ = nextBuffer;
}

function safeMark(markName: string): void {
  if (typeof performance === 'undefined' || typeof performance.mark !== 'function') {
    return;
  }
  performance.mark(markName);
}

function safeMeasure(measureName: string, startMark: string, endMark: string): void {
  if (
    typeof performance === 'undefined'
    || typeof performance.measure !== 'function'
    || typeof performance.clearMarks !== 'function'
    || typeof performance.clearMeasures !== 'function'
  ) {
    return;
  }

  performance.measure(measureName, startMark, endMark);
  performance.clearMarks(startMark);
  performance.clearMarks(endMark);
  performance.clearMeasures(measureName);
}

function finalizeMeasurement(
  name: CanvasMeasurementName,
  trigger: CanvasMeasurementTrigger,
  startedAt: number,
  markPrefix: string,
): void {
  const endedAt = typeof performance !== 'undefined' && typeof performance.now === 'function'
    ? performance.now()
    : Date.now();
  const durationMs = Math.max(0, Number((endedAt - startedAt).toFixed(3)));
  const endMark = `${markPrefix}:end`;

  safeMark(endMark);
  safeMeasure(markPrefix, `${markPrefix}:start`, endMark);
  pushCanvasMeasurement({
    name,
    trigger,
    durationMs,
    recordedAt: new Date().toISOString(),
  });
}

export function measureCanvasWork<T>(
  name: CanvasMeasurementName,
  trigger: CanvasMeasurementTrigger,
  work: () => T,
): T {
  const startedAt = typeof performance !== 'undefined' && typeof performance.now === 'function'
    ? performance.now()
    : Date.now();
  const markPrefix = `${name}:${trigger}:${Date.now()}`;
  safeMark(`${markPrefix}:start`);

  try {
    return work();
  } finally {
    finalizeMeasurement(name, trigger, startedAt, markPrefix);
  }
}

export async function measureCanvasAsyncWork<T>(
  name: CanvasMeasurementName,
  trigger: CanvasMeasurementTrigger,
  work: () => Promise<T>,
): Promise<T> {
  const startedAt = typeof performance !== 'undefined' && typeof performance.now === 'function'
    ? performance.now()
    : Date.now();
  const markPrefix = `${name}:${trigger}:${Date.now()}`;
  safeMark(`${markPrefix}:start`);

  try {
    return await work();
  } finally {
    finalizeMeasurement(name, trigger, startedAt, markPrefix);
  }
}
