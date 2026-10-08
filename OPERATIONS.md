# Instance administration

The standalone `theia-admin` binary manages Docker deployment without Git, Make,
Go or Node on the server. Docker Engine and Compose are the host prerequisites.
Architecture is recorded in [the ADRs](docs/adr/0001-guided-portable-instance-maintenance.md).

## Guided Docker deployment

Use the administration binary from a versioned release. Its release build pins
the matching image version; development builds require `-release` explicitly.

```sh
theia-admin install -dir /opt/theia -hostname theia.example.org
```

This creates private persistent secrets once, starts bundled PostgreSQL 18,
runs verified offline migrations, starts the backend and HTTPS frontend, and
checks the actual HTTPS endpoint before printing the temporary activation link.
Choose your administrator credentials and save the recovery file on your
computer through that link. Do not store its private identity on the instance host.

LAN IP addresses and local hostnames use an internal CA automatically. Export its
public certificate and trust it once on each client:

```sh
theia-admin ca -dir /opt/theia -output theia-ca.crt
```

The CA persists across restarts and belongs to the encrypted instance backup.
Public DNS names use automatic certificate issuance and renewal. DNS must reach
the host and ports 80/443 must be available for certificate challenges. Override
the TLS choice with `-tls internal` or `-tls auto`. Use `-tls external` with
`-tls-cert-file` and `-tls-key-file` for existing PEM certificates; their owner
handles renewal. `-tls proxy` exposes HTTP behind an existing HTTPS proxy; specify
that proxy's source CIDRs with `-trusted-proxies` so forwarded HTTPS is preserved.
`-http-port`, `-https-port` and `-bind-address` customize published frontend ports.
The generated deployment publishes neither PostgreSQL nor the backend API.

For external PostgreSQL, pass `-database-dsn-file /private/postgres-connection.txt`
during installation. The connection is saved privately; it does not appear in
Compose or command arguments. The database owner manages server upgrades.

```sh
theia-admin status -dir /opt/theia
theia-admin up -dir /opt/theia
theia-admin activation -dir /opt/theia
theia-admin backup -dir /opt/theia -output /independent-storage/instance.age
theia-admin upgrade -dir /opt/theia -release v1.8.1
theia-admin resume -dir /opt/theia
```

Up and upgrade reuse persistent secrets. Upgrade downloads pinned images before
stopping writes, makes a verified preventive backup and a separate Safety Snapshot,
then runs migrations before starting HTTP. An interrupted operation resumes from
its original snapshot. Automatic rollback closes permanently when HTTP writes
reopen. The frontend serves maintenance status while the backend is stopped.
`-offline` uses images already loaded on the host, including air-gapped installs.

A replacement host needs the encrypted archive and the administrator-held recovery
file. One restore command provisions its destination database, verifies the archive
with a real isolated restore, recovers users, keys, device backups and CA files,
and checks HTTPS. Existing instances use the same command inside a maintenance window:

```sh
theia-admin restore -dir /opt/theia -hostname theia.example.org \
  -archive /operator/instance.age -recovery-file /operator/theia-recovery.txt
```

The destination's database connection/password remain its own. Original credential
keys travel inside the encrypted archive. The supplied recovery file is mounted
from private temporary storage and removed after the command; it is never copied
into persistent instance state. Exported backups are verified against the original
digest and never overwrite an existing output file.

`make deployment-test` builds local images and exercises first activation in Chromium,
restart, upgrade, credential rotation, replacement restore, CA continuity and
rejection of a damaged archive in two isolated temporary Compose projects.
`make maintenance-test` verifies database rollback and interrupted-operation
resumption with isolated PostgreSQL clusters. These are development checks, not
installation prerequisites.

## Persistent secrets and existing installations

