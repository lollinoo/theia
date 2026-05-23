import { request, type APIRequestContext } from '@playwright/test';

const backendBaseURL = 'http://127.0.0.1:38080';
const authStorageStatePath = '/tmp/theia-playwright-auth.json';
const bootstrapPassword = 'theia';
const e2ePassword = 'Correct Horse Battery Staple 2026!';

const deviceSeedPayload = {
  hostname: 'router-a',
  ip: '127.0.10.21',
  snmp: {
    version: 'v2c',
    community: 'public',
  },
};

const legacyDeviceSeedPayload = {
  ...deviceSeedPayload,
  snmp: {
    ...deviceSeedPayload.snmp,
    version: '2c',
  },
};

const areaSeedPayload = {
  name: 'Backbone',
  description: 'Seeded area for topology hub e2e coverage',
  color: '#2979FF',
};

type APIListResponse<T> = {
  data?: T[];
};

type APIDataResponse<T> = {
  data?: T;
};

type AuthSession = {
  authenticated?: boolean;
  user?: {
    must_change_password?: boolean;
  };
};

type SeedDevice = {
  id: string;
  hostname?: string;
  ip?: string;
};

type DeviceResource = {
  id?: string;
  attributes?: {
    hostname?: string;
    ip?: string;
  };
};

type SeedArea = {
  id: string;
  name?: string;
  description?: string;
  color?: string;
};

type SeedCanvasMap = {
  id: string;
  name?: string;
  is_default?: boolean;
};

type AuthenticatedAPI = {
  api: APIRequestContext;
  csrfHeaders: Record<string, string>;
};

async function waitForBackend(): Promise<void> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < 60_000) {
    try {
      const response = await fetch(`${backendBaseURL}/api/v1/auth/me`);
      if (response.ok) {
        return;
      }
    } catch {
      // Backend is still starting.
    }

    await new Promise((resolve) => setTimeout(resolve, 500));
  }

  throw new Error('Backend did not become ready within 60 seconds');
}

async function createAuthenticatedAPI(): Promise<AuthenticatedAPI> {
  const api = await request.newContext({ baseURL: backendBaseURL });

  let session = await login(api, bootstrapPassword);
  if (!session) {
    session = await login(api, e2ePassword);
  }
  if (!session?.authenticated) {
    throw new Error('Unable to authenticate the e2e administrator account');
  }

  let csrfHeaders = await readCSRFHeaders(api);
  if (session.user?.must_change_password) {
    const changeResponse = await api.post('/api/v1/auth/password/change', {
      headers: csrfHeaders,
      data: {
        current_password: bootstrapPassword,
        new_password: e2ePassword,
      },
    });
    if (!changeResponse.ok()) {
      throw new Error(`Failed to set e2e administrator password: ${changeResponse.status()}`);
    }
    csrfHeaders = await readCSRFHeaders(api);
  }

  await api.storageState({ path: authStorageStatePath });
  return { api, csrfHeaders };
}

async function login(api: APIRequestContext, password: string): Promise<AuthSession | null> {
  const response = await api.post('/api/v1/auth/login', {
    data: {
      identifier: 'administrator',
      password,
    },
  });
  if (response.status() === 401 || response.status() === 403) {
    return null;
  }
  if (!response.ok()) {
    throw new Error(`Failed to login e2e administrator: ${response.status()}`);
  }
  return (await response.json()) as AuthSession;
}

async function readCSRFHeaders(api: APIRequestContext): Promise<Record<string, string>> {
  const state = await api.storageState();
  const csrfCookie = state.cookies.find((cookie) => cookie.name === 'theia_csrf');
  if (!csrfCookie?.value) {
    throw new Error('Authenticated e2e session did not receive a CSRF cookie');
  }
  return { 'X-CSRF-Token': csrfCookie.value };
}

