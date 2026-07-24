/**
 * Exercises realtime browser workflow behavior so refactors preserve the documented contract.
 */
import { expect, test, type Page } from '@playwright/test';

type CanvasMapSummary = {
  id: string;
  is_default?: boolean;
};

type CanvasBootstrapPayload = {
  topology_version?: string;
  runtime_stream_id?: string;
  runtime_version?: number;
  runtime_identity?: string;
  devices?: Array<{
    hostname?: string;
    ip?: string;
    sys_name?: string;
  }>;
};

type BootstrapCapture = {
  url: string;
  payload: CanvasBootstrapPayload;
};

type TopologyRevalidationCapture = {
  status: number;
  ifNoneMatch?: string;
};

type HelloPayload = {
  runtime_protocol?: number;
  runtime_stream_id?: string;
  runtime_version?: number;
  runtime_identity?: string;
};

type RuntimeCursor = {
  streamId: string;
  version: number;
};

type RealtimeMessage = {
  type: string;
  payload: Record<string, unknown>;
};

type RealtimeFrameEvent = {
  direction: 'sent' | 'received';
  payload: string;
};

type WebSocketCapture = {
  url: string;
  sent: string[];
  received: string[];
  events: RealtimeFrameEvent[];
  closed: boolean;
};

async function expectTopologyVisible(page: Page): Promise<void> {
  await expect(page.getByTestId('topology-canvas')).toBeVisible();
  await expect(page.getByTestId('topology-canvas').getByText('127.0.10.21').first()).toBeVisible();
}

async function waitForTopology(page: Page): Promise<void> {
  await page.goto('/');
  await expectTopologyVisible(page);
}

async function primaryMapId(page: Page): Promise<string> {
  const response = await page.request.get('/api/v1/canvas/maps');
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as { data?: CanvasMapSummary[] };
  const primaryMap = (payload.data ?? []).find((map) => map.is_default === true);
  expect(primaryMap?.id).toBeTruthy();
  return primaryMap!.id;
}

function captureSavedMapBootstraps(page: Page, mapId: string): BootstrapCapture[] {
  const captures: BootstrapCapture[] = [];
  const bootstrapPath = `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/bootstrap`;
  page.on('response', async (response) => {
    const url = new URL(response.url());
    if (response.request().method() !== 'GET' || url.pathname !== bootstrapPath) {
      return;
    }
    captures.push({
      url: response.url(),
      payload: (await response.json()) as CanvasBootstrapPayload,
    });
  });
  return captures;
}

function captureSavedMapTopologyRevalidations(
  page: Page,
  mapId: string,
): TopologyRevalidationCapture[] {
  const captures: TopologyRevalidationCapture[] = [];
  const topologyPath = `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/topology`;
  page.on('response', (response) => {
    const request = response.request();
    if (request.method() !== 'GET' || new URL(response.url()).pathname !== topologyPath) {
      return;
    }
    captures.push({
      status: response.status(),
      ifNoneMatch: request.headers()['if-none-match'],
    });
  });
  return captures;
}

async function installActiveWebSocketTracker(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const NativeWebSocket = window.WebSocket;

    // Playwright observes frames; the browser subclass only exposes the active socket for closing.
    class TrackingWebSocket extends NativeWebSocket {
      constructor(url: string | URL, protocols?: string | string[]) {
        super(url, protocols);
        if (String(url).includes('/api/v1/ws')) {
          (window as Window & { __playwrightLastSocket?: WebSocket }).__playwrightLastSocket = this;
        }
      }
    }

    window.WebSocket = TrackingWebSocket as typeof WebSocket;
  });
}

function captureRealtimeWebSockets(page: Page): WebSocketCapture[] {
  const captures: WebSocketCapture[] = [];
  page.on('websocket', (socket) => {
    if (!socket.url().includes('/api/v1/ws')) {
      return;
    }
    const capture: WebSocketCapture = {
      url: socket.url(),
      sent: [],
      received: [],
      events: [],
      closed: false,
    };
    captures.push(capture);
    socket.on('framesent', (event) => {
      const payload =
        typeof event.payload === 'string' ? event.payload : event.payload.toString('utf8');
      capture.sent.push(payload);
      capture.events.push({ direction: 'sent', payload });
    });
    socket.on('framereceived', (event) => {
      const payload =
        typeof event.payload === 'string' ? event.payload : event.payload.toString('utf8');
      capture.received.push(payload);
      capture.events.push({ direction: 'received', payload });
    });
    socket.on('close', () => {
      capture.closed = true;
    });
  });
  return captures;
}

