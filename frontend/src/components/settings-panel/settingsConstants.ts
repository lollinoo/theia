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

function createDefaultWorkerSettings(): Record<WorkerSettingKey, string> {
  const values = {} as Record<WorkerSettingKey, string>;
  for (const setting of WORKER_SETTINGS) {
    values[setting.key] = setting.defaultValue;
  }
  return values;
}

function createWorkerSavedFlags(): Record<WorkerSettingKey, boolean> {
  const flags = {} as Record<WorkerSettingKey, boolean>;
  for (const setting of WORKER_SETTINGS) {
    flags[setting.key] = false;
  }
  return flags;
}

function createWorkerTimerRefs(): Record<WorkerSettingKey, number | null> {
  const refs = {} as Record<WorkerSettingKey, number | null>;
  for (const setting of WORKER_SETTINGS) {
    refs[setting.key] = null;
  }
  return refs;
}

const DEFAULT_WORKER_SETTINGS = createDefaultWorkerSettings();
const PRESET_VALUES = new Set(POLLING_PRESETS.map((p) => p.value).filter((v) => v !== 'custom'));

export {
  DEFAULT_WORKER_SETTINGS,
  POLLING_PRESETS,
  PRESET_VALUES,
  TIMEZONES,
  WORKER_SETTING_GROUPS,
  WORKER_SETTINGS,
  createWorkerSavedFlags,
  createWorkerTimerRefs,
};
export type { WorkerSetting, WorkerSettingGroup, WorkerSettingKey };
