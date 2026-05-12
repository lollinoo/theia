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

async function waitForBackend(): Promise<void> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < 60_000) {
    try {
      const response = await fetch('http://127.0.0.1:38080/api/v1/health');
      if (response.ok) {
        return;
      }
    } catch {
      // Backend is still starting.
    }

    await new Promise((resolve) => setTimeout(resolve, 500));
  }

  throw new Error('Backend did not become healthy within 60 seconds');
}

async function fetchSeededDevice(): Promise<SeedDevice | null> {
  const devicesResponse = await fetch('http://127.0.0.1:38080/api/v1/devices');
  if (!devicesResponse.ok) {
    throw new Error(`Failed to fetch devices: ${devicesResponse.status}`);
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

async function seedRouter(): Promise<SeedDevice> {
  const existingDevice = await fetchSeededDevice();
  if (existingDevice) {
    return existingDevice;
  }

  const createResponse = await fetch('http://127.0.0.1:38080/api/v1/devices', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(deviceSeedPayload),
  });

  if (createResponse.ok) {
    const seededDevice = await fetchSeededDevice();
    if (seededDevice) {
      return seededDevice;
    }
    throw new Error('Seeded router-a was not returned by the devices API');
  }

  if (createResponse.status !== 400) {
    throw new Error(`Failed to seed router-a: ${createResponse.status}`);
  }

  const legacyCreateResponse = await fetch('http://127.0.0.1:38080/api/v1/devices', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(legacyDeviceSeedPayload),
  });

  if (!legacyCreateResponse.ok) {
    throw new Error(
      `Failed to seed router-a with legacy SNMP version: ${legacyCreateResponse.status}`,
    );
  }

  const seededDevice = await fetchSeededDevice();
  if (!seededDevice) {
    throw new Error('Seeded router-a was not returned by the devices API');
  }
  return seededDevice;
}

async function seedArea(): Promise<void> {
  const areasResponse = await fetch('http://127.0.0.1:38080/api/v1/areas');
  if (!areasResponse.ok) {
    throw new Error(`Failed to fetch areas: ${areasResponse.status}`);
  }

  const areasPayload = (await areasResponse.json()) as APIListResponse<{ name?: string }>;
  const alreadySeeded = (areasPayload.data ?? []).some((area) =>
    area.name === areaSeedPayload.name,
  );
  if (alreadySeeded) {
    return;
  }

  const createResponse = await fetch('http://127.0.0.1:38080/api/v1/areas', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(areaSeedPayload),
  });

  if (!createResponse.ok && createResponse.status !== 409) {
    throw new Error(`Failed to seed Backbone area: ${createResponse.status}`);
  }
}

async function getPrimaryMap(): Promise<SeedCanvasMap> {
  const mapsResponse = await fetch('http://127.0.0.1:38080/api/v1/canvas/maps');
  if (!mapsResponse.ok) {
    throw new Error(`Failed to fetch canvas maps: ${mapsResponse.status}`);
  }

  const mapsPayload = (await mapsResponse.json()) as APIListResponse<SeedCanvasMap>;
  const primaryMap = (mapsPayload.data ?? []).find((map) => map.is_default === true);
  if (!primaryMap) {
    throw new Error('No primary canvas map was seeded');
  }
  return primaryMap;
}

async function seedDeviceIntoPrimaryMap(map: SeedCanvasMap, device: SeedDevice): Promise<void> {
  const response = await fetch(
    `http://127.0.0.1:38080/api/v1/canvas/maps/${encodeURIComponent(map.id)}/devices/${encodeURIComponent(device.id)}`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ include_connected_links: true }),
    },
  );

  if (!response.ok && response.status !== 409) {
    throw new Error(`Failed to add router-a to primary map: ${response.status}`);
  }
}

async function seedPrimaryMapArea(map: SeedCanvasMap): Promise<SeedArea> {
  const areasResponse = await fetch(
    `http://127.0.0.1:38080/api/v1/canvas/maps/${encodeURIComponent(map.id)}/areas`,
  );
  if (!areasResponse.ok) {
    throw new Error(`Failed to fetch primary map areas: ${areasResponse.status}`);
  }

  const areasPayload = (await areasResponse.json()) as APIListResponse<SeedArea>;
  const existingArea = (areasPayload.data ?? []).find((area) => area.name === areaSeedPayload.name);
  if (existingArea) {
    return existingArea;
  }

  const createResponse = await fetch(
    `http://127.0.0.1:38080/api/v1/canvas/maps/${encodeURIComponent(map.id)}/areas`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(areaSeedPayload),
    },
  );

  if (!createResponse.ok) {
    throw new Error(`Failed to seed primary map Backbone area: ${createResponse.status}`);
  }

  const createPayload = (await createResponse.json()) as APIDataResponse<SeedArea>;
  if (!createPayload.data) {
    throw new Error('Primary map Backbone area response did not include data');
  }
  return createPayload.data;
}

async function assignDeviceToPrimaryMapArea(
  map: SeedCanvasMap,
  device: SeedDevice,
  area: SeedArea,
): Promise<void> {
  const response = await fetch(
    `http://127.0.0.1:38080/api/v1/canvas/maps/${encodeURIComponent(map.id)}/device-areas`,
    {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ device_ids: [device.id], area_ids: [area.id] }),
    },
  );

  if (!response.ok) {
    throw new Error(`Failed to assign router-a to primary map Backbone area: ${response.status}`);
  }
}

async function seedPrimaryMap(device: SeedDevice): Promise<void> {
  const primaryMap = await getPrimaryMap();
  await seedDeviceIntoPrimaryMap(primaryMap, device);
  const primaryArea = await seedPrimaryMapArea(primaryMap);
  await assignDeviceToPrimaryMapArea(primaryMap, device, primaryArea);
}

async function assertSeededPrimaryMap(device: SeedDevice): Promise<void> {
  const primaryMap = await getPrimaryMap();
  const topologyResponse = await fetch(
    `http://127.0.0.1:38080/api/v1/canvas/maps/${encodeURIComponent(primaryMap.id)}/topology`,
  );
  if (!topologyResponse.ok) {
    throw new Error(`Failed to fetch seeded primary topology: ${topologyResponse.status}`);
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
  const device = await seedRouter();
  await seedArea();
  await seedPrimaryMap(device);
  await assertSeededPrimaryMap(device);
}