function realtimeMessages(frames: string[]): RealtimeMessage[] {
  const messages: RealtimeMessage[] = [];
  for (const frame of frames) {
    try {
      const value = JSON.parse(frame) as unknown;
      if (
        typeof value === 'object' &&
        value !== null &&
        typeof (value as { type?: unknown }).type === 'string' &&
        typeof (value as { payload?: unknown }).payload === 'object' &&
        (value as { payload?: unknown }).payload !== null
      ) {
        messages.push(value as RealtimeMessage);
      }
    } catch {
      // Ignore non-JSON frames.
    }
  }
  return messages;
}

function runtimeCursor(
  message: RealtimeMessage,
  versionField: 'runtime_version' | 'version' = 'runtime_version',
): RuntimeCursor | null {
  const streamId = message.payload.runtime_stream_id;
  const version = message.payload[versionField];
  if (
    typeof streamId !== 'string' ||
    streamId.trim().length === 0 ||
    typeof version !== 'number' ||
    !Number.isSafeInteger(version) ||
    version < 0
  ) {
    return null;
  }
  return { streamId, version };
}

function sentControls(capture: WebSocketCapture, type: string): RealtimeMessage[] {
  return realtimeMessages(capture.sent).filter((message) => message.type === type);
}

function realtimeMessageEvents(
  capture: WebSocketCapture,
): Array<RealtimeFrameEvent & { message: RealtimeMessage }> {
  return capture.events.flatMap((event) => {
    const message = realtimeMessages([event.payload]).at(0);
    return message === undefined ? [] : [{ ...event, message }];
  });
}

async function waitForSentControl(
  capture: WebSocketCapture,
  type: string,
  predicate: (message: RealtimeMessage) => boolean = () => true,
): Promise<RealtimeMessage> {
  await expect
    .poll(() => sentControls(capture, type).some(predicate), { timeout: 15_000 })
    .toBe(true);
  return sentControls(capture, type).find(predicate)!;
}

function socketResumeCursor(capture: WebSocketCapture): RuntimeCursor {
  const url = new URL(capture.url);
  expect(url.searchParams.get('runtime_protocol')).toBe('2');
  const streamId = url.searchParams.get('runtime_stream_id');
  const rawVersion = url.searchParams.get('runtime_version');
  expect(streamId?.trim().length).toBeGreaterThan(0);
  expect(rawVersion?.trim().length).toBeGreaterThan(0);
  const version = Number(rawVersion);
  expect(Number.isSafeInteger(version)).toBe(true);
  expect(version).toBeGreaterThanOrEqual(0);
  return { streamId: streamId!, version };
}

function expectSocketResumeURL(capture: WebSocketCapture, cursor: RuntimeCursor): void {
  expect(socketResumeCursor(capture)).toEqual(cursor);
}

function bootstrapRuntimeCursor(bootstrap: CanvasBootstrapPayload): RuntimeCursor {
  expect(typeof bootstrap.runtime_stream_id).toBe('string');
  expect(bootstrap.runtime_stream_id?.trim().length).toBeGreaterThan(0);
  expect(Number.isSafeInteger(bootstrap.runtime_version)).toBe(true);
  expect(bootstrap.runtime_version).toBeGreaterThanOrEqual(0);
  return {
    streamId: bootstrap.runtime_stream_id!,
    version: bootstrap.runtime_version!,
  };
}

async function waitForHelloMatching(
  capture: WebSocketCapture,
  bootstrap: CanvasBootstrapPayload,
): Promise<HelloPayload> {
  const hello = await waitForSentControl(
    capture,
    'hello',
    (message) =>
      message.payload.runtime_identity === bootstrap.runtime_identity &&
      message.payload.runtime_version === bootstrap.runtime_version,
  );
  return hello.payload as HelloPayload;
}

function expectBootstrapIdentity(bootstrap: CanvasBootstrapPayload): void {
  expect(bootstrap.topology_version).toBeTruthy();
  expect(typeof bootstrap.runtime_version).toBe('number');
  expect(bootstrap.runtime_identity).toMatch(/^rt-sha256:[a-f0-9]{64}$/);
  expect((bootstrap.devices ?? []).length).toBeGreaterThan(0);
}

