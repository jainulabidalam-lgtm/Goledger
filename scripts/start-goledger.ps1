$ErrorActionPreference = "Stop"
Set-Location "F:\GoLedger"

$listener = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
if ($listener) {
    foreach ($pidValue in ($listener.OwningProcess | Sort-Object -Unique)) {
        $p = Get-CimInstance Win32_Process -Filter "ProcessId = $pidValue"
        Write-Host "Port 8080 is occupied by PID $pidValue"
        Write-Host "Process: $($p.Name)"
        Write-Host "Command: $($p.CommandLine)"
    }
    Write-Host "No new server started. Inspect the process above first."
    exit 1
}

& "F:\Golang\bin\go.exe" test ./...
if ($LASTEXITCODE -ne 0) { throw "Tests failed; server not started." }

& "F:\Golang\bin\go.exe" build -o ".\goledger-check.exe" ".\cmd\goledger"
if ($LASTEXITCODE -ne 0) { throw "Build failed; server not started." }

Write-Host "Starting GoLedger. Press Ctrl+C to stop it."
& ".\goledger-check.exe"
