/**
 * Defines settings constants behavior for settings screens.
 * Keeps validation, saved-state display, and defaults close to the controls that use them.
 */
const TIMEZONES = [
  { label: 'UTC', value: 'UTC' },
  { label: 'Europe/London (GMT/BST)', value: 'Europe/London' },
  { label: 'Europe/Paris (CET/CEST)', value: 'Europe/Paris' },
  { label: 'Europe/Berlin (CET/CEST)', value: 'Europe/Berlin' },
  { label: 'Europe/Rome (CET/CEST)', value: 'Europe/Rome' },
  { label: 'Europe/Madrid (CET/CEST)', value: 'Europe/Madrid' },
  { label: 'Europe/Amsterdam (CET/CEST)', value: 'Europe/Amsterdam' },
  { label: 'Europe/Zurich (CET/CEST)', value: 'Europe/Zurich' },
  { label: 'Europe/Vienna (CET/CEST)', value: 'Europe/Vienna' },
  { label: 'Europe/Brussels (CET/CEST)', value: 'Europe/Brussels' },
  { label: 'Europe/Stockholm (CET/CEST)', value: 'Europe/Stockholm' },
  { label: 'Europe/Warsaw (CET/CEST)', value: 'Europe/Warsaw' },
  { label: 'Europe/Prague (CET/CEST)', value: 'Europe/Prague' },
  { label: 'Europe/Helsinki (EET/EEST)', value: 'Europe/Helsinki' },
  { label: 'Europe/Bucharest (EET/EEST)', value: 'Europe/Bucharest' },
  { label: 'Europe/Athens (EET/EEST)', value: 'Europe/Athens' },
  { label: 'Europe/Istanbul (TRT)', value: 'Europe/Istanbul' },
  { label: 'Europe/Moscow (MSK)', value: 'Europe/Moscow' },
  { label: 'Asia/Dubai (GST)', value: 'Asia/Dubai' },
  { label: 'Asia/Kolkata (IST)', value: 'Asia/Kolkata' },
  { label: 'Asia/Singapore (SGT)', value: 'Asia/Singapore' },
  { label: 'Asia/Shanghai (CST)', value: 'Asia/Shanghai' },
  { label: 'Asia/Tokyo (JST)', value: 'Asia/Tokyo' },
  { label: 'Asia/Seoul (KST)', value: 'Asia/Seoul' },
  { label: 'Australia/Sydney (AEST/AEDT)', value: 'Australia/Sydney' },
  { label: 'Pacific/Auckland (NZST/NZDT)', value: 'Pacific/Auckland' },
  { label: 'America/New_York (EST/EDT)', value: 'America/New_York' },
  { label: 'America/Chicago (CST/CDT)', value: 'America/Chicago' },
  { label: 'America/Denver (MST/MDT)', value: 'America/Denver' },
  { label: 'America/Los_Angeles (PST/PDT)', value: 'America/Los_Angeles' },
  { label: 'America/Anchorage (AKST/AKDT)', value: 'America/Anchorage' },
  { label: 'Pacific/Honolulu (HST)', value: 'Pacific/Honolulu' },
  { label: 'America/Sao_Paulo (BRT)', value: 'America/Sao_Paulo' },
  { label: 'America/Argentina/Buenos_Aires (ART)', value: 'America/Argentina/Buenos_Aires' },
  { label: 'America/Toronto (EST/EDT)', value: 'America/Toronto' },
  { label: 'America/Vancouver (PST/PDT)', value: 'America/Vancouver' },
  { label: 'America/Mexico_City (CST/CDT)', value: 'America/Mexico_City' },
  { label: 'Africa/Cairo (EET)', value: 'Africa/Cairo' },
  { label: 'Africa/Johannesburg (SAST)', value: 'Africa/Johannesburg' },
  { label: 'Africa/Lagos (WAT)', value: 'Africa/Lagos' },
];