test('renders topology after bootstrap', async ({ page }) => {
  await waitForTopology(page);
});

test('renders dashboard rows for seeded devices', async ({ page }) => {
  await waitForTopology(page);

  await page.getByLabel('Devices Dashboard').click();

  await expect(page.getByTestId('dashboard-table')).toBeVisible();
  await expect(page.getByTestId('dashboard-table').getByText('router-a')).toBeVisible();
});

test('recovers from websocket reconnects', async ({ page }) => {
  const mapId = await primaryMapId(page);
  const bootstrapPath = `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/bootstrap`;
  let bootstrapGETs = 0;
  page.on('request', (request) => {
    if (request.method() === 'GET' && new URL(request.url()).pathname === bootstrapPath) {
      bootstrapGETs += 1;
    }
  });
  const bootstraps = captureSavedMapBootstraps(page, mapId);
  const topologyRevalidations = captureSavedMapTopologyRevalidations(page, mapId);
  const sockets = captureRealtimeWebSockets(page);
  await installActiveWebSocketTracker(page);

  await waitForTopology(page);

  await expect.poll(() => bootstraps.length).toBeGreaterThanOrEqual(1);
  await expect.poll(() => sockets.filter((capture) => !capture.closed).length).toBe(1);
  const bootstrap = bootstraps[0].payload;
  expectBootstrapIdentity(bootstrap);
  const bootstrapCursor = bootstrapRuntimeCursor(bootstrap);
  const initialBootstrapGETs = bootstrapGETs;
  const initialSocket = sockets.find((capture) => !capture.closed)!;
  expectSocketResumeURL(initialSocket, bootstrapCursor);

  const initialHello = await waitForSentControl(initialSocket, 'hello');
  expect(initialHello.payload).toMatchObject({
    runtime_protocol: 2,
    runtime_stream_id: bootstrapCursor.streamId,
    runtime_version: bootstrapCursor.version,
  });

  await waitForSentControl(initialSocket, 'runtime_ack', (message) => {
    return runtimeCursor(message) !== null;
  });

  const acknowledgedCursors = sentControls(initialSocket, 'runtime_ack')
    .map((message) => runtimeCursor(message))
    .filter((cursor): cursor is RuntimeCursor => cursor !== null);
  expect(acknowledgedCursors.length).toBeGreaterThan(0);
  const lastAcknowledgedCursor = acknowledgedCursors.at(-1)!;
  const topologyRevalidationBaseline = topologyRevalidations.length;

  const reconnectBanner = page.getByTestId('reconnect-banner');

  await page.evaluate(() => {
    (window as Window & { __playwrightBackendReconnected?: boolean }).__playwrightBackendReconnected = false;
    window.addEventListener('backend-reconnected', () => {
      (window as Window & { __playwrightBackendReconnected?: boolean }).__playwrightBackendReconnected = true;
    }, { once: true });
  });

  const socketBaseline = sockets.length;
  const socketClosed = await page.evaluate(() => {
    const socket = (window as Window & { __playwrightLastSocket?: WebSocket })
      .__playwrightLastSocket;
    if (!socket) {
      return false;
    }
    socket.close();
    return true;
  });
  expect(socketClosed).toBe(true);

  await expect(reconnectBanner).toHaveAttribute('aria-hidden', 'false');

  await expect.poll(() => initialSocket.closed, { timeout: 15_000 }).toBe(true);
  await expect
    .poll(
      () =>
        sockets.filter((capture, index) => index >= socketBaseline && !capture.closed).length,
      { timeout: 15_000 },
    )
    .toBe(1);
  const recoveredSocket = sockets.find(
    (capture, index) => index >= socketBaseline && !capture.closed,
  )!;
  const resumedCursor = socketResumeCursor(recoveredSocket);
  if (resumedCursor.streamId === lastAcknowledgedCursor.streamId) {
    expect(resumedCursor.version).toBeGreaterThanOrEqual(lastAcknowledgedCursor.version);
  }
  const recoveredHello = await waitForSentControl(recoveredSocket, 'hello');
  expect(recoveredHello.payload).toMatchObject({
    runtime_protocol: 2,
    runtime_stream_id: resumedCursor.streamId,
    runtime_version: resumedCursor.version,
  });

  await expect
    .poll(
      () =>
        page.evaluate(() =>
          Boolean(
            (window as Window & { __playwrightBackendReconnected?: boolean })
              .__playwrightBackendReconnected,
          ),
        ),
      { timeout: 15_000 },
    )
    .toBe(true);

  await expect(reconnectBanner).toHaveAttribute('aria-hidden', 'true', { timeout: 15_000 });

  await expect
    .poll(
      () =>
        realtimeMessages(recoveredSocket.received).some(
          (message) =>
            message.type === 'ready' &&
            ['current', 'replay', 'snapshot'].includes(String(message.payload.sync_mode)) &&
            runtimeCursor(message) !== null,
        ),
      { timeout: 15_000 },
    )
    .toBe(true);
  const recoveredReady = realtimeMessageEvents(recoveredSocket).find(
    (event) =>
      event.direction === 'received' &&
      event.message.type === 'ready' &&
      ['current', 'replay', 'snapshot'].includes(String(event.message.payload.sync_mode)) &&
      runtimeCursor(event.message) !== null,
  )!.message;
  const recoveredMode = String(recoveredReady.payload.sync_mode);
  const recoveredTarget = runtimeCursor(recoveredReady)!;

  await waitForSentControl(recoveredSocket, 'runtime_ack', (message) => {
    const cursor = runtimeCursor(message);
    return (
      cursor?.streamId === recoveredTarget.streamId && cursor.version === recoveredTarget.version
    );
  });

  const recoveredEvents = realtimeMessageEvents(recoveredSocket);
  const readyEventIndex = recoveredEvents.findIndex((event) => {
    if (
      event.direction !== 'received' ||
      event.message.type !== 'ready' ||
      event.message.payload.sync_mode !== recoveredMode
    ) {
      return false;
    }
    const cursor = runtimeCursor(event.message);
    return (
      cursor?.streamId === recoveredTarget.streamId && cursor.version === recoveredTarget.version
    );
  });
  expect(readyEventIndex).toBeGreaterThanOrEqual(0);
  if (recoveredMode === 'current') {
    expect(recoveredTarget).toEqual(resumedCursor);
  } else if (recoveredMode === 'replay') {
    expect(recoveredTarget.streamId).toBe(resumedCursor.streamId);
    expect(recoveredTarget.version).toBeGreaterThan(resumedCursor.version);
  } else {
    expect(recoveredMode).toBe('snapshot');
    if (recoveredTarget.streamId === resumedCursor.streamId) {
      expect(recoveredTarget.version).toBeGreaterThanOrEqual(resumedCursor.version);
    }
  }
  if (recoveredMode !== 'current') {
    const recoveryMessageType = recoveredMode === 'replay' ? 'runtime_replay' : 'snapshot';
    const recoveryStateEventIndex = recoveredEvents.findIndex(
      (event) =>
        event.direction === 'received' &&
        event.message.type === recoveryMessageType &&
        runtimeCursor(event.message, 'version')?.streamId === recoveredTarget.streamId &&
        runtimeCursor(event.message, 'version')?.version === recoveredTarget.version,
    );
    expect(recoveryStateEventIndex).toBeGreaterThanOrEqual(0);
    expect(recoveryStateEventIndex).toBeLessThan(readyEventIndex);
  }
  const recoveredAckEventIndex = recoveredEvents.findIndex((event) => {
    if (event.direction !== 'sent' || event.message.type !== 'runtime_ack') {
      return false;
    }
    const cursor = runtimeCursor(event.message);
    return (
      cursor?.streamId === recoveredTarget.streamId && cursor.version === recoveredTarget.version
    );
  });
  expect(recoveredAckEventIndex).toBeGreaterThan(readyEventIndex);

  let observedCursor = resumedCursor;
  for (const [eventIndex, event] of recoveredEvents.entries()) {
    if (event.direction === 'received') {
      const versionField =
        event.message.type === 'runtime_delta' ||
        event.message.type === 'runtime_replay' ||
        event.message.type === 'snapshot'
          ? 'version'
          : 'runtime_version';
      const nextCursor = runtimeCursor(event.message, versionField);
      if (nextCursor !== null) {
        observedCursor = nextCursor;
      }
      continue;
    }
    if (event.message.type !== 'resume_runtime') {
      continue;
    }
    const cursor = runtimeCursor(event.message);
    expect(cursor).not.toBeNull();
    if (cursor?.streamId === observedCursor.streamId) {
      expect(cursor.version).toBeGreaterThanOrEqual(observedCursor.version);
    } else {
      // HTTP fallback state is applied outside the WebSocket frame timeline.
      expect(cursor?.streamId).toBe(recoveredTarget.streamId);
      expect(eventIndex).toBeLessThan(readyEventIndex);
      expect(cursor?.version).toBeLessThanOrEqual(recoveredTarget.version);
    }
    if (cursor?.streamId === resumedCursor.streamId) {
      expect(cursor.version).toBeGreaterThanOrEqual(resumedCursor.version);
    }
  }

  await expectTopologyVisible(page);
  await expect
    .poll(() => topologyRevalidations.length, { timeout: 15_000 })
    .toBeGreaterThan(topologyRevalidationBaseline);
  const reconnectRevalidations = topologyRevalidations.slice(topologyRevalidationBaseline);
  for (const revalidation of reconnectRevalidations) {
    expect(revalidation).toMatchObject({ status: 304 });
    expect(revalidation.ifNoneMatch).toBeTruthy();
  }
  expect(bootstrapGETs).toBe(initialBootstrapGETs);
  expect(bootstraps).toHaveLength(initialBootstrapGETs);
});

