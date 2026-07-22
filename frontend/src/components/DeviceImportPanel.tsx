/**
 * Owns the one-time, label-blind node import workflow shown inside the Admin Area.
 */
import {
  type ChangeEvent,
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  commitDeviceImport,
  type DeviceImportCommitResult,
  type DeviceImportConfiguration,
  type DeviceImportDiagnostic,
  type DeviceImportMetricsMode,
  DeviceImportPartialCommitError,
  type DeviceImportPreview,
  type DeviceImportPreviewTarget,
  type DeviceImportResult,
  fetchCanvasMapAreas,
  fetchCanvasMaps,
  fetchSNMPProfiles,
  previewDeviceImport,
} from '../api/client';
import type { Area, CanvasMap, SNMPProfile } from '../types/api';

interface DeviceImportPanelProps {
  canReadCredentials: boolean;
  onOpenMap?: (map: CanvasMap) => void;
}

type PendingAction = 'preview' | 'commit' | null;

const metricsModes: Array<{
  value: DeviceImportMetricsMode;
  label: string;
  description: string;
  requiresCredentials: boolean;
}> = [
  {
    value: 'prometheus',
    label: 'Prometheus',
    description: 'Use each target as the Prometheus instance value.',
    requiresCredentials: false,
  },
  {
    value: 'prometheus_snmp_fallback',
    label: 'Prometheus with SNMP fallback',
    description: 'Prefer Prometheus and use the selected SNMP Profile as fallback.',
    requiresCredentials: true,
  },
  {
    value: 'snmp',
    label: 'SNMP',
    description: 'Poll imported addresses directly with the selected SNMP Profile.',
    requiresCredentials: true,
  },
];

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

function profileVersion(profile: SNMPProfile): string {
  return `v${profile.snmp.version.replace(/^v/i, '')}`;
}

function statusLabel(status: DeviceImportPreviewTarget['status'] | DeviceImportResult['status']) {
  switch (status) {
    case 'ready':
      return 'Ready';
    case 'invalid':
      return 'Invalid';
    case 'skipped_duplicate_in_file':
      return 'Skipped duplicate in file';
    case 'skipped_existing':
      return 'Skipped existing';
    case 'created':
      return 'Created';
    case 'failed':
      return 'Failed';
    case 'not_processed':
      return 'Not processed';
  }
}

function statusClass(status: DeviceImportPreviewTarget['status'] | DeviceImportResult['status']) {
  switch (status) {
    case 'ready':
    case 'created':
      return 'border-success/40 bg-success/10 text-success';
    case 'invalid':
    case 'failed':
      return 'border-warning/40 bg-warning/10 text-warning';
    case 'not_processed':
      return 'border-outline-strong bg-surface-container-high text-on-bg';
    default:
      return 'border-outline-subtle bg-surface-container text-on-bg-secondary';
  }
}

