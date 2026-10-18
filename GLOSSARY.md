# Theia

Shared domain vocabulary for administering network operations and recovering a Theia instance.

## Language

**Theia Instance**:
An independently administered Theia environment containing its users, network inventory and topology, settings, device credentials, and retained device backups.
_Avoid_: Database, installation (when referring to the complete administered environment)

**Device Backup**:
A saved configuration of a network device managed by a Theia instance.
_Avoid_: Instance backup, database backup

**Instance Backup**:
A preserved recovery point for a Theia instance's application data and persistent artifacts, including its retained device backups.
_Avoid_: Device backup, database dump (when referring to the complete recovery point)

**Instance Restore**:
The recovery of a Theia instance from an instance backup and its required recovery material, including protected device credentials and retained device backups.
_Avoid_: Database import (when referring to recovery of the complete instance)

**Recovery Key**:
The administrator-held secret that unlocks protected instance backups and the credential-encryption keys they contain.
_Avoid_: Database password, session secret, credential-encryption key

**Recovery File**:
The administrator's recovery credential containing the private recovery-key versions needed to open retained instance backups.
_Avoid_: Environment file, instance backup, database password

**Credential Encryption Key**:
A secret belonging to a Theia instance that protects its stored network-device credentials.
_Avoid_: Recovery key, database password, session secret

**Safety Snapshot**:
The preserved pre-operation recovery point for a specific instance maintenance operation, representing the state to return to if that operation fails before application writes reopen.
_Avoid_: Instance backup, database dump (when referring to the complete pre-operation recovery point)