test('reuses saved-map runtime identity across browser reload bootstrap', async ({ page }) => {
  const mapId = await primaryMapId(page);
  const bootstraps = captureSavedMapBootstraps(page, mapId);
  const sockets = captureRealtimeWebSockets(page);

  await waitForTopology(page);
  await expect.poll(() => bootstraps.length).toBeGreaterThanOrEqual(1);
  await expect.poll(() => sockets.filter((capture) => !capture.closed).length).toBe(1);
  const firstBootstrap = bootstraps[0].payload;
  expect(bootstraps[0].url).toContain(`/api/v1/canvas/maps/${mapId}/bootstrap`);
  expectBootstrapIdentity(firstBootstrap);
  const firstSocket = sockets.find((capture) => !capture.closed)!;
  const firstHello = await waitForHelloMatching(firstSocket, firstBootstrap);
  expect(firstHello.runtime_version).toBe(firstBootstrap.runtime_version);
  expect(firstHello.runtime_identity).toBe(firstBootstrap.runtime_identity);

  const socketBaseline = sockets.length;
  await page.reload();
  await expectTopologyVisible(page);
  await expect.poll(() => bootstraps.length).toBeGreaterThanOrEqual(2);
  await expect
    .poll(
      () =>
        sockets.filter((capture, index) => index >= socketBaseline && !capture.closed).length,
    )
    .toBe(1);
  const secondBootstrap = bootstraps[1].payload;
  expect(bootstraps[1].url).toContain(`/api/v1/canvas/maps/${mapId}/bootstrap`);
  expectBootstrapIdentity(secondBootstrap);
  const secondSocket = sockets.find(
    (capture, index) => index >= socketBaseline && !capture.closed,
  )!;
  const secondHello = await waitForHelloMatching(secondSocket, secondBootstrap);
  expect(secondHello.runtime_version).toBe(secondBootstrap.runtime_version);
  expect(secondHello.runtime_identity).toBe(secondBootstrap.runtime_identity);

  expect(secondBootstrap.topology_version).toBe(firstBootstrap.topology_version);
  if (secondBootstrap.runtime_version === firstBootstrap.runtime_version) {
    expect(secondBootstrap.runtime_identity).toBe(firstBootstrap.runtime_identity);
  }
});

test('opens the device detail panel from the topology canvas', async ({ page }) => {
  const sockets = captureRealtimeWebSockets(page);

  await waitForTopology(page);
  await expect.poll(() => sockets.filter((capture) => !capture.closed).length).toBe(1);
  const activeSocket = sockets.find((capture) => !capture.closed)!;

  await page.getByTestId('topology-canvas').getByText('127.0.10.21').first().click();

  await waitForSentControl(activeSocket, 'subscribe_detail');

  await expect(page.getByTestId('device-detail-panel')).toBeVisible();
  await expect(page.getByTestId('device-detail-runtime')).toContainText('Live Detail Telemetry');
});