A new managed instance uses `theia instance init -state /persistent/secrets.json
-site https://theia.example.org` with its destination connection supplied through
`THEIA_DB_DSN_FILE`. This explicitly creates secrets once and prints an activation
link that expires after one hour. Run `maintenance migrate` before starting HTTP.
The activation page downloads a recovery file to the administrator's computer,
requires selecting that saved file again, and creates the chosen administrator
account only after proving it matches the configured public recipient. No fixed
administrator password or private recovery key is saved by the server. If the link
expires before creating any users, `instance activation` issues a replacement.
Existing users, including disabled users, prevent first-administrator activation.

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
SSH known hosts, and the persistent HTTPS certificate/CA files under
`THEIA_DATA_DIR/certificates`. Stop the frontend during restore so its certificate
cache cannot retain replaced state. Each archive is encrypted directly while writing. Before
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

Historical unencrypted archives continue using the legacy restore path in an
unmanaged installation. Managed encrypted restore uses the operator recovery file
through the maintenance CLI. Do not upload recovery identities to the legacy
archive upload form.

## Offline maintenance

Stop the application and execute a maintenance container/Job using the same
persistent instance state, application data, and PostgreSQL connection:

```sh
theia maintenance migrate
theia maintenance backup
theia maintenance restore -archive /input/instance.tar.gz.age -recovery-file /input/recovery.txt
theia maintenance status -state /persistent/secrets.json
theia maintenance resume
theia maintenance verify -archive /input/instance.tar.gz.age -recovery-file /input/recovery.txt
```

`THEIA_INSTANCE_STATE` selects the state file; `THEIA_DATA_DIR`,
`THEIA_BACKUP_DIR`, and `THEIA_INSTANCE_BACKUP_DIR` select the existing data paths.
`-config` loads a configuration file when necessary. Mount recovery input
read-only for the duration of the restore/verification command, then remove it.
`verify` needs PostgreSQL 18 tools, the archive and recovery file; it never opens
the live database. `status` needs only the state path and remains available when
the application or database cannot start.

Managed HTTP startup checks the schema, database identity and prior verification
receipt without migrating data or repeatedly decrypting all credentials. Set
`THEIA_RELEASE_TAG` identically for maintenance and HTTP containers to bind the
receipt to a pinned release. The HTTP process does not create a fixed default
administrator. Only one application or
maintenance process can acquire the persistent instance lease. The operating
system releases the lease after a crash. A fresh managed database is migrated
before HTTP startup and receives backup defaults of every 24 hours and seven
successful archives; existing settings are preserved.

When release, schema, verified keys and instance identity remain current, a
repeated `migrate` exits without creating another backup/snapshot or rewriting
data. Due rotation and explicit `-rotate-credentials` still run full maintenance.
A public summary containing only maintenance activity, action and phase is
written to the `public/status.json` directory beside instance state for the
deployment proxy to serve while the backend is stopped.

For an existing database, destructive maintenance first completes a verified
Instance Backup (including an external copy if configured), then creates a
separate encrypted Safety Snapshot. Each operation has a unique directory beside
the instance state. Its online snapshot key remains outside the snapshot and
outside replaced application data; the operator's recovery file is unnecessary
for automatic rollback. Snapshot identifiers and verified bytes are immutable.

Credential keys older than 90 days rotate during `migrate`; use
`-rotate-credentials` to rotate early. Historical keys remain available. The
procedure verifies every sensitive credential after SQL/data migrations. A
failure before writes reopen attempts to restore the original database, device
files, SSH hosts, and instance secrets. The original deployment release must be
restarted after a rollback to its earlier schema.

Interrupted changes block managed HTTP startup. `resume` uses the verified
snapshot belonging to that operation to roll back; it never replaces the snapshot
with partially changed live data. Recovery is bounded to three attempts. If it
cannot finish, the instance stays in maintenance and `status` reports the required
operator action. Completed operations are never automatically rolled back after
the application is allowed to serve writes again.

Offline restore defaults allow an encrypted archive of up to 2 GiB and reject
path traversal, duplicate entries, links, incomplete authentication, mismatched
database hashes, and unsupported newer schemas. Original database connection
credentials remain destination-specific; archived credential-encryption keys and
users are restored. Restored login sessions are revoked.

Developer verification: `make maintenance-test` builds the runtime image and tests
encrypted recovery across credential rotation, rollback after partial mutation,
and recovery of an interrupted second operation with its own snapshot. Tests use
isolated PostgreSQL clusters and do not touch the deployed instance.
