---
status: accepted
---

# Encrypted instance backups with an administrator-held recovery key

New instance backups will be encrypted and include the credential-encryption keys required to recover their data in protected form. A recovery key generated during initial configuration and held by the administrator outside the instance host, together with an instance backup, will enable recovery after loss of the host without manually configuring historical credential keys for each new-format backup.

The instance will keep only the public part of the recovery key; the private part is handed to the administrator and supplied temporarily when recovering an archive. This supports unattended encryption while keeping the persistent ability to open recovery archives outside the instance host.

Guided activation must verify that the exported private recovery material corresponds to the configured public recovery recipient. Verification through the temporary recipient alone does not establish that the administrator's recovery file can open the archive.

New archives will use the standard age format with the public recovery recipient and a per-backup temporary verification recipient. Its private verification identity is used only during backup creation and verification and is never persisted. This identity can decrypt the complete archive; its separation is temporal, not a restricted verification permission. Every newly created backup must be decrypted completely and checked for integrity before success is reported. A full restore into an isolated database must also run at least weekly on a newly created backup, and for backups preceding destructive maintenance; later full verification of retained archives uses the administrator's recovery file.

All creation-time verification must finish before releasing the temporary identity. An interruption that loses it leaves an unverified archive incomplete; a retry creates and verifies a replacement rather than reporting success from public metadata alone. Already verified bytes may be uploaded again after interruption using their durable verified digest, and verification of an external copy must establish that it contains those same bytes.

The default installation remains self-contained and will automatically generate operational secrets in protected persistent state for new installations, or import them from existing installations. Restart and upgrade reuse this state. Existing protected data with missing credential keys must stop startup and identify the recovery path, rather than generating replacement keys that cannot decrypt that data.

Backups support local storage and optional automatic upload to an S3-compatible destination configured once. Initial setup enables a backup every 24 hours and retention of the latest seven successful backups, with settings editable through the UI. External storage enables recovery from loss of the host that holds the local copy. The shared engine implements this decision and performs a real isolated PostgreSQL restore for every newly created managed backup.

When external storage is configured, backup completion requires verification of the external copy as well. Failed uploads preserve the local copy, trigger bounded retries, and remain visibly incomplete while the application continues running. Destructive maintenance waits for the required preventive backup to complete. Incomplete uploads cannot displace the retained successful recovery points.