async function fetchSeededDevice(api: APIRequestContext): Promise<SeedDevice | null> {
  const devicesResponse = await api.get('/api/v1/devices');
  if (!devicesResponse.ok()) {
    throw new Error(`Failed to fetch devices: ${devicesResponse.status()}`);
  }

  const devicesPayload = (await devicesResponse.json()) as APIListResponse<DeviceResource>;
  const device = (devicesPayload.data ?? []).find(
    (resource) =>
      resource.attributes?.hostname === deviceSeedPayload.hostname ||
      resource.attributes?.ip === deviceSeedPayload.ip,
  );
  if (!device?.id) {
    return null;
  }
  return {
    id: device.id,
    hostname: device.attributes?.hostname,
    ip: device.attributes?.ip,
  };
}

async function seedRouter({ api, csrfHeaders }: AuthenticatedAPI): Promise<SeedDevice> {
  const existingDevice = await fetchSeededDevice(api);
  if (existingDevice) {
    return existingDevice;
  }

  const createResponse = await api.post('/api/v1/devices', {
    headers: csrfHeaders,
    data: deviceSeedPayload,
  });

  if (createResponse.ok()) {
    const seededDevice = await fetchSeededDevice(api);
    if (seededDevice) {
      return seededDevice;
    }
    throw new Error('Seeded router-a was not returned by the devices API');
  }

  if (createResponse.status() !== 400) {
    throw new Error(`Failed to seed router-a: ${createResponse.status()}`);
  }

  const legacyCreateResponse = await api.post('/api/v1/devices', {
    headers: csrfHeaders,
    data: legacyDeviceSeedPayload,
  });

  if (!legacyCreateResponse.ok()) {
    throw new Error(
      `Failed to seed router-a with legacy SNMP version: ${legacyCreateResponse.status()}`,
    );
  }

  const seededDevice = await fetchSeededDevice(api);
  if (!seededDevice) {
    throw new Error('Seeded router-a was not returned by the devices API');
  }
  return seededDevice;
}

async function seedArea({ api, csrfHeaders }: AuthenticatedAPI): Promise<void> {
  const areasResponse = await api.get('/api/v1/areas');
  if (!areasResponse.ok()) {
    throw new Error(`Failed to fetch areas: ${areasResponse.status()}`);
  }

  const areasPayload = (await areasResponse.json()) as APIListResponse<{ name?: string }>;
  const alreadySeeded = (areasPayload.data ?? []).some((area) => area.name === areaSeedPayload.name);
  if (alreadySeeded) {
    return;
  }

  const createResponse = await api.post('/api/v1/areas', {
    headers: csrfHeaders,
    data: areaSeedPayload,
  });

  if (!createResponse.ok() && createResponse.status() !== 409) {
    throw new Error(`Failed to seed Backbone area: ${createResponse.status()}`);
  }
}

async function getPrimaryMap(api: APIRequestContext): Promise<SeedCanvasMap> {
  const mapsResponse = await api.get('/api/v1/canvas/maps');
  if (!mapsResponse.ok()) {
    throw new Error(`Failed to fetch canvas maps: ${mapsResponse.status()}`);
  }

  const mapsPayload = (await mapsResponse.json()) as APIListResponse<SeedCanvasMap>;
  const primaryMap = (mapsPayload.data ?? []).find((map) => map.is_default === true);
  if (!primaryMap) {
    throw new Error('No primary canvas map was seeded');
  }
  return primaryMap;
}

async function seedDeviceIntoPrimaryMap(
  { api, csrfHeaders }: AuthenticatedAPI,
  map: SeedCanvasMap,
  device: SeedDevice,
): Promise<void> {
  const response = await api.post(
    `/api/v1/canvas/maps/${encodeURIComponent(map.id)}/devices/${encodeURIComponent(device.id)}`,
    {
      headers: csrfHeaders,
      data: { include_connected_links: true },
    },
  );

  if (!response.ok() && response.status() !== 409) {
    throw new Error(`Failed to add router-a to primary map: ${response.status()}`);
  }
}

