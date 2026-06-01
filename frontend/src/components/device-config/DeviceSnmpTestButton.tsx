import { useEffect, useRef, useState } from 'react';
import { testSNMPConnection } from '../../api/client';

interface DeviceSnmpTestButtonProps {
  deviceId: string;
}

type DeviceSnmpTestResult = Awaited<ReturnType<typeof testSNMPConnection>>;

export function DeviceSnmpTestButton({ deviceId }: DeviceSnmpTestButtonProps) {
  const [testingDeviceId, setTestingDeviceId] = useState<string | null>(null);
  const [resultState, setResultState] = useState<{
    deviceId: string;
    result: DeviceSnmpTestResult;
  } | null>(null);
  const requestGenerationRef = useRef(0);
  const activeDeviceIdRef = useRef(deviceId);

  if (activeDeviceIdRef.current !== deviceId) {
    activeDeviceIdRef.current = deviceId;
    requestGenerationRef.current += 1;
  }

  const testing = testingDeviceId === deviceId;
  const result = resultState?.deviceId === deviceId ? resultState.result : null;

  useEffect(() => {
    setTestingDeviceId(null);
    setResultState(null);
  }, [deviceId]);

  async function handleTest() {
    const requestGeneration = requestGenerationRef.current + 1;
    requestGenerationRef.current = requestGeneration;
    const requestDeviceId = deviceId;
    setTestingDeviceId(deviceId);
    setResultState(null);
    try {
      const r = await testSNMPConnection(deviceId);
      if (
        requestGenerationRef.current !== requestGeneration ||
        activeDeviceIdRef.current !== requestDeviceId
      ) {
        return;
      }
      setResultState({ deviceId: requestDeviceId, result: r });
    } catch (err) {
      if (
        requestGenerationRef.current !== requestGeneration ||
        activeDeviceIdRef.current !== requestDeviceId
      ) {
        return;
      }
      setResultState({
        deviceId: requestDeviceId,
        result: { success: false, error: err instanceof Error ? err.message : 'Test failed' },
      });
    } finally {
      if (
        requestGenerationRef.current === requestGeneration &&
        activeDeviceIdRef.current === requestDeviceId
      ) {
        setTestingDeviceId(null);
      }
    }
  }

  return (
    <div className="space-y-2">
      <button
        type="button"
        onClick={() => {
          void handleTest();
        }}
        disabled={testing}
        className="w-full rounded-lg bg-surface-high px-4 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
      >
        {testing ? 'Testing SNMP...' : 'Test SNMP Connectivity'}
      </button>
      {result && (
        <div
          className={`rounded-lg border px-3 py-2 text-xs ${
            result.success
              ? 'border-status-up/30 bg-status-up/10 text-status-up'
              : 'border-status-down/30 bg-status-down/10 text-status-down'
          }`}
        >
          {result.success ? (
            <div className="space-y-1">
              <div className="font-medium">SNMP OK</div>
              {result.sys_name && <div>sysName: {result.sys_name}</div>}
              {result.sys_descr && <div className="truncate">sysDescr: {result.sys_descr}</div>}
            </div>
          ) : (
            <div className="space-y-1">
              <div className="font-medium">SNMP Failed</div>
              <div className="break-words">{result.error}</div>
              {result.target_ip && (
                <div className="text-on-bg-secondary">Target: {result.target_ip}:161 (UDP)</div>
              )}
              {result.snmp_version && (
                <div className="text-on-bg-secondary">Version: {result.snmp_version}</div>
              )}
              <div className="text-on-bg-secondary mt-1">
                Check: SNMP enabled on device, community/credentials correct, UDP/161 reachable from
                container
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
