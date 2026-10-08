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
may still require those keys. The automatic maintenance command and encrypted
recovery workflow are subsequent implementation stages.
