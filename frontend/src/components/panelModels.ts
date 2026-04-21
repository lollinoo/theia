import type { AlertDTO, RuntimeReason } from '../types/metrics';

export interface AlertsPanelAlertModel {
  deviceId: string;
  deviceLabel: string;
  alertName: string;
  severity: AlertDTO['severity'];
  state: AlertDTO['state'];
  summary: string;
}

export interface AlertsPanelModel {
  activeAlertCount: number;
  firingAlerts: AlertsPanelAlertModel[];
  resolvedAlerts: AlertsPanelAlertModel[];
  prometheusDiagnostics: {
    title: string;
    detail: string;
  } | null;
}

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

export interface DeviceInterfacePanelModel {
  deviceId: string;
  deviceLabel: string;
  loadingInterfaces: boolean;
  sections: InterfaceSectionModel[];
}

export interface LinkNegotiationModel {
  sourceLabel: string;
  targetLabel: string;
  summaryLabel: string;
  detailLabel: string;
  tone: 'matched' | 'mismatch' | 'partial' | 'unknown';
}

export interface LinkInterfacePanelModel {
  linkId: string;
  source: InterfaceSectionModel;
  target: InterfaceSectionModel;
  negotiation: LinkNegotiationModel;
}