const POLLING_PRESETS = [
  { label: '15 seconds', value: '15' },
  { label: '30 seconds', value: '30' },
  { label: '60 seconds (default)', value: '60' },
  { label: '2 minutes', value: '120' },
  { label: '5 minutes', value: '300' },
  { label: 'Custom...', value: 'custom' },
];

type WorkerSettingKey =
  | 'polling_essential_workers'
  | 'snmp_worker_pool_performance_size'
  | 'snmp_worker_pool_operational_size'
  | 'snmp_worker_pool_static_size'
  | 'polling_max_workers_per_device'
  | 'polling_max_workers_per_site'
  | 'polling_max_workers_per_subnet'
  | 'polling_max_inflight_per_snmp_profile';

interface WorkerSetting {
  key: WorkerSettingKey;
  label: string;
  defaultValue: string;
  min: number;
  max: number;
}

type SNMPDebugSettingKey =
  | 'snmp_timeout_seconds'
  | 'snmp_retries'
  | 'snmp_performance_counter_timeout_ms'
  | 'snmp_performance_counter_retries'
  | 'polling_essential_timeout_ms'
  | 'polling_essential_retries'
  | 'snmp_worker_pool_size'
  | WorkerSettingKey;

interface SNMPDebugSetting {
  key: SNMPDebugSettingKey;
  label: string;
  defaultValue: string;
  min: number;
  max: number;
  unit?: string;
}

interface SNMPDebugSettingGroup {
  title: string;
  settings: readonly SNMPDebugSetting[];
}

interface WorkerSettingGroup {
  title: string;
  settings: readonly WorkerSetting[];
}

const WORKER_SETTING_GROUPS: readonly WorkerSettingGroup[] = [
  {
    title: 'Worker Pools',
    settings: [
      {
        key: 'polling_essential_workers',
        label: 'Essential Workers',
        defaultValue: '64',
        min: 1,
        max: 256,
      },
      {
        key: 'snmp_worker_pool_performance_size',
        label: 'Performance Pool',
        defaultValue: '3',
        min: 1,
        max: 128,
      },
      {
        key: 'snmp_worker_pool_operational_size',
        label: 'Operational Pool',
        defaultValue: '1',
        min: 1,
        max: 128,
      },
      {
        key: 'snmp_worker_pool_static_size',
        label: 'Static Pool',
        defaultValue: '1',
        min: 1,
        max: 128,
      },
    ],
  },
  {
    title: 'Isolation Limits',
    settings: [
      {
        key: 'polling_max_workers_per_device',
        label: 'Max Workers Per Device',
        defaultValue: '1',
        min: 1,
        max: 32,
      },
      {
        key: 'polling_max_workers_per_site',
        label: 'Max Workers Per Site',
        defaultValue: '16',
        min: 1,
        max: 256,
      },
      {
        key: 'polling_max_workers_per_subnet',
        label: 'Max Workers Per Subnet',
        defaultValue: '8',
        min: 1,
        max: 256,
      },
      {
        key: 'polling_max_inflight_per_snmp_profile',
        label: 'Max Inflight Per SNMP Profile',
        defaultValue: '16',
        min: 1,
        max: 256,
      },
    ],
  },
] as const;

const WORKER_SETTINGS = WORKER_SETTING_GROUPS.flatMap((group) => group.settings);
const SNMP_MAX_REPETITIONS = '25';

