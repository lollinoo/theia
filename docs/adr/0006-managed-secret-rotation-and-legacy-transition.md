---
status: accepted
---

# Managed secret rotation and legacy transition

Credential encryption keys will rotate automatically during maintenance when older than the default 90-day interval. Operational database passwords, session secrets, and metrics tokens are generated initially and replaced through an explicit administration command. Recovery-key replacement is an explicit guided operation, with the tooling managing the history still required by retained backups; private recovery material remains in the administrator-held recovery file.

Transition from an existing installation imports its original configuration and credential secrets through a guided operation. Accessible legacy PostgreSQL archives can be converted into verified encrypted instance backups, retaining the originals until conversion is verified. Original credential secrets must be preserved faithfully, and their historical recovery capability retained while unconverted archives still require them.

SQLite archives retain a separate migration path through the compatible legacy Theia version before entering the supported PostgreSQL workflow. The new workflow does not infer missing credential secrets or promise native restoration of unsupported legacy database formats. Implementation is pending.
