# GoLedger

A production-minded, lightweight double-entry bookkeeping application built with Go and SQLite.

## Stage 1

Currently implements a minimal HTTP server with a health check endpoint.

### Running the server

```bash
go build ./cmd/goledger
./goledger
```

Check the liveness endpoint: `http://localhost:8080/health/live`
Check the readiness endpoint: `http://localhost:8080/health/ready`

### Admin Provisioning

To create the initial administrative user, run the application with the following environment variables:

```bash
$env:PROVISION_ADMIN_USER="admin"
$env:PROVISION_ADMIN_PASS="your_secure_password"
./goledger
```
The application will hash and store the credentials securely and then clear the variables from the active environment. Note: PowerShell keeps `$env` variables for the session duration unless cleared. Consider clearing them or using a wrapper script to avoid keeping passwords in session history.

### Configuration

The server can be configured using environment variables:

- `HOST`: Host to bind the server to (default: `127.0.0.1`).
- `PORT`: Port to listen on (default: `8080`).
- `DB_PATH`: Path to the SQLite database (default: `goledger.db`).

Example:
```bash
$env:HOST="0.0.0.0"
$env:PORT="9090"
$env:DB_PATH="custom.db"
./goledger
```

## Backup and Recovery

### Backing up the database

To safely back up the SQLite database without stopping the server, run the provided PowerShell script:

```powershell
.\scripts\backup-goledger.ps1
```

You can optionally specify the SQLite executable path and retention count:

```powershell
.\scripts\backup-goledger.ps1 -SqlitePath "C:\path\to\sqlite3.exe" -RetentionCount 7
```

### Testing a backup

To verify a backup is valid, you can use the restore-test script. This restores the database to a separate test file and verifies integrity and table counts. It will never overwrite the active `goledger.db`.

```powershell
.\scripts\restore-test.ps1 -BackupFile ".\backups\goledger-20231010-100000.db"
```

If `-BackupFile` is not provided, it automatically selects the latest backup.

## Phase 5: Audit Trail and Accounting Integrity

GoLedger now includes an immutable audit trail that tracks all account creations, journal entries, and reversals. This feature uses application-level immutability to prevent standard users from deleting or altering logs. Reversals are supported through the /api/journal-entries/{id}/reverse endpoint, which securely posts an offset entry to reverse previous balances without mutating historical records.