const SNMP_DEBUG_SETTING_GROUPS: readonly SNMPDebugSettingGroup[] = [
  {
    title: 'Request Profiles',
    settings: [
      {
        key: 'snmp_timeout_seconds',
        label: 'Background Timeout',
        defaultValue: '10',
        min: 1,
        max: 120,
        unit: 'sec',
      },
      {
        key: 'snmp_retries',
        label: 'Background Retries',
        defaultValue: '2',
        min: 0,
        max: 10,
      },
      {
        key: 'snmp_performance_counter_timeout_ms',
        label: 'Performance Counter Timeout',
        defaultValue: '2000',
        min: 100,
        max: 30000,
        unit: 'ms',
      },
      {
        key: 'snmp_performance_counter_retries',
        label: 'Performance Counter Retries',
        defaultValue: '0',
        min: 0,
        max: 10,
      },
      {
        key: 'polling_essential_timeout_ms',
        label: 'Essential Timeout',
        defaultValue: '1200',
        min: 100,
        max: 30000,
        unit: 'ms',
      },
      {
        key: 'polling_essential_retries',
        label: 'Essential Retries',
        defaultValue: '1',
        min: 0,
        max: 10,
      },
    ],
  },
  {
    title: 'Worker Pools',
    settings: [
      {
        key: 'polling_essential_workers',
        label: 'Essential Workers',
        defaultValue: '64',
        min: 1,
        max: 256,
      },
      {
        key: 'snmp_worker_pool_size',
        label: 'Legacy Total Pool',
        defaultValue: '5',
        min: 1,
        max: 128,
      },
      {
        key: 'snmp_worker_pool_performance_size',
        label: 'Performance Pool',
        defaultValue: '3',
        min: 1,
        max: 128,
      },
      {
        key: 'snmp_worker_pool_operational_size',
        label: 'Operational Pool',
        defaultValue: '1',
        min: 1,
        max: 128,
      },
      {
        key: 'snmp_worker_pool_static_size',
        label: 'Static Pool',
        defaultValue: '1',
        min: 1,
        max: 128,
      },
    ],
  },
  {
    title: 'Isolation Limits',
    settings: [
      {
        key: 'polling_max_workers_per_device',
        label: 'Max Workers Per Device',
        defaultValue: '1',
        min: 1,
        max: 32,
      },
      {
        key: 'polling_max_workers_per_site',
        label: 'Max Workers Per Site',
        defaultValue: '16',
        min: 1,
        max: 256,
      },
      {
        key: 'polling_max_workers_per_subnet',
        label: 'Max Workers Per Subnet',
        defaultValue: '8',
        min: 1,
        max: 256,
      },
      {
        key: 'polling_max_inflight_per_snmp_profile',
        label: 'Max Inflight Per SNMP Profile',
        defaultValue: '16',
        min: 1,
        max: 256,
      },
    ],
  },
] as const;

const SNMP_DEBUG_SETTINGS = SNMP_DEBUG_SETTING_GROUPS.flatMap((group) => group.settings);

function createDefaultWorkerSettings(): Record<WorkerSettingKey, string> {
  const values = {} as Record<WorkerSettingKey, string>;
  for (const setting of WORKER_SETTINGS) {
    values[setting.key] = setting.defaultValue;
  }
  return values;
}

function createDefaultSNMPDebugSettings(): Record<SNMPDebugSettingKey, string> {
  const values = {} as Record<SNMPDebugSettingKey, string>;
  for (const setting of SNMP_DEBUG_SETTINGS) {
    values[setting.key] = setting.defaultValue;
  }
  return values;
}

const DEFAULT_WORKER_SETTINGS = createDefaultWorkerSettings();
const DEFAULT_SNMP_DEBUG_SETTINGS = createDefaultSNMPDebugSettings();
const PRESET_VALUES = new Set(POLLING_PRESETS.map((p) => p.value).filter((v) => v !== 'custom'));

export type {
  SNMPDebugSetting,
  SNMPDebugSettingGroup,
  SNMPDebugSettingKey,
  WorkerSetting,
  WorkerSettingGroup,
  WorkerSettingKey,
};
export {
  DEFAULT_SNMP_DEBUG_SETTINGS,
  DEFAULT_WORKER_SETTINGS,
  POLLING_PRESETS,
  PRESET_VALUES,
  SNMP_DEBUG_SETTING_GROUPS,
  SNMP_DEBUG_SETTINGS,
  SNMP_MAX_REPETITIONS,
  TIMEZONES,
  WORKER_SETTING_GROUPS,
  WORKER_SETTINGS,
};
