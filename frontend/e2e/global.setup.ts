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

export default async function globalSetup(): Promise<void> {
  await waitForBackend();

  const devicesResponse = await fetch('http://127.0.0.1:38080/api/v1/devices');
  if (!devicesResponse.ok) {
    throw new Error(`Failed to fetch devices: ${devicesResponse.status}`);
  }

  const devicesPayload = await devicesResponse.json() as { data?: Array<{ hostname?: string; ip?: string }> };
  const alreadySeeded = (devicesPayload.data ?? []).some((device) =>
    device.hostname === deviceSeedPayload.hostname || device.ip === deviceSeedPayload.ip,
  );

  if (alreadySeeded) {
    return;
  }

  const createResponse = await fetch('http://127.0.0.1:38080/api/v1/devices', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(deviceSeedPayload),
  });

  if (createResponse.ok) {
    return;
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
    throw new Error(`Failed to seed router-a with legacy SNMP version: ${legacyCreateResponse.status}`);
  }
}
