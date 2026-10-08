# Instance administration

The managed deployment is being introduced incrementally. The existing Compose
deployment remains usable during the transition. Architecture and remaining
deployment work are recorded in [the ADRs](docs/adr/0001-guided-portable-instance-maintenance.md).

## Persistent secrets and existing installations

`THEIA_INSTANCE_STATE=/persistent/secrets.json` selects managed instance secrets.
The file and its directory must be private (0600 and 0700 respectively) and
persistent across container replacement. Missing or corrupt state stops startup;
startup never creates replacement credential keys.

For an existing instance, run the new backend binary with the original
configuration and original secret environment loaded:

```sh
theia instance import -config config.yaml -state /persistent/secrets.json
theia instance status -state /persistent/secrets.json
```

Import preserves original credential passphrases, session secrets, metrics tokens,
database connection, and any supplied `POSTGRES_PASSWORD`. It refuses an existing
state file. Original credential keys are required; derived AES bytes cannot
substitute for the passphrases. Preserve the original environment file securely
until the deployment transition and recovery test complete.

Stop the application before changing configuration to use `THEIA_INSTANCE_STATE`.
Remove the original encryption, session, and metrics secret variables and YAML
values: conflicting sources are rejected. `THEIA_DB_DSN` may override the saved
connection for a destination database on another host. Mount state outside the
application's restore staging area. `instance status` prints identifiers and due
dates, never private keys or passwords.

Unmanaged installations can alternatively use secret files:
`THEIA_DB_DSN_FILE`, `THEIA_SESSION_SECRET_FILE`, `THEIA_METRICS_TOKEN_FILE`,
`THEIA_ENCRYPTION_KEY_ID_FILE`, `THEIA_ENCRYPTION_KEYS_FILE`, and
`THEIA_ENCRYPTION_KEY_FILE`. Set either a direct variable or its `_FILE` variant;
setting both is an error. File line endings are removed. Kubernetes projected
secret files and Docker Compose secrets can use this interface.

Credential rotation retains original and historical keys. Rewrapping and removal
of any historical key must remain separate operations: old unconverted archives
may still require those keys.

## Encrypted backups

For an imported instance, export its recovery file explicitly:

```sh
theia instance recovery -state /persistent/secrets.json -recovery-file /operator/recovery.txt
```

The operator directory must already exist. The command refuses an existing
recovery file, rereads the exported file to verify its public recipient, and saves
only that recipient in instance state. Move the private recovery file to
independent storage outside the instance host. Protect it like a password.

Managed backups created by the existing UI and scheduler use `.tar.gz.age` and
contain protected instance secrets, the PostgreSQL dump, retained device backups,
and SSH known hosts. Each archive is encrypted directly while writing. Before
success, the backend decrypts the entire archive with a temporary in-memory
identity, checks its contents, actually restores an isolated PostgreSQL 18 cluster,
runs migrations, and verifies every stored sensitive credential. The temporary
identity is never written to disk. The production image includes the PostgreSQL
server tools needed for this verification; no external database permissions or
Docker socket are required. This verification runs on every managed backup.

After interruption, the existence of an encrypted archive is insufficient to
declare success. A complete durable verification receipt and matching archive
digest are required. An archive interrupted before verification must be recreated.

Optionally configure an existing S3-compatible bucket once. Supply access
credentials through `THEIA_S3_ACCESS_KEY_FILE` and `THEIA_S3_SECRET_KEY_FILE`, then:

```sh
theia instance s3 -state /persistent/secrets.json -endpoint https://s3.example.org -bucket theia-backups -region us-east-1
```

The command saves credentials in private instance state; remove the input
environment variables afterwards and restart the application. S3 credentials are
also protected inside encrypted backups. The bucket must already exist and permit
upload, download, and deletion within the selected prefix. External verification
downloads and hashes the complete object; an ETag alone is insufficient.

When S3 is configured, success requires a verified external copy. An unavailable
destination leaves the archive in `pending_upload`, preserves the verified local
copy for download, and retries at the scheduler's hourly cycle (up to three pending
archives per cycle). Pending uploads are excluded from automatic retention and
cannot displace successful recovery points. An interrupted upload can resume
using the same verified bytes without retaining the temporary private identity.
Keep enough disk space for local pending copies during prolonged outages.

Historical unencrypted archives continue using the legacy restore path. Managed
encrypted restore uses the operator recovery file through the maintenance CLI;
that CLI is the next implementation stage. Do not upload recovery identities to
the existing legacy archive upload form.
