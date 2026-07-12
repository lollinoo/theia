/**
 * Renders interface stats panel UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import type {
  DeviceInterfacePanelModel,
  InterfaceSectionModel,
  LinkInterfacePanelModel,
} from './panelModels';

interface InterfaceStatsSectionProps {
  section: InterfaceSectionModel;
}

function InterfaceStatsSection({ section }: InterfaceStatsSectionProps) {
  const metricsUnavailable = section.metricsUnavailableMessage !== null;

  return (
    <div
      className={`rounded-xl p-4 space-y-3 transition-colors duration-200 ${metricsUnavailable ? 'bg-status-down/10' : 'bg-surface-high'}`}
    >
      <div>
        <p className="text-[12px] font-medium uppercase text-on-bg-secondary">Device</p>
        <p className="mt-0.5 text-sm font-medium text-on-bg">{section.deviceLabel}</p>
        {section.metricsUnavailableMessage && (
          <p className="mt-1 text-xs font-medium text-status-down">
            {section.metricsUnavailableMessage}
          </p>
        )}
      </div>

      <div>
        <p className="text-[12px] font-medium uppercase text-on-bg-secondary">Interface</p>
        <p className="mt-0.5 text-sm font-mono text-on-bg">{section.ifName}</p>
        {section.interfaceDescription && (
          <p className="text-xs text-on-bg-secondary">{section.interfaceDescription}</p>
        )}
      </div>

      <div className="grid grid-cols-2 gap-3">
        {section.speedLabel && (
          <div>
            <p className="text-[12px] font-medium uppercase text-on-bg-secondary">Speed</p>
            <p className="mt-0.5 font-mono text-[11px] font-semibold text-on-bg">
              {section.speedLabel}
            </p>
          </div>
        )}
        {section.statusLabel && (
          <div>
            <p className="text-[12px] font-medium uppercase text-on-bg-secondary">Status</p>
            <p
              className={`mt-0.5 text-sm ${section.statusTone === 'up' ? 'text-status-up' : section.statusTone === 'down' ? 'text-status-down' : 'text-on-bg-secondary'}`}
            >
              {section.statusLabel}
            </p>
          </div>
        )}
      </div>

      <div className="grid grid-cols-2 gap-3 mt-3 pt-3">
        <div>
          <p className="text-[12px] font-medium uppercase text-on-bg-secondary">TX</p>
          <p
            className={`mt-0.5 font-mono text-[11px] font-semibold ${metricsUnavailable ? 'text-status-down/70' : 'text-on-bg'}`}
          >
            {section.txLabel}
          </p>
        </div>
        <div>
          <p className="text-[12px] font-medium uppercase text-on-bg-secondary">RX</p>
          <p
            className={`mt-0.5 font-mono text-[11px] font-semibold ${metricsUnavailable ? 'text-status-down/70' : 'text-on-bg'}`}
          >
            {section.rxLabel}
          </p>
        </div>
      </div>

      {section.utilizationPct !== null && (
        <div>
          <div className="flex items-center justify-between">
            <p className="text-[12px] font-medium uppercase text-on-bg-secondary">Utilization</p>
            <p className="text-xs font-mono" style={{ color: section.utilizationColor }}>
              {section.utilizationPct}%
            </p>
          </div>
          <div className="mt-1 h-1.5 w-full rounded-full bg-surface">
            <div
              className="h-1.5 rounded-full transition-all duration-500"
              style={{
                width: `${Math.min(section.utilizationPct, 100)}%`,
                backgroundColor: section.utilizationColor,
              }}
            />
          </div>
        </div>
      )}
    </div>
  );
}

interface InterfaceStatsPanelProps {
  model: LinkInterfacePanelModel;
}

function negotiationToneClass(tone: LinkInterfacePanelModel['negotiation']['tone']): string {
  switch (tone) {
    case 'matched':
    case 'up':
      return 'border-status-up/30 bg-status-up/10 text-status-up';
    case 'critical':
      return 'border-status-down/35 bg-status-down/10 text-status-down';
    case 'warning':
      return 'border-warning/35 bg-warning/10 text-warning';
    case 'mismatch':
    case 'partial':
      return 'border-status-probing/30 bg-status-probing/10 text-status-probing';
    default:
      return 'border-outline-subtle bg-surface-high text-on-bg';
  }
}

/** Renders the InterfaceStatsPanel component within the UI component boundary. */
export function InterfaceStatsPanel({ model }: InterfaceStatsPanelProps) {
  return (
    <div className="space-y-3 p-4">
      <div
        className={`rounded-xl border px-4 py-3 transition-colors duration-200 ${negotiationToneClass(model.negotiation.tone)}`}
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="text-[12px] font-medium uppercase text-on-bg-secondary">
              Autonegotiation
            </p>
            <p className="mt-1 text-sm font-semibold">{model.negotiation.summaryLabel}</p>
            <p className="mt-1 text-xs text-on-bg-secondary">{model.negotiation.detailLabel}</p>
          </div>
        </div>

        <div className="mt-3 grid grid-cols-2 gap-3 border-t border-outline-subtle/70 pt-3">
          <div>
            <p className="text-[12px] font-medium uppercase text-on-bg-secondary">Source</p>
            <p className="mt-0.5 font-mono text-[11px] font-semibold text-on-bg">
              {model.negotiation.sourceLabel}
            </p>
          </div>
          <div>
            <p className="text-[12px] font-medium uppercase text-on-bg-secondary">Target</p>
            <p className="mt-0.5 font-mono text-[11px] font-semibold text-on-bg">
              {model.negotiation.targetLabel}
            </p>
          </div>
        </div>
      </div>
      <InterfaceStatsSection section={model.source} />
      <InterfaceStatsSection section={model.target} />
    </div>
  );
}

interface DeviceInterfaceStatsPanelProps {
  model: DeviceInterfacePanelModel;
}

/** Renders the DeviceInterfaceStatsPanel component within the UI component boundary. */
export function DeviceInterfaceStatsPanel({ model }: DeviceInterfaceStatsPanelProps) {
  if (model.loadingInterfaces) {
    return <div className="p-4 text-sm text-on-bg-secondary">Loading interface details...</div>;
  }

  if (!model.loadingInterfaces && model.sections.length === 0) {
    return (
      <div className="p-4 text-sm text-on-bg-secondary">
        No interfaces discovered for this device.
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      {model.sections.map((section) => (
        <InterfaceStatsSection key={section.ifName} section={section} />
      ))}
    </div>
  );
}
