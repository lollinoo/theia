---
status: accepted
---

# Operation-scoped rollback and resumable maintenance

Each upgrade or instance restore will have its own verified safety snapshot containing the preceding database, persistent artifacts, operational secrets, deployment configuration, and release version. It will be encrypted with instance-managed material so rollback can run without the administrator's offline recovery key. The snapshot and its decryption material must remain available until the operation has completed and verification has succeeded; neither a retry nor a subsequent operation may overwrite or reuse another operation's snapshot.

Snapshot decryption material must be retained outside the encrypted snapshot and independently of the database, secrets, or configuration being replaced. This avoids a circular recovery dependency and permits rollback when an operation fails while changing the live key state.

Only one instance-changing operation may run at a time. Its durable progress, identifiers, recovery-point references, and verification results must survive process interruption and replacement of the application database. Recovery resumes from a verified phase, with bounded retries for transient failures. An ambiguous state keeps the instance in maintenance with an actionable administration command.

Automatic rollback applies before application writes reopen. If rollback or its verification fails, the instance remains in maintenance; a completed operation does not later roll back automatically after accepting new writes. Implementation is pending.
