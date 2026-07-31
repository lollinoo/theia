import type { DeviceImportTopologyRunSnapshot } from '../../api/deviceImport';
import type {
  DeviceImportTopologyPhase,
  DeviceImportTopologyProgress,
} from './useDeviceImportTopologyRun';

interface DeviceImportTopologyOverlayProps {
  snapshot: DeviceImportTopologyRunSnapshot | null;
  phase: DeviceImportTopologyPhase | null;
  progress: DeviceImportTopologyProgress;
  applyingLayout: boolean;
  error: string | null;
  deviceNames: Map<string, string>;
  onContinue: () => void;
  onRetry: (deviceIDs: string[]) => void;
  onConfigureDevice: (deviceID: string) => void;
  onCreateManualLink: () => void;
  onRefresh: () => void;
}

const phases: Array<{ id: Exclude<DeviceImportTopologyPhase, 'complete'>; label: string }> = [
  { id: 'discovery', label: 'Discovery' },
  { id: 'links', label: 'Link creation' },
  { id: 'layout', label: 'Layout' },
];

function phaseTitle(phase: DeviceImportTopologyPhase | null, applyingLayout: boolean): string {
  switch (phase) {
    case 'discovery':
      return 'Discovering LLDP/CDP neighbors';
    case 'links':
      return 'Creating automatic links';
    case 'layout':
      return applyingLayout ? 'Organizing map layout' : 'Preparing map layout';
    default:
      return 'Topology Bootstrap-Once';
  }
}

