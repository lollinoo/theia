/**
 * Renders panel models UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import type { AlertDTO, RuntimeReason } from '../types/metrics';

/** Describes the alerts panel alert model contract used by the UI component boundary. */
export interface AlertsPanelAlertModel {
  deviceId: string;
  deviceLabel: string;
  alertName: string;
  severity: AlertDTO['severity'];
  state: AlertDTO['state'];
  summary: string;
}

/** Describes the alerts panel model contract used by the UI component boundary. */
export interface AlertsPanelModel {
  activeAlertCount: number;
  firingAlerts: AlertsPanelAlertModel[];
  resolvedAlerts: AlertsPanelAlertModel[];
  prometheusDiagnostics: {
    title: string;
    detail: string;
  } | null;
}

/** Describes the interface section model contract used by the UI component boundary. */
export interface InterfaceSectionModel {
  deviceLabel: string;
  ifName: string;
  interfaceDescription: string | null;
  speedLabel: string | null;
  statusLabel: string | null;
  statusTone: 'up' | 'down' | 'neutral';
  availabilityReason: Exclude<RuntimeReason, 'ok'> | null;
  metricsUnavailableMessage: string | null;
  txLabel: string;
  rxLabel: string;
  utilizationPct: number | null;
  utilizationColor: string;
}

/** Describes the device interface panel model contract used by the UI component boundary. */
export interface DeviceInterfacePanelModel {
  deviceId: string;
  deviceLabel: string;
  loadingInterfaces: boolean;
  sections: InterfaceSectionModel[];
}

/** Describes the link negotiation model contract used by the UI component boundary. */
export interface LinkNegotiationModel {
  sourceLabel: string;
  targetLabel: string;
  summaryLabel: string;
  detailLabel: string;
  tone: 'matched' | 'mismatch' | 'partial' | 'unknown' | 'up' | 'warning' | 'critical';
}

/** Describes the link interface panel model contract used by the UI component boundary. */
export interface LinkInterfacePanelModel {
  linkId: string;
  source: InterfaceSectionModel;
  target: InterfaceSectionModel;
  negotiation: LinkNegotiationModel;
}
