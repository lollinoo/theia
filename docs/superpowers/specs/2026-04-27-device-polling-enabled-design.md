# Device Polling Enable Switch Design

## Context

Theia already has a polling scheduler that builds recurring tasks from the device inventory and removes tasks for devices that are no longer schedulable. Operators need a per-device control on the map to stop backend polling for temporarily unreachable or intentionally unmonitored devices. The goal is to reduce API, SNMP, and websocket churn without changing whether a device is user-managed.

## Requirements

- Add a per-device `polling_enabled` flag.
- Default the flag to `true` for existing and newly created devices.
- When `polling_enabled` is `false`, the backend must not enqueue recurring polling tasks for that device.
- Disabling polling must cover all recurring scheduler work for the device: essential, performance, operational, static, and recurring topology/bootstrap scheduler tasks.
- Manual user actions such as SNMP tests, explicit probes, and "Run Topology Discovery Now" remain available.
- The UI must expose the setting as a switch on the map device configuration surface.
- The existing `managed` field keeps its current meaning and is not reused for this feature.

## Architecture

Add `PollingEnabled bool` to `domain.Device`, serialized as `polling_enabled`. SQLite and Postgres migrations add a non-null boolean/integer column with a default enabled value. Repository create, update, get, and list paths read and write the field. Service update handling accepts the optional field independently from `Managed` and `PollIntervalOverride`.

The device API includes `polling_enabled` in JSON:API responses and accepts it in `PUT /api/v1/devices/{id}`. Missing values in older clients are treated as no change for updates and enabled for stored devices that predate the column.

## Scheduler Behavior

`Scheduler.refreshDevices` treats a device as schedulable only when it is managed, has polling enabled, and passes existing virtual-device rules. Disabled devices are omitted from the `seen` set. Existing scheduler cleanup then removes heap and ready queue entries or marks in-flight entries disabled so they are not rescheduled after completion.

Capacity and health calculations use only devices that are eligible for recurring essential polling. This keeps worker demand warnings aligned with the reduced queue after devices are suspended.

Manual flows do not consult `polling_enabled`. A user can still test credentials, probe, or run topology discovery explicitly because those operations are user-triggered diagnostics, not recurring monitoring.

## Frontend Behavior

The TypeScript `Device` type and parser include `polling_enabled` with a fallback of `true` when older payloads omit the field. `DeviceConfigPanel` adds a switch in the polling section.

When the switch is enabled, the current polling override controls remain available. When disabled, the UI updates the device with `{ polling_enabled: false }`, shows that continuous polling is suspended, and disables the cadence override controls because they have no effect until polling is re-enabled. Re-enabling sends `{ polling_enabled: true }` and restores the existing override selection without clearing it.

## Error Handling

The switch uses the existing update-device error handling pattern: validation and server errors render inline in the polling section, and successful saves show the existing short "Saved" feedback. Backend validation only accepts boolean JSON values for `polling_enabled`.

If a recurring task is already running when polling is disabled, that task may finish, but it is marked disabled and is not requeued. No forced cancellation is required for this feature.

## Testing

Backend tests:
- Migration/repository coverage proves `polling_enabled` defaults to enabled and round-trips through create, update, get, and list.
- API tests prove the field is emitted and can be updated.
- Service tests prove the flag updates independently of polling interval overrides.
- Scheduler tests prove disabled devices create no recurring tasks and existing tasks are removed or disabled on refresh.

Frontend tests:
- Parser tests prove omitted `polling_enabled` falls back to `true` and explicit `false` is preserved.
- Client/panel tests prove toggling sends `polling_enabled`, shows save/error feedback, and disables polling override controls while polling is suspended.

## Out Of Scope

- Bulk toggling multiple devices in one action.
- Automatically disabling polling after repeated failures.
- Cancelling an already running poll mid-flight.
- Changing device status, freshness labels, or historical metrics retention when polling is suspended.