/** Shows durable Bootstrap-Once progress without preventing use of the map. */
export function DeviceImportTopologyOverlay({
  snapshot,
  phase,
  progress,
  applyingLayout,
  error,
  deviceNames,
  onContinue,
  onRetry,
  onConfigureDevice,
  onCreateManualLink,
  onRefresh,
}: DeviceImportTopologyOverlayProps) {
  if (!snapshot || phase === null) {
    if (!error) return null;
    return (
      <section
        role="alert"
        aria-label="Topology Bootstrap-Once status error"
        className="pointer-events-auto absolute top-20 left-1/2 z-[60] flex w-[min(94vw,34rem)] -translate-x-1/2 items-center justify-between gap-4 rounded-2xl border border-status-down/35 bg-surface/95 px-4 py-3 shadow-floating backdrop-blur-md"
      >
        <div className="min-w-0">
          <p className="text-sm font-medium text-status-down">Topology status unavailable</p>
          <p className="mt-0.5 text-xs text-on-bg-secondary">{error}</p>
        </div>
        <button
          type="button"
          onClick={onRefresh}
          className="flex-none rounded-full border border-status-down/40 px-3 py-2 text-xs font-medium text-status-down hover:bg-status-down/10"
        >
          Try again
        </button>
      </section>
    );
  }
  if (phase === 'complete') return null;

  const runFailed = snapshot.run.state === 'failed';

  if (snapshot.run.backgrounded) {
    return (
      <div
        data-testid="topology-bootstrap-background-status"
        role="status"
        className="pointer-events-none absolute top-20 left-1/2 z-[55] flex -translate-x-1/2 items-center gap-2.5 rounded-full border border-primary/30 bg-surface-container-high/95 px-4 py-2.5 text-sm text-on-bg-secondary shadow-floating backdrop-blur-sm"
      >
        <span className="h-2 w-2 animate-pulse rounded-full bg-primary" aria-hidden="true" />
        <span>Topology discovery continues in the background</span>
        <span className="font-medium text-primary">
          {progress.completed}/{progress.total}
        </span>
      </div>
    );
  }

  const phaseIndex = Math.max(
    0,
    phases.findIndex((candidate) => candidate.id === phase),
  );
  const issueItems = snapshot.items.filter(
    (item) => item.state === 'warning' || item.state === 'failed',
  );
  const issueIDs = issueItems.map((item) => item.device_id);
  const primaryIssue = issueItems[0];
  const progressPercent =
    progress.total === 0 ? 0 : Math.round((progress.completed / progress.total) * 100);

  return (
    <section
      data-testid="topology-bootstrap-overlay"
      aria-label="Topology Bootstrap-Once progress"
      aria-live="polite"
      className="pointer-events-auto absolute top-20 left-1/2 z-[60] w-[min(94vw,42rem)] -translate-x-1/2 rounded-[24px] border border-outline bg-surface/95 p-5 shadow-canvas backdrop-blur-md"
    >
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.22em] text-primary">
            Bootstrap-Once
          </p>
          <h2 className="mt-1.5 text-lg font-semibold tracking-tight text-on-bg">
            {runFailed
              ? 'Automatic link creation needs attention'
              : phaseTitle(phase, applyingLayout)}
          </h2>
          <p className="mt-1 text-sm text-on-bg-secondary">
            {progress.completed} of {progress.total} devices checked
          </p>
        </div>
        <div
          className="flex h-10 w-10 flex-none items-center justify-center rounded-full bg-primary/10"
          aria-hidden="true"
        >
          <span
            className={
              runFailed
                ? 'h-5 w-5 rounded-full border-2 border-warning bg-warning/20'
                : 'h-5 w-5 animate-spin rounded-full border-2 border-primary/20 border-t-primary'
            }
          />
        </div>
      </div>

      <ol className="mt-4 grid grid-cols-3 gap-2" aria-label="Bootstrap phases">
        {phases.map((step, index) => {
          const active = index === phaseIndex;
          const complete = index < phaseIndex;
          return (
            <li
              key={step.id}
              aria-current={active ? 'step' : undefined}
              className={`rounded-xl border px-3 py-2 text-center text-xs font-medium ${
                active
                  ? 'border-primary/45 bg-primary/10 text-primary'
                  : complete
                    ? 'border-status-up/30 bg-status-up/10 text-status-up'
                    : 'border-outline-subtle bg-surface-container text-on-bg-secondary'
              }`}
            >
              {step.label}
            </li>
          );
        })}
      </ol>

      <div className="mt-4 h-1.5 overflow-hidden rounded-full bg-surface-container-high">
        <div
          className="h-full rounded-full bg-primary transition-[width] duration-300"
          style={{ width: `${progressPercent}%` }}
        />
      </div>

      <div className="mt-4 grid grid-cols-3 gap-2 text-center">
        <div className="rounded-xl bg-surface-container px-2 py-2">
          <p className="text-base font-semibold text-on-bg">{progress.neighbors}</p>
          <p className="text-[11px] text-on-bg-secondary">Neighbors</p>
        </div>
        <div className="rounded-xl bg-surface-container px-2 py-2">
          <p className="text-base font-semibold text-on-bg">{progress.linksCreated}</p>
          <p className="text-[11px] text-on-bg-secondary">Links created</p>
        </div>
        <div className="rounded-xl bg-surface-container px-2 py-2">
          <p
            className={
              progress.unresolved > 0
                ? 'text-base font-semibold text-warning'
                : 'text-base font-semibold text-on-bg'
            }
          >
            {progress.unresolved}
          </p>
          <p className="text-[11px] text-on-bg-secondary">Needs attention</p>
        </div>
      </div>

      {primaryIssue && (
        <div className="mt-4 rounded-xl border border-warning/30 bg-warning/5 p-3">
          <div className="flex items-start gap-2">
            <span className="mt-1 h-2 w-2 flex-none rounded-full bg-warning" aria-hidden="true" />
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-on-bg">
                {deviceNames.get(primaryIssue.device_id) ?? primaryIssue.device_id}
              </p>
              <p className="mt-0.5 text-xs leading-5 text-on-bg-secondary">
                {primaryIssue.message ?? 'Automatic topology discovery needs verification.'}
              </p>
              {issueItems.length > 1 && (
                <p className="mt-1 text-[11px] text-warning">
                  +{issueItems.length - 1} more affected device{issueItems.length === 2 ? '' : 's'}
                </p>
              )}
            </div>
          </div>
        </div>
      )}

      {runFailed && snapshot.run.failure_message && (
        <div
          className="mt-4 rounded-xl border border-status-down/35 bg-status-down/5 p-3"
          role="alert"
        >
          <p className="text-sm font-medium text-status-down">{snapshot.run.failure_message}</p>
          <p className="mt-1 text-xs leading-5 text-on-bg-secondary">
            Automatic reconciliation was retried. You can continue with the imported nodes and
            complete links manually.
          </p>
          {snapshot.run.failure_reference && (
            <p className="mt-1 font-mono text-[11px] text-on-bg-secondary">
              Reference: {snapshot.run.failure_reference}
            </p>
          )}
        </div>
      )}

      {error && (
        <div
          role="alert"
          className="mt-4 flex items-center justify-between gap-3 rounded-xl border border-status-down/35 bg-status-down/5 px-3 py-2.5"
        >
          <p className="text-xs text-status-down">{error}</p>
          <button
            type="button"
            onClick={onRefresh}
            className="flex-none text-xs font-medium text-status-down hover:opacity-75"
          >
            Try again
          </button>
        </div>
      )}

      <div className="mt-4 flex flex-wrap items-center justify-end gap-2">
        {primaryIssue && (
          <>
            <button
              type="button"
              onClick={() => onConfigureDevice(primaryIssue.device_id)}
              className="rounded-full border border-outline px-3 py-2 text-xs font-medium text-on-bg-secondary transition-colors hover:bg-surface-container-high hover:text-on-bg"
            >
              Configure device
            </button>
            <button
              type="button"
              onClick={onCreateManualLink}
              className="rounded-full border border-outline px-3 py-2 text-xs font-medium text-on-bg-secondary transition-colors hover:bg-surface-container-high hover:text-on-bg"
            >
              Create link manually
            </button>
            <button
              type="button"
              onClick={() => onRetry(issueIDs)}
              className="rounded-full border border-warning/40 px-3 py-2 text-xs font-medium text-warning transition-colors hover:bg-warning/10"
            >
              Retry affected devices
            </button>
          </>
        )}
        <button
          type="button"
          onClick={onContinue}
          className="rounded-full bg-primary px-4 py-2 text-xs font-semibold text-on-primary transition-opacity hover:opacity-90"
        >
          {runFailed ? 'Continue with manual map' : 'Continue with partial map'}
        </button>
      </div>
    </section>
  );
}
