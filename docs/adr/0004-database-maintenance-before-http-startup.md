---
status: accepted
---

# Execute database maintenance before starting HTTP service

Schema and credential migrations, and instance restore, will execute through dedicated administration commands or maintenance Jobs before the HTTP service becomes available. These entry points reuse the shared maintenance procedures and prepare and verify the state before application startup, separating destructive operations and their retries from serving user requests.

The HTTP server continues to handle ordinary application activity and backups through the shared procedures. Maintenance writes must be coordinated with application shutdown and readiness, and the application becomes available only after the required maintenance verification succeeds. Operation state must survive interruption and replacement of the application database, as described in [ADR-0005](0005-operation-scoped-rollback-and-resumption.md).

The offline engine and managed HTTP startup checks are implemented. Legacy unmanaged deployments retain their existing startup behavior during the transition. Kubernetes will use the same commands in a dedicated maintenance container before HTTP starts.
