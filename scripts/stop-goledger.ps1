$ErrorActionPreference = "Stop"
$root = "F:\GoLedger"

$listeners = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
if (-not $listeners) {
    Write-Host "GoLedger is not listening on port 8080."
    exit 0
}

foreach ($processId in ($listeners.OwningProcess | Sort-Object -Unique)) {
    $p = Get-CimInstance Win32_Process -Filter "ProcessId = $processId"
    if (-not $p -or -not $p.ExecutablePath -or
        -not $p.ExecutablePath.StartsWith($root, [StringComparison]::OrdinalIgnoreCase)) {
        Write-Host "Refusing to stop PID $processId: it is not verified as a GoLedger executable under $root."
        continue
    }

    Write-Host "Verified GoLedger process:"
    Write-Host "PID: $processId"
    Write-Host "Executable: $($p.ExecutablePath)"
    $confirm = Read-Host "Stop this GoLedger process? Type YES to confirm"
    if ($confirm -ceq "YES") {
        Stop-Process -Id $processId
        Write-Host "Stop requested for PID $processId."
    } else {
        Write-Host "Skipped PID $processId."
    }
}
