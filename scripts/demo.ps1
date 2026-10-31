# One-command Windows PowerShell demo: starts the local server (if needed)
# and runs the guided three-scene demonstration.
$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $RepoRoot
try {
    $Api = if ($env:CLEANPOINT_API) { $env:CLEANPOINT_API } else { "http://127.0.0.1:8099" }
    $env:CLEANPOINT_API = $Api
    $Addr = $Api -replace '^https?://', ''

    New-Item -ItemType Directory -Force -Path "sandbox" | Out-Null

    $healthy = $false
    try {
        $resp = Invoke-RestMethod -Uri "$Api/api/v1/healthz" -TimeoutSec 2
        if ($resp.status -eq "ok") { $healthy = $true }
    } catch {}

    $proc = $null
    if (-not $healthy) {
        if (-not (Test-Path "bin\cleanpoint-server.exe")) {
            & "$PSScriptRoot\build.ps1"
        }
        Write-Host "==> starting CleanPoint server on $Api"
        $env:CLEANPOINT_ADDR = $Addr
        $proc = Start-Process -FilePath "bin\cleanpoint-server.exe" `
            -WorkingDirectory $RepoRoot `
            -RedirectStandardOutput "sandbox\server.log" `
            -RedirectStandardError "sandbox\server.err.log" `
            -PassThru -NoNewWindow
        $proc.Id | Set-Content -Path "sandbox\server.pid"

        for ($i = 0; $i -lt 20; $i++) {
            Start-Sleep -Milliseconds 500
            try {
                $resp = Invoke-RestMethod -Uri "$Api/api/v1/healthz" -TimeoutSec 2
                if ($resp.status -eq "ok") { $healthy = $true; break }
            } catch {}
        }
        if (-not $healthy) {
            throw "server failed to start; see sandbox\server.log and sandbox\server.err.log"
        }
    }

    Write-Host "==> running guided demo"
    python simulator\demo.py --api $Api
    if ($LASTEXITCODE -ne 0) { throw "demo failed" }

    Write-Host ""
    Write-Host "demo complete. Landing: $Api/ | Console: $Api/app/"
    Write-Host "tokens in sandbox\.tokens | restores in sandbox\restored\"
    if ($proc) {
        Write-Host "stop the server with: Stop-Process -Id $($proc.Id)"
    }
} finally {
    Pop-Location
}
