import { expect, test } from '@playwright/test';

async function waitForTopology(page: Parameters<typeof test>[0]['page']): Promise<void> {
  await page.goto('/');
  await expect(page.getByTestId('topology-canvas')).toBeVisible();
  await expect(page.getByTestId('topology-canvas').getByText('127.0.10.21').first()).toBeVisible();
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

  const reconnectBanner = page.locator('div[aria-hidden]', { hasText: 'Reconnecting...' });

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
  await expect(page.getByTestId('device-detail-runtime')).toContainText('Operational status');
  await expect(page.getByTestId('device-detail-runtime')).toContainText('up');
});
