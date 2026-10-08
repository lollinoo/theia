---
status: accepted
---

# Shared maintenance procedures for the administration command and UI

Theia will expose a single administration command for installation, upgrade, backup, restore, and status inspection that remains usable when the UI is unavailable. The UI will provide daily backup scheduling, initiation, and monitoring through the same maintenance procedures, so their validation and operation semantics remain consistent across entry points.

The administrator chooses when to upgrade and selects an explicit release version; the procedure automates preflight checks, a preventive backup, migrations, and final verification. This preserves control of maintenance timing while removing manual orchestration.

An upgrade or restore failure after data changes will trigger an attempt to recover the previous state before writes reopen. If recovery or verification fails, the instance stays in maintenance with diagnostics and an intervention command. The operation-scoped rollback and interruption policy is recorded in [ADR-0005](0005-operation-scoped-rollback-and-resumption.md). Implementation is pending.
