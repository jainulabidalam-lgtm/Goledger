param(
    [string]$SqlitePath = "C:\Users\alama\AppData\Local\Android\Sdk\platform-tools\sqlite3.exe",
    [string]$BackupFile,
    [string]$TestDbPath = "backups\goledger-restore-test.db"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $SqlitePath)) {
    throw "SQLite CLI not found at $SqlitePath"
}

if (-not $BackupFile) {
    $backups = Get-ChildItem -Path "backups" -Filter "goledger-*.db" | Where-Object { $_.Name -notmatch "restore-test" } | Sort-Object CreationTime -Descending
    if ($backups.Count -eq 0) {
        throw "No backups found."
    }
    $BackupFile = $backups[0].FullName
}

if (-not (Test-Path $BackupFile)) {
    throw "Backup file not found at $BackupFile"
}

if (Test-Path $TestDbPath) {
    Remove-Item $TestDbPath -Force
}

if ([System.IO.Path]::GetFullPath($TestDbPath) -eq [System.IO.Path]::GetFullPath("goledger.db")) {
    throw "Safety check failed: Cannot restore over the active goledger.db"
}

Write-Host "Restoring $BackupFile to $TestDbPath..."
Copy-Item $BackupFile $TestDbPath

Write-Host "Verifying restored database integrity..."
$integrity = & $SqlitePath -readonly $TestDbPath "PRAGMA integrity_check;"
if ($integrity -ne "ok") {
    throw "Integrity check failed: $integrity"
}
Write-Host "Integrity check passed."

Write-Host "Verifying table counts..."
$query = "SELECT 'accounts', COUNT(*) FROM accounts UNION ALL SELECT 'journal_entries', COUNT(*) FROM journal_entries UNION ALL SELECT 'journal_lines', COUNT(*) FROM journal_lines;"
$counts = & $SqlitePath -readonly $TestDbPath $query
$counts | ForEach-Object {
    Write-Host $_
}
Write-Host "Restore test completed successfully."
