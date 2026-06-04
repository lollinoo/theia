import { expect, test, type Page } from '@playwright/test';

type CanvasMapSummary = {
  id: string;
  is_default?: boolean;
};

type CanvasBootstrapPayload = {
  topology_version?: string;
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

type HelloPayload = {
  runtime_version?: number;
  runtime_identity?: string;
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

async function installWebSocketHelloRecorder(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const NativeWebSocket = window.WebSocket;

    class TrackingWebSocket extends NativeWebSocket {
      constructor(url: string | URL, protocols?: string | string[]) {
        super(url, protocols);
        const socketUrl = String(url);
        if (socketUrl.includes('/api/v1/ws')) {
          (window as Window & { __playwrightHelloPayloads?: unknown[] }).__playwrightHelloPayloads = [];
          const nativeSend = this.send.bind(this);
          this.send = (data: string | ArrayBufferLike | Blob | ArrayBufferView) => {
            if (typeof data === 'string') {
              try {
                const message = JSON.parse(data) as { type?: string; payload?: unknown };
                if (message.type === 'hello') {
                  (window as Window & { __playwrightHelloPayloads?: unknown[] }).__playwrightHelloPayloads?.push(
                    message.payload,
                  );
                }
              } catch {
                // Ignore non-JSON frames.
              }
            }
            nativeSend(data);
          };
        }
      }
    }

    window.WebSocket = TrackingWebSocket as typeof WebSocket;
  });
}

async function waitForHelloMatching(
  page: Page,
  bootstrap: CanvasBootstrapPayload,
): Promise<HelloPayload> {
  await page.waitForFunction(
    ({ runtimeIdentity, runtimeVersion }) => {
      const payloads =
        (window as Window & { __playwrightHelloPayloads?: Array<Record<string, unknown>> })
          .__playwrightHelloPayloads ?? [];
      return payloads.some(
        (payload) =>
          payload.runtime_identity === runtimeIdentity &&
          payload.runtime_version === runtimeVersion,
      );
    },
    {
      runtimeIdentity: bootstrap.runtime_identity,
      runtimeVersion: bootstrap.runtime_version,
    },
    { timeout: 15_000 },
  );

  return page.evaluate(
    ({ runtimeIdentity, runtimeVersion }) => {
      const payloads =
        (window as Window & { __playwrightHelloPayloads?: HelloPayload[] }).__playwrightHelloPayloads ??
        [];
      const match = payloads.find(
        (payload) =>
          payload.runtime_identity === runtimeIdentity &&
          payload.runtime_version === runtimeVersion,
      );
      if (!match) {
        throw new Error('matching websocket hello was not recorded');
      }
      return match;
    },
    {
      runtimeIdentity: bootstrap.runtime_identity,
      runtimeVersion: bootstrap.runtime_version,
    },
  );
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
  await page.addInitScript(() => {
    const NativeWebSocket = window.WebSocket;

    class TrackingWebSocket extends NativeWebSocket {
      constructor(url: string | URL, protocols?: string | string[]) {
        super(url, protocols);
        const socketUrl = String(url);
        if (socketUrl.includes('/api/v1/ws')) {
          (window as Window & { __playwrightLastSocket?: WebSocket }).__playwrightLastSocket = this;
        }
      }
    }

    window.WebSocket = TrackingWebSocket as typeof WebSocket;
  });

  await waitForTopology(page);

  const reconnectBanner = page.getByTestId('reconnect-banner');

  await page.evaluate(() => {
    (window as Window & { __playwrightBackendReconnected?: boolean }).__playwrightBackendReconnected = false;
    window.addEventListener('backend-reconnected', () => {
      (window as Window & { __playwrightBackendReconnected?: boolean }).__playwrightBackendReconnected = true;
    }, { once: true });
  });

  await page.evaluate(() => {
    (window as Window & { __playwrightLastSocket?: WebSocket }).__playwrightLastSocket?.close();
  });

  await expect(reconnectBanner).toHaveAttribute('aria-hidden', 'false');

  await page.waitForFunction(() => {
    return Boolean((window as Window & { __playwrightBackendReconnected?: boolean }).__playwrightBackendReconnected);
  }, null, { timeout: 15_000 });

  await expect(reconnectBanner).toHaveAttribute('aria-hidden', 'true', { timeout: 15_000 });
  await expect(page.getByTestId('topology-canvas').getByText('127.0.10.21').first()).toBeVisible();
});

test('reuses saved-map runtime identity across browser reload bootstrap', async ({ page }) => {
  const mapId = await primaryMapId(page);
  const bootstraps = captureSavedMapBootstraps(page, mapId);
  await installWebSocketHelloRecorder(page);

  await waitForTopology(page);
  await expect.poll(() => bootstraps.length).toBeGreaterThanOrEqual(1);
  const firstBootstrap = bootstraps[0].payload;
  expect(bootstraps[0].url).toContain(`/api/v1/canvas/maps/${mapId}/bootstrap`);
  expectBootstrapIdentity(firstBootstrap);
  const firstHello = await waitForHelloMatching(page, firstBootstrap);
  expect(firstHello.runtime_version).toBe(firstBootstrap.runtime_version);
  expect(firstHello.runtime_identity).toBe(firstBootstrap.runtime_identity);

  await page.reload();
  await expectTopologyVisible(page);
  await expect.poll(() => bootstraps.length).toBeGreaterThanOrEqual(2);
  const secondBootstrap = bootstraps[1].payload;
  expect(bootstraps[1].url).toContain(`/api/v1/canvas/maps/${mapId}/bootstrap`);
  expectBootstrapIdentity(secondBootstrap);
  const secondHello = await waitForHelloMatching(page, secondBootstrap);
  expect(secondHello.runtime_version).toBe(secondBootstrap.runtime_version);
  expect(secondHello.runtime_identity).toBe(secondBootstrap.runtime_identity);

  expect(secondBootstrap.topology_version).toBe(firstBootstrap.topology_version);
  if (secondBootstrap.runtime_version === firstBootstrap.runtime_version) {
    expect(secondBootstrap.runtime_identity).toBe(firstBootstrap.runtime_identity);
  }
});

test('opens the device detail panel from the topology canvas', async ({ page }) => {
  await page.addInitScript(() => {
    const NativeWebSocket = window.WebSocket;

    class TrackingWebSocket extends NativeWebSocket {
      constructor(url: string | URL, protocols?: string | string[]) {
        super(url, protocols);
        const socketUrl = String(url);
        if (socketUrl.includes('/api/v1/ws')) {
          (window as Window & { __playwrightSocketMessages?: string[] }).__playwrightSocketMessages = [];
          const nativeSend = this.send.bind(this);
          this.send = (data: string | ArrayBufferLike | Blob | ArrayBufferView) => {
            if (typeof data === 'string') {
              (window as Window & { __playwrightSocketMessages?: string[] }).__playwrightSocketMessages?.push(data);
            }
            nativeSend(data);
          };
        }
      }
    }

    window.WebSocket = TrackingWebSocket as typeof WebSocket;
  });

  await waitForTopology(page);

  await page.getByTestId('topology-canvas').getByText('127.0.10.21').first().click();

  await page.waitForFunction(() => {
    const messages = (window as Window & { __playwrightSocketMessages?: string[] }).__playwrightSocketMessages ?? [];
    return messages.some((message) => message.includes('subscribe_detail'));
  });

  await expect(page.getByTestId('device-detail-panel')).toBeVisible();
  await expect(page.getByTestId('device-detail-runtime')).toContainText('Live Detail Telemetry');
});
