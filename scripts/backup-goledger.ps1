param(
    [string]$SqlitePath = "C:\Users\alama\AppData\Local\Android\Sdk\platform-tools\sqlite3.exe",
    [string]$DbPath = "goledger.db",
    [string]$BackupDir = "backups",
    [int]$RetentionCount = 7
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $SqlitePath)) {
    throw "SQLite CLI not found at $SqlitePath"
}

if (-not (Test-Path $DbPath)) {
    throw "Database not found at $DbPath"
}

if (-not (Test-Path $BackupDir)) {
    New-Item -ItemType Directory -Path $BackupDir | Out-Null
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$backupFile = Join-Path $BackupDir "goledger-$timestamp.db"

Write-Host "Creating backup at $backupFile..."
& $SqlitePath $DbPath ".backup '$backupFile'"
if ($LASTEXITCODE -ne 0) {
    throw "Failed to create backup."
}

Write-Host "Verifying backup integrity..."
$integrity = & $SqlitePath -readonly $backupFile "PRAGMA integrity_check;"
if ($integrity -ne "ok") {
    throw "Integrity check failed: $integrity"
}

Write-Host "Backup verified successfully."

$verifiedBackups = Get-ChildItem -Path $BackupDir -Filter "goledger-*.db" | Where-Object { $_.Name -notmatch "restore-test" } | Sort-Object CreationTime -Descending
if ($verifiedBackups.Count -gt $RetentionCount) {
    $backupsToDelete = $verifiedBackups | Select-Object -Skip $RetentionCount
    foreach ($b in $backupsToDelete) {
        Write-Host "Removing old backup $($b.FullName)..."
        Remove-Item $b.FullName -Force
    }
}
Write-Host "Backup process completed."
