import type { Area, Device } from '../types/api';
import { DeviceAreasSection } from './device-config/DeviceAreasSection';
import { DeviceCredentialsSection } from './device-config/DeviceCredentialsSection';
import { DeviceDestructiveActionsSection } from './device-config/DeviceDestructiveActionsSection';
import { DeviceGrafanaDashboardSection } from './device-config/DeviceGrafanaDashboardSection';
import { DeviceNetworkSettingsSection } from './device-config/DeviceNetworkSettingsSection';
import { DevicePollingSection } from './device-config/DevicePollingSection';
import { DeviceSnmpTestButton } from './device-config/DeviceSnmpTestButton';
import { DeviceTopologyDiscoverySection } from './device-config/DeviceTopologyDiscoverySection';
import { useDeviceConfigEditor } from './device-config/useDeviceConfigEditor';

interface DeviceConfigPanelProps {
  device: Device;
  readOnly?: boolean;
  areas?: Area[];
  mapContext?: {
    mapId: string;
    mapName: string;
  };
  onDeviceUpdated: (updated: Device) => void;
  onDeviceDeleted: () => void;
  onRemoveFromMap?: (deviceId: string) => void | Promise<void>;
  onSettingsChange?: () => void;
  onWinBoxAvailabilityChange?: (hasWinboxProfile: boolean) => void;
  isVirtual?: boolean;
}

export function DeviceConfigPanel({
  device,
  readOnly = false,
  areas: providedAreas,
  mapContext,
  onDeviceUpdated,
  onDeviceDeleted,
  onRemoveFromMap,
  onSettingsChange,
  onWinBoxAvailabilityChange,
  isVirtual,
}: DeviceConfigPanelProps) {
  const {
    form,
    fieldErrors,
    editLoading,
    editError,
    editSaved,
    usesSNMP,
    deviceConfigSyncKey,
    updateForm,
    updateSnmp,
    updatePrometheus,
    updateVirtual,
    setFieldError,
    handleBlur,
    validateIPField,
    validateDisplayNameField,
    applyProfile,
    handleEditSave,
  } = useDeviceConfigEditor({
    device,
    readOnly,
    mapContext,
    onDeviceUpdated,
    isVirtual,
  });

  return (
    <div className="space-y-6 p-4 transition-colors duration-200">
      {/* Polling Override — physical devices only */}
      {!isVirtual && (
        <DevicePollingSection
          device={device}
          readOnly={readOnly}
          resetKey={deviceConfigSyncKey}
          onDeviceUpdated={onDeviceUpdated}
        />
      )}

      <DeviceTopologyDiscoverySection
        device={device}
        topologyDiscoveryMode={form.topologyDiscoveryMode}
        metricsMode={form.metricsMode}
        ip={form.ip}
        readOnly={readOnly}
        resetKey={deviceConfigSyncKey}
        isVirtual={isVirtual}
        onTopologyDiscoveryModeChange={(topologyDiscoveryMode) =>
          updateForm({ topologyDiscoveryMode })
        }
      />

      <DeviceGrafanaDashboardSection
        device={device}
        readOnly={readOnly}
        isVirtual={isVirtual}
        onSettingsChange={onSettingsChange}
      />

      {/* Edit Device */}
      <form
        noValidate
        onSubmit={(e) => {
          void handleEditSave(e);
        }}
        className="space-y-3"
      >
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Edit Device
          </p>
          <span
            className={`text-xs text-status-up transition-opacity duration-500 ${editSaved ? 'opacity-100' : 'opacity-0'}`}
          >
            Saved
          </span>
        </div>

        {device.sys_name && (
          <div className="bg-surface-high rounded-lg px-3 py-2">
            <p className="text-[10px] uppercase tracking-widest text-on-bg-secondary mb-0.5">
              Auto-discovered Hostname
            </p>
            <p className="text-sm font-mono text-on-bg">{device.sys_name}</p>
          </div>
        )}

        <fieldset disabled={readOnly} className="space-y-3 disabled:opacity-70">
          <input
            type="text"
            value={form.displayName}
            onChange={(e) => {
              updateForm({ displayName: e.target.value });
              setFieldError('displayName', null);
            }}
            onBlur={handleBlur('displayName', () => validateDisplayNameField(form.displayName))}
            placeholder={
              isVirtual
                ? 'e.g. ISP Gateway'
                : device.sys_name
                  ? `Override "${device.sys_name}"`
                  : 'Custom name (optional)'
            }
            required={isVirtual}
            className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:cursor-not-allowed${fieldErrors['displayName'] ? ' border-status-down' : ' border-outline-subtle'}`}
          />
          {fieldErrors['displayName'] && (
            <p className="mt-1 text-xs text-status-down">{fieldErrors['displayName']}</p>
          )}

          <input
            type="text"
            value={form.ip}
            onChange={(e) => {
              updateForm({ ip: e.target.value });
              setFieldError('ip', null);
            }}
            onBlur={handleBlur('ip', () => validateIPField(form.ip))}
            placeholder="IP Address"
            className={`w-full rounded-lg border bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors['ip'] ? ' border-status-down' : ' border-outline-subtle'}`}
          />
          {fieldErrors['ip'] && (
            <p className="mt-1 text-xs text-status-down">{fieldErrors['ip']}</p>
          )}

          <div className="space-y-1">
            <label
              htmlFor="device-notes"
              className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary"
            >
              Device Notes
            </label>
            <textarea
              id="device-notes"
              value={form.notes}
              onChange={(e) => updateForm({ notes: e.target.value })}
              rows={5}
              placeholder="Add internal notes for this device (optional)"
              className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
            />
          </div>

          <DeviceAreasSection
            form={form}
            areas={providedAreas}
            readOnly={readOnly}
            isVirtual={isVirtual}
            mapContext={mapContext}
            onFormChange={updateForm}
            onVirtualChange={updateVirtual}
          />

          <DeviceNetworkSettingsSection
            device={device}
            form={form}
            readOnly={readOnly}
            isVirtual={isVirtual}
            fieldErrors={fieldErrors}
            onFormChange={updateForm}
            onPrometheusChange={updatePrometheus}
            onSnmpChange={updateSnmp}
            onFieldError={setFieldError}
            onSNMPProfileSelected={(profileId) => {
              void applyProfile(profileId);
            }}
          >
            <DeviceCredentialsSection
              device={device}
              readOnly={readOnly}
              isVirtual={isVirtual}
              onWinBoxAvailabilityChange={onWinBoxAvailabilityChange}
            />
          </DeviceNetworkSettingsSection>
        </fieldset>

        {editError && (
          <p className="rounded-lg border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs text-status-down">
            {editError}
          </p>
        )}

        <button
          type="submit"
          disabled={readOnly || editLoading}
          className="w-full rounded-lg bg-surface-high px-4 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
        >
          {editLoading ? 'Saving...' : 'Save Changes'}
        </button>
      </form>

      {/* SNMP Test — visible when metrics source uses SNMP (physical only) */}
      {!isVirtual && usesSNMP && <DeviceSnmpTestButton deviceId={device.id} />}

      <DeviceDestructiveActionsSection
        deviceId={device.id}
        readOnly={readOnly}
        mapContext={mapContext}
        onRemoveFromMap={onRemoveFromMap}
        onDeviceDeleted={onDeviceDeleted}
      />
    </div>
  );
}