async function seedPrimaryMapArea(
  { api, csrfHeaders }: AuthenticatedAPI,
  map: SeedCanvasMap,
): Promise<SeedArea> {
  const areasResponse = await api.get(
    `/api/v1/canvas/maps/${encodeURIComponent(map.id)}/areas`,
  );
  if (!areasResponse.ok()) {
    throw new Error(`Failed to fetch primary map areas: ${areasResponse.status()}`);
  }

  const areasPayload = (await areasResponse.json()) as APIListResponse<SeedArea>;
  const existingArea = (areasPayload.data ?? []).find((area) => area.name === areaSeedPayload.name);
  if (existingArea) {
    return existingArea;
  }

  const createResponse = await api.post(`/api/v1/canvas/maps/${encodeURIComponent(map.id)}/areas`, {
    headers: csrfHeaders,
    data: areaSeedPayload,
  });

  if (!createResponse.ok()) {
    throw new Error(`Failed to seed primary map Backbone area: ${createResponse.status()}`);
  }

  const createPayload = (await createResponse.json()) as APIDataResponse<SeedArea>;
  if (!createPayload.data) {
    throw new Error('Primary map Backbone area response did not include data');
  }
  return createPayload.data;
}

async function assignDeviceToPrimaryMapArea(
  { api, csrfHeaders }: AuthenticatedAPI,
  map: SeedCanvasMap,
  device: SeedDevice,
  area: SeedArea,
): Promise<void> {
  const response = await api.put(
    `/api/v1/canvas/maps/${encodeURIComponent(map.id)}/device-areas`,
    {
      headers: csrfHeaders,
      data: { device_ids: [device.id], area_ids: [area.id] },
    },
  );

  if (!response.ok()) {
    throw new Error(`Failed to assign router-a to primary map Backbone area: ${response.status()}`);
  }
}

async function seedPrimaryMap(authenticatedAPI: AuthenticatedAPI, device: SeedDevice): Promise<void> {
  const primaryMap = await getPrimaryMap(authenticatedAPI.api);
  await seedDeviceIntoPrimaryMap(authenticatedAPI, primaryMap, device);
  const primaryArea = await seedPrimaryMapArea(authenticatedAPI, primaryMap);
  await assignDeviceToPrimaryMapArea(authenticatedAPI, primaryMap, device, primaryArea);
}

async function assertSeededPrimaryMap(api: APIRequestContext, device: SeedDevice): Promise<void> {
  const primaryMap = await getPrimaryMap(api);
  const topologyResponse = await api.get(
    `/api/v1/canvas/maps/${encodeURIComponent(primaryMap.id)}/topology`,
  );
  if (!topologyResponse.ok()) {
    throw new Error(`Failed to fetch seeded primary topology: ${topologyResponse.status()}`);
  }

  const topologyPayload = (await topologyResponse.json()) as {
    devices?: Array<{ id?: string }>;
    areas?: Array<{ name?: string }>;
  };
  const hasSeededDevice = (topologyPayload.devices ?? []).some(
    (candidate) => candidate.id === device.id,
  );
  const hasSeededArea = (topologyPayload.areas ?? []).some(
    (candidate) => candidate.name === areaSeedPayload.name,
  );
  if (!hasSeededDevice || !hasSeededArea) {
    throw new Error('Primary map seed did not produce the expected device and area membership');
  }
}

export default async function globalSetup(): Promise<void> {
  await waitForBackend();
  const authenticatedAPI = await createAuthenticatedAPI();
  try {
    const device = await seedRouter(authenticatedAPI);
    await seedArea(authenticatedAPI);
    await seedPrimaryMap(authenticatedAPI, device);
    await assertSeededPrimaryMap(authenticatedAPI.api, device);
  } finally {
    await authenticatedAPI.api.dispose();
  }
}