function SummaryGrid({
  testId,
  items,
}: {
  testId: string;
  items: Array<{ label: string; value: number }>;
}) {
  return (
    <dl data-testid={testId} className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
      {items.map((item) => (
        <div
          key={item.label}
          className="rounded-lg border border-outline-subtle bg-surface-container px-4 py-3"
        >
          <dt className="text-xs font-medium text-on-bg-secondary">{item.label}</dt>
          <dd className="mt-1 text-2xl font-semibold tabular-nums text-on-bg">{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}

function Diagnostics({ diagnostics }: { diagnostics: DeviceImportDiagnostic[] }) {
  if (diagnostics.length === 0) {
    return null;
  }
  return (
    <section
      aria-labelledby="device-import-diagnostics-title"
      className="rounded-lg border border-warning/40 bg-warning/10 p-4"
    >
      <h3 id="device-import-diagnostics-title" className="text-sm font-semibold text-warning">
        File diagnostics
      </h3>
      <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-on-bg">
        {diagnostics.map((diagnostic) => (
          <li key={`${diagnostic.group_index}:${diagnostic.message}`}>
            Group {diagnostic.group_index + 1}: <span>{diagnostic.message}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function TargetTable({ targets }: { targets: DeviceImportPreviewTarget[] }) {
  return (
    <div className="overflow-x-auto rounded-lg border border-outline-subtle bg-surface">
      <table className="w-full min-w-[48rem] text-sm">
        <thead className="bg-surface-container text-left text-xs uppercase text-on-bg-secondary">
          <tr>
            <th className="px-3 py-2 font-semibold">Location</th>
            <th className="px-3 py-2 font-semibold">Target</th>
            <th className="px-3 py-2 font-semibold">Address</th>
            <th className="px-3 py-2 font-semibold">Status</th>
            <th className="px-3 py-2 font-semibold">Detail</th>
          </tr>
        </thead>
        <tbody>
          {targets.map((target) => (
            <tr
              key={`${target.group_index}:${target.item_index}`}
              data-testid="device-import-preview-row"
              className="border-t border-outline-subtle"
            >
              <td className="px-3 py-3 text-xs tabular-nums text-on-bg-secondary">
                {target.group_index + 1}:{target.item_index + 1}
              </td>
              <td data-testid="target-value" className="px-3 py-3 font-mono text-xs text-on-bg">
                {target.target}
              </td>
              <td className="px-3 py-3 font-mono text-xs text-on-bg-secondary">
                {target.address || '—'}
              </td>
              <td className="px-3 py-3">
                <span
                  className={`inline-flex rounded-full border px-2 py-1 text-xs font-medium ${statusClass(
                    target.status,
                  )}`}
                >
                  {statusLabel(target.status)}
                </span>
              </td>
              <td className="px-3 py-3 text-on-bg-secondary">{target.message || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ResultTable({ results }: { results: DeviceImportResult[] }) {
  return (
    <div className="overflow-x-auto rounded-lg border border-outline-subtle bg-surface">
      <table className="w-full min-w-[48rem] text-sm">
        <thead className="bg-surface-container text-left text-xs uppercase text-on-bg-secondary">
          <tr>
            <th className="px-3 py-2 font-semibold">Location</th>
            <th className="px-3 py-2 font-semibold">Target</th>
            <th className="px-3 py-2 font-semibold">Address</th>
            <th className="px-3 py-2 font-semibold">Status</th>
            <th className="px-3 py-2 font-semibold">Detail</th>
          </tr>
        </thead>
        <tbody>
          {results.map((result) => (
            <tr
              key={`${result.group_index}:${result.item_index}`}
              data-testid="device-import-result-row"
              className="border-t border-outline-subtle"
            >
              <td className="px-3 py-3 text-xs tabular-nums text-on-bg-secondary">
                {result.group_index + 1}:{result.item_index + 1}
              </td>
              <td className="px-3 py-3 font-mono text-xs text-on-bg">{result.target}</td>
              <td className="px-3 py-3 font-mono text-xs text-on-bg-secondary">
                {result.address || '—'}
              </td>
              <td className="px-3 py-3">
                <span
                  data-testid="result-status"
                  className={`inline-flex rounded-full border px-2 py-1 text-xs font-medium ${statusClass(
                    result.status,
                  )}`}
                >
                  {statusLabel(result.status)}
                </span>
              </td>
              <td className="px-3 py-3 text-on-bg-secondary">{result.message || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** Renders the permission-aware node import configuration, preview, and result stages. */
export function DeviceImportPanel({ canReadCredentials, onOpenMap }: DeviceImportPanelProps) {
  const [file, setFile] = useState<File | null>(null);
  const [fileInputRevision, setFileInputRevision] = useState(0);
  const [metricsMode, setMetricsMode] = useState<DeviceImportMetricsMode>('prometheus');
  const [mapId, setMapId] = useState('');
  const [areaId, setAreaId] = useState('');
  const [snmpProfileId, setSNMPProfileId] = useState('');
  const [maps, setMaps] = useState<CanvasMap[]>([]);
  const [areas, setAreas] = useState<Area[]>([]);
  const [profiles, setProfiles] = useState<SNMPProfile[]>([]);
  const [mapsLoading, setMapsLoading] = useState(true);
  const [areasLoading, setAreasLoading] = useState(false);
  const [profilesLoading, setProfilesLoading] = useState(canReadCredentials);
  const [resourceError, setResourceError] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [preview, setPreview] = useState<DeviceImportPreview | null>(null);
  const [result, setResult] = useState<DeviceImportCommitResult | null>(null);
  const [pendingAction, setPendingAction] = useState<PendingAction>(null);
  const requestInFlight = useRef(false);
  const configurationRevision = useRef(0);

  const requiresSNMPProfile = metricsMode !== 'prometheus';
  const primaryMapId = maps.find((map) => map.is_default)?.id ?? '';
  const selectedMap = maps.find((map) => map.id === mapId) ?? null;

  const invalidateOutcome = useCallback(() => {
    configurationRevision.current += 1;
    setPreview(null);
    setResult(null);
    setError(null);
  }, []);

  useEffect(() => {
    let ignore = false;
    setMapsLoading(true);
    setResourceError(null);
    void fetchCanvasMaps()
      .then((nextMaps) => {
        if (ignore) return;
        setMaps(nextMaps);
        setMapId(nextMaps.find((map) => map.is_default)?.id ?? '');
      })
      .catch((loadError) => {
        if (ignore) return;
        setMaps([]);
        setMapId('');
        setResourceError(errorMessage(loadError, 'Failed to load saved maps'));
      })
      .finally(() => {
        if (!ignore) setMapsLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, []);

  useEffect(() => {
    if (!canReadCredentials) {
      setProfiles([]);
      setProfilesLoading(false);
      return;
    }
    let ignore = false;
    setProfilesLoading(true);
    void fetchSNMPProfiles()
      .then((nextProfiles) => {
        if (!ignore) setProfiles(nextProfiles);
      })
      .catch((loadError) => {
        if (ignore) return;
        setProfiles([]);
        setResourceError(errorMessage(loadError, 'Failed to load SNMP Profiles'));
      })
      .finally(() => {
        if (!ignore) setProfilesLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, [canReadCredentials]);

  useEffect(() => {
    if (!mapId) {
      setAreas([]);
      setAreasLoading(false);
      return;
    }
    let ignore = false;
    setAreasLoading(true);
    void fetchCanvasMapAreas(mapId)
      .then((nextAreas) => {
        if (!ignore) setAreas(nextAreas);
      })
      .catch((loadError) => {
        if (ignore) return;
        setAreas([]);
        setResourceError(errorMessage(loadError, 'Failed to load map areas'));
      })
      .finally(() => {
        if (!ignore) setAreasLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, [mapId]);

  const configuration = useMemo<DeviceImportConfiguration | null>(() => {
    if (!file || !mapId || (requiresSNMPProfile && (!canReadCredentials || !snmpProfileId))) {
      return null;
    }
    return {
      file,
      metrics_mode: metricsMode,
      map_id: mapId,
      ...(areaId ? { area_id: areaId } : {}),
      ...(requiresSNMPProfile ? { snmp_profile_id: snmpProfileId } : {}),
    };
  }, [areaId, canReadCredentials, file, mapId, metricsMode, requiresSNMPProfile, snmpProfileId]);

  function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    invalidateOutcome();
    setFile(event.target.files?.[0] ?? null);
  }

  function handleModeChange(nextMode: DeviceImportMetricsMode) {
    invalidateOutcome();
    setMetricsMode(nextMode);
  }

  function handleMapChange(nextMapId: string) {
    invalidateOutcome();
    setMapId(nextMapId);
    setAreaId('');
    setAreas([]);
  }

  function handleAreaChange(nextAreaId: string) {
    invalidateOutcome();
    setAreaId(nextAreaId);
  }

  function handleProfileChange(nextProfileId: string) {
    invalidateOutcome();
    setSNMPProfileId(nextProfileId);
  }

  async function handlePreview(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!configuration || requestInFlight.current) {
      return;
    }
    requestInFlight.current = true;
    const revision = configurationRevision.current;
    setPendingAction('preview');
    setError(null);
    try {
      const nextPreview = await previewDeviceImport(configuration);
      if (revision === configurationRevision.current) {
        setPreview(nextPreview);
        setResult(null);
      }
    } catch (previewError) {
      if (revision === configurationRevision.current) {
        setError(errorMessage(previewError, 'Failed to preview node import'));
      }
    } finally {
      requestInFlight.current = false;
      setPendingAction(null);
    }
  }

  async function handleCommit() {
    if (!configuration || !preview || preview.summary.ready < 1 || requestInFlight.current) {
      return;
    }
    requestInFlight.current = true;
    const revision = configurationRevision.current;
    setPendingAction('commit');
    setError(null);
    try {
      const nextResult = await commitDeviceImport(configuration, preview.file_digest);
      if (revision === configurationRevision.current) {
        setResult(nextResult);
      }
    } catch (commitError) {
      if (revision === configurationRevision.current) {
        if (commitError instanceof DeviceImportPartialCommitError) {
          setResult(commitError.result);
        }
        setError(errorMessage(commitError, 'Failed to commit node import'));
      }
    } finally {
      requestInFlight.current = false;
      setPendingAction(null);
    }
  }

  function handleBack() {
    invalidateOutcome();
  }

  function handleRetry() {
    invalidateOutcome();
  }

  function handleReset() {
    configurationRevision.current += 1;
    setFile(null);
    setFileInputRevision((revision) => revision + 1);
    setMetricsMode('prometheus');
    setMapId(primaryMapId);
    setAreaId('');
    setSNMPProfileId('');
    setPreview(null);
    setResult(null);
    setError(null);
  }

  return (
    <section aria-labelledby="device-import-title" className="flex flex-col gap-4">
      <div className="rounded-lg border border-outline-subtle bg-surface p-4">
        <h2 id="device-import-title" className="text-lg font-semibold text-on-bg">
          One-time node import
        </h2>
        <p className="mt-1 text-sm text-on-bg-secondary">
          Upload a Prometheus file-SD YAML document. Only values in targets are imported; every
          label is ignored.
        </p>
      </div>

      {(resourceError || error) && (
        <div
          role="alert"
          className="rounded-lg border border-warning/40 bg-warning/10 px-4 py-3 text-sm text-warning"
        >
          {error ?? resourceError}
        </div>
      )}

      {!preview && !result && (
        <form
          onSubmit={handlePreview}
          noValidate
          aria-busy={pendingAction === 'preview'}
          className="flex flex-col gap-5 rounded-lg border border-outline-subtle bg-surface p-4"
        >
          <fieldset className="flex flex-col gap-3" disabled={pendingAction !== null}>
            <legend className="text-sm font-semibold text-on-bg">Metrics mode</legend>
            <div className="grid gap-3 lg:grid-cols-3">
              {metricsModes.map((mode) => {
                const disabled = mode.requiresCredentials && !canReadCredentials;
                return (
                  <label
                    key={mode.value}
                    className={`rounded-lg border p-3 ${
                      metricsMode === mode.value
                        ? 'border-primary bg-primary/10'
                        : 'border-outline-subtle bg-surface-container'
                    } ${disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'}`}
                  >
                    <span className="flex items-center gap-2 text-sm font-medium text-on-bg">
                      <input
                        type="radio"
                        name="device-import-mode"
                        value={mode.value}
                        aria-label={mode.label}
                        checked={metricsMode === mode.value}
                        disabled={disabled}
                        onChange={() => handleModeChange(mode.value)}
                      />
                      {mode.label}
                    </span>
                    <span className="mt-1 block text-xs text-on-bg-secondary">
                      {mode.description}
                    </span>
                  </label>
                );
              })}
            </div>
            {!canReadCredentials && (
              <p className="text-xs text-on-bg-secondary">
                The credentials:read permission is required to select SNMP import modes.
              </p>
            )}
          </fieldset>

          <div className="grid gap-4 lg:grid-cols-2">
            <label className="flex flex-col gap-1 text-sm font-medium text-on-bg">
              Prometheus file-SD YAML
              <input
                key={fileInputRevision}
                type="file"
                aria-label="Prometheus file-SD YAML"
                accept=".yml,.yaml,application/yaml,text/yaml"
                required
                disabled={pendingAction !== null}
                onChange={handleFileChange}
                className="rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm text-on-bg file:mr-3 file:rounded-md file:border-0 file:bg-surface-container-high file:px-3 file:py-1 file:text-on-bg"
              />
              <span className="text-xs font-normal text-on-bg-secondary">
                Maximum 2 MiB and 5,000 target entries.
              </span>
            </label>

            <label className="flex flex-col gap-1 text-sm font-medium text-on-bg">
              Destination map
              <select
                required
                aria-label="Destination map"
                value={mapId}
                disabled={mapsLoading || pendingAction !== null}
                onChange={(event) => handleMapChange(event.target.value)}
                className="rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm text-on-bg outline-none focus:border-primary disabled:opacity-60"
              >
                <option value="">
                  {mapsLoading ? 'Loading saved maps…' : 'Select a saved map'}
                </option>
                {maps.map((map) => (
                  <option key={map.id} value={map.id}>
                    {map.name}
                    {map.is_default ? ' (primary)' : ''}
                  </option>
                ))}
              </select>
            </label>

            <label className="flex flex-col gap-1 text-sm font-medium text-on-bg">
              Map area (optional)
              <select
                value={areaId}
                aria-label="Map area (optional)"
                disabled={!mapId || areasLoading || pendingAction !== null}
                onChange={(event) => handleAreaChange(event.target.value)}
                className="rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm text-on-bg outline-none focus:border-primary disabled:opacity-60"
              >
                <option value="">
                  {areasLoading ? 'Loading map areas…' : 'No area assignment'}
                </option>
                {areas.map((area) => (
                  <option key={area.id} value={area.id}>
                    {area.name}
                  </option>
                ))}
              </select>
            </label>

            {requiresSNMPProfile && (
              <label className="flex flex-col gap-1 text-sm font-medium text-on-bg">
                SNMP Profile
                <select
                  required
                  aria-label="SNMP Profile"
                  value={snmpProfileId}
                  disabled={profilesLoading || pendingAction !== null || !canReadCredentials}
                  onChange={(event) => handleProfileChange(event.target.value)}
                  className="rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm text-on-bg outline-none focus:border-primary disabled:opacity-60"
                >
                  <option value="">
                    {profilesLoading ? 'Loading redacted profiles…' : 'Select an SNMP Profile'}
                  </option>
                  {profiles.map((profile) => (
                    <option key={profile.id} value={profile.id}>
                      {profile.name} ({profileVersion(profile)})
                    </option>
                  ))}
                </select>
                <span className="text-xs font-normal text-on-bg-secondary">
                  Secret values remain on the server and are never revealed by this workflow.
                </span>
              </label>
            )}
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-on-bg-secondary">
              Existing addresses are skipped and never modified or remapped.
            </p>
            <button
              type="submit"
              disabled={!configuration || pendingAction !== null}
              className="rounded-md bg-primary px-4 py-2 text-sm font-semibold text-on-primary disabled:cursor-not-allowed disabled:opacity-60"
            >
              {pendingAction === 'preview' ? 'Previewing…' : 'Preview import'}
            </button>
          </div>
        </form>
      )}

      {preview && !result && (
        <section aria-labelledby="device-import-preview-title" className="flex flex-col gap-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 id="device-import-preview-title" className="text-lg font-semibold text-on-bg">
                Import preview
              </h3>
              <p className="text-sm text-on-bg-secondary">
                Review the ordered targets before creating any nodes.
              </p>
            </div>
            <span className="rounded-full border border-outline-subtle bg-surface-container px-3 py-1 font-mono text-xs text-on-bg-secondary">
              {preview.file_digest}
            </span>
          </div>
          <SummaryGrid
            testId="device-import-preview-summary"
            items={[
              { label: 'Total', value: preview.summary.total },
              { label: 'Ready', value: preview.summary.ready },
              { label: 'Invalid', value: preview.summary.invalid },
              { label: 'Existing', value: preview.summary.skipped_existing },
              { label: 'Duplicates', value: preview.summary.skipped_duplicate_in_file },
            ]}
          />
          <Diagnostics diagnostics={preview.diagnostics} />
          <TargetTable targets={preview.targets} />
          {preview.summary.ready === 0 && (
            <p role="status" className="text-sm text-on-bg-secondary">
              No ready targets can be committed. Return to the configuration or reset the import.
            </p>
          )}
          <div className="flex flex-wrap justify-end gap-2">
            <button
              type="button"
              disabled={pendingAction !== null}
              onClick={handleBack}
              className="rounded-md border border-outline-subtle bg-surface-container px-4 py-2 text-sm text-on-bg hover:bg-surface-container-high disabled:opacity-60"
            >
              Back to configuration
            </button>
            <button
              type="button"
              disabled={!configuration || preview.summary.ready < 1 || pendingAction !== null}
              onClick={() => void handleCommit()}
              className="rounded-md bg-primary px-4 py-2 text-sm font-semibold text-on-primary disabled:cursor-not-allowed disabled:opacity-60"
            >
              {pendingAction === 'commit' ? 'Committing…' : 'Commit import'}
            </button>
          </div>
        </section>
      )}

      {result && (
        <section aria-labelledby="device-import-result-title" className="flex flex-col gap-4">
          <div
            role="status"
            className={`rounded-lg border px-4 py-3 ${
              result.incomplete
                ? 'border-warning/40 bg-warning/10 text-warning'
                : 'border-success/40 bg-success/10 text-success'
            }`}
          >
            <h3 id="device-import-result-title" className="font-semibold">
              {result.incomplete ? 'Import incomplete' : 'Import completed'}
            </h3>
            <p className="mt-1 text-sm">
              Every row below is the final outcome for that uploaded target.
            </p>
          </div>
          <SummaryGrid
            testId="device-import-result-summary"
            items={[
              { label: 'Total', value: result.summary.total },
              { label: 'Created', value: result.summary.created },
              { label: 'Skipped', value: result.summary.skipped },
              { label: 'Failed', value: result.summary.failed },
              { label: 'Not processed', value: result.summary.not_processed },
            ]}
          />
          <Diagnostics diagnostics={result.diagnostics} />
          <ResultTable results={result.results} />
          <div className="flex flex-wrap justify-end gap-2">
            <button
              type="button"
              onClick={handleRetry}
              className="rounded-md border border-outline-subtle bg-surface-container px-4 py-2 text-sm text-on-bg hover:bg-surface-container-high"
            >
              Retry import
            </button>
            <button
              type="button"
              onClick={handleReset}
              className="rounded-md border border-outline-subtle bg-surface-container px-4 py-2 text-sm text-on-bg hover:bg-surface-container-high"
            >
              Reset import
            </button>
            <button
              type="button"
              disabled={!selectedMap || !onOpenMap}
              onClick={() => {
                if (selectedMap) onOpenMap?.(selectedMap);
              }}
              className="rounded-md bg-primary px-4 py-2 text-sm font-semibold text-on-primary disabled:cursor-not-allowed disabled:opacity-60"
            >
              Open destination map
            </button>
          </div>
        </section>
      )}
    </section>
  );
}
