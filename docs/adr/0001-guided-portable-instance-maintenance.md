---
status: accepted
---

# Guided, portable deployment and instance maintenance

Theia will provide guided first startup from published images on Linux hosts with Docker, plus a verified Helm deployment for Kubernetes with one application replica initially. The standard deployment will be self-contained and require no external secret-management service; this keeps the initial operational burden small while allowing integrations with existing infrastructure.

Release distribution will include complete deployment bundles, an administration binary, and backend/frontend images for Linux AMD64 and ARM64. Docker hosts require Engine and Compose; Kubernetes operators require a configured cluster, Helm, and kubectl. Git, Make, Go, and Node are excluded from the server installation prerequisites. The initial workflow targets one administration command followed by guided activation.

Bundled PostgreSQL is the default, and an external PostgreSQL connection will be supported through guided configuration without hand-editing Compose files. The infrastructure owner remains responsible for managing an external PostgreSQL server. Both LAN addresses and public domains are supported; a public domain is not a prerequisite for installation.

The deployment will provision and renew the HTTPS certificates it manages. LAN clients without an existing trusted organizational CA will require a one-time trust step per client; existing certificates and reverse proxies remain supported, while Kubernetes deployments integrate with the cluster's TLS management. Renewal of externally supplied certificates remains with their infrastructure owner. The certificate authority state must persist across restarts to avoid repeatedly enrolling clients.

A new instance's first administrator will be created through a one-time activation link produced by the administration command, with administrator-chosen credentials and recovery-key handover in the guided flow. Existing installations retain their registered users.

Existing installations are part of the supported transition: preserve application data and device credentials, assist the import of existing configuration and encryption keys, and maintain a recovery path for retained backups. Instance restore covers the database and the persistent artifacts required to recover the complete Theia instance on a replacement host.

Restore and upgrades that change database structure or data may use an explicit maintenance window with application writes stopped and operation status visible. Bundled PostgreSQL major-version upgrades are included through explicitly supported and verified transition paths, preserving the original database until the replacement is verified. Multi-replica high availability remains outside the initial scope.

The guided Docker adapter, activation, persistent CA and replacement restore are implemented and exercised by `make deployment-test`. Kubernetes, release packaging and the bundled PostgreSQL major-version transition remain pending.

Related decisions: [encrypted instance backups](0002-encrypted-instance-backups-with-recovery-key.md), [shared CLI and UI procedures](0003-shared-cli-and-ui-maintenance-procedures.md), [maintenance before HTTP startup](0004-database-maintenance-before-http-startup.md), [operation-scoped rollback](0005-operation-scoped-rollback-and-resumption.md), and [secret rotation and legacy transition](0006-managed-secret-rotation-and-legacy-transition.md).
