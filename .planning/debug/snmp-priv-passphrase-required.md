---
status: awaiting_human_verify
trigger: "SNMP discovery fails with securityParameters.PrivacyPassphrase is required when user selects Prometheus without Fallback"
created: 2026-03-23T00:00:00Z
updated: 2026-03-23T00:30:00Z
---

## Current Focus

hypothesis: CONFIRMED — probeDevice() in device_service.go calls discoverFunc (gosnmp) unconditionally regardless of MetricsSource. When MetricsSourcePrometheus is selected, the code must skip the SNMP probe entirely and set status to "up" without credentials.
test: Traced full call chain: AddDevice → probeDevice → discoverFunc(gosnmp) with no MetricsSource branch check
expecting: Fix requires a MetricsSource guard in probeDevice that skips gosnmp when source is "prometheus" or "prometheus_snmp_fallback"
next_action: Apply fix to internal/service/device_service.go

## Symptoms

expected: When "Prometheus without Fallback" is selected as discovery method, the backend should query Prometheus/SNMP-Exporter for device metrics instead of using direct gosnmp connections. No SNMP credentials should be required.
actual: Backend still attempts direct SNMP connection using gosnmp and fails because SNMPv3 privacy passphrase is missing.
errors: "SNMP discovery failed for 10.0.9.254: securityParameters.PrivacyPassphrase is required when a privacy protocol is specified"
reproduction: Add a device with "Prometheus without Fallback" discovery method in production. The gosnmp library validates SNMPv3 security parameters and rejects the connection.
started: First production deploy. The discovery method selection may not be properly routing to the Prometheus code path.

## Eliminated

(none yet)

## Evidence

- timestamp: 2026-03-23T00:10:00Z
  checked: internal/service/device_service.go — probeDevice()
  found: Line 131 calls `s.discoverFunc(deviceIP, creds)` with NO check on `device.MetricsSource`. The metricsSource is stored in the device struct but completely ignored in the probe code path.
  implication: Regardless of whether user chose "prometheus", "snmp", or "prometheus_snmp_fallback", the gosnmp discovery is always attempted.

- timestamp: 2026-03-23T00:10:00Z
  checked: internal/service/device_service.go — AddDevice()
  found: MetricsSource is accepted as a parameter and stored in the device, but the async probe goroutine on line 116 just calls s.probeDevice(device) with no routing logic.
  implication: There is zero conditional branching on MetricsSource before gosnmp is invoked.

- timestamp: 2026-03-23T00:10:00Z
  checked: cmd/theia/main.go — newSNMPDiscoverFunc()
  found: The discoverFunc creates a gosnmp Client via snmp.NewClient() and calls client.Connect(). If SNMPv3 credentials have a PrivProtocol set but PrivPassword is empty, gosnmp validates the SecurityParameters on Connect() and returns "PrivacyPassphrase is required".
  implication: The error occurs even before any OID query — it happens at the connection/validation stage inside gosnmp.

- timestamp: 2026-03-23T00:10:00Z
  checked: internal/snmp/client.go — NewClient()
  found: Lines 79-97: when MsgFlags has AuthPriv bit set (i.e. SecurityLevel = "authPriv"), user.PrivacyPassphrase is set from creds.V3.PrivPassword. If PrivPassword is empty but PrivProtocol is non-empty, PrivacyProtocol gets set to a non-NoPriv value (e.g. AES) but PrivacyPassphrase is empty string — exactly what gosnmp validates against.
  implication: Two separate bugs can trigger this: (1) Prometheus-only device with no SNMP creds still attempts SNMP probe; (2) If SNMP creds ARE provided with authPriv but no priv password, same error.

- timestamp: 2026-03-23T00:10:00Z
  checked: internal/worker/metrics_collector.go — buildSnapshot()
  found: Lines 276, 418, 488, 596: MetricsSource IS properly checked in the metrics collector. Prometheus-sourced devices are queried via Prometheus; SNMP-sourced devices are polled via snmpPollFunc. The routing logic exists correctly in the collector — just not in the discovery/probe path.
  implication: The missing guard is ONLY in device_service.go's probeDevice method, not in the broader system design.

- timestamp: 2026-03-23T00:10:00Z
  checked: internal/api/device_handler.go — parseSNMPCreds()
  found: When no SNMP creds are provided for a Prometheus device, the function defaults to version "2c" with community "public". So the stored SNMPCredentials will be v2c, not v3 — meaning the authPriv/privPassphrase error wouldn't come from a freshly created Prometheus device... UNLESS the user also fills in SNMPv3 fields.
  implication: The bug specifically manifests when user selects "Prometheus without Fallback" AND submits SNMPv3 credentials with PrivProtocol set but PrivPassword empty (common if using authNoPriv level but PrivProtocol field is visible and populated from a profile or partial form).

## Resolution

root_cause: probeDevice() in internal/service/device_service.go called discoverFunc (gosnmp) unconditionally, ignoring device.MetricsSource. Any device — regardless of whether it was configured for Prometheus-only collection — had gosnmp attempted against it. When a device with MetricsSourcePrometheus also had SNMPv3 authPriv credentials with an empty PrivPassword (possible from the form when priv fields are submitted), gosnmp's Connect() validation rejected it with "PrivacyPassphrase is required when a privacy protocol is specified". The error also surfaces even with empty v2c creds for Prometheus devices if the form is left on v3 defaults.

fix: Added a MetricsSource guard at the top of probeDevice() in internal/service/device_service.go. When metricsSource == domain.MetricsSourcePrometheus, the function skips gosnmp entirely, sets the device status to "up" directly (Prometheus/blackbox_exporter determines real reachability via probe_success), and returns early. The prometheus_snmp_fallback case is intentionally NOT skipped — those devices benefit from SNMP discovery for sys info and interface enumeration.

verification: All existing service tests pass. Two tests (TestProbeCompletes_DeviceStatusUp, TestProbeFails_DeviceStatusDown) were updated to explicitly use MetricsSourceSNMP since they test SNMP-specific behavior. Two new regression tests added: TestPrometheusDevice_SkipsSNMPProbe and TestPrometheusDevice_SNMPv3WithPrivProtocol — both verify the specific bug scenario and pass. All 5 internal packages build and test clean.

files_changed:
  - internal/service/device_service.go
  - internal/service/device_service_test.go
