$ErrorActionPreference = "Continue"

$listeners = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
if (-not $listeners) {
    Write-Host "GoLedger: STOPPED (port 8080 is free)"
    exit 0
}

foreach ($processId in ($listeners.OwningProcess | Sort-Object -Unique)) {
    $p = Get-CimInstance Win32_Process -Filter "ProcessId = $processId"
    Write-Host "PID: $processId"
    Write-Host "Process: $($p.Name)"
    Write-Host "Executable: $($p.ExecutablePath)"
    Write-Host "Command: $($p.CommandLine)"
}

try {
    $health = Invoke-RestMethod "http://localhost:8080/health" -TimeoutSec 3
    Write-Host "Health endpoint: $($health.status)"
} catch {
    Write-Host "Health endpoint did not respond successfully."
}
