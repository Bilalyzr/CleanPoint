# Windows PowerShell benchmark runner: runs snapshot, restore, and latency
# benchmarks against a running CleanPoint server into results\.
$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $RepoRoot
try {
    $Api = if ($env:CLEANPOINT_API) { $env:CLEANPOINT_API } else { "http://127.0.0.1:8099" }
    New-Item -ItemType Directory -Force -Path "results" | Out-Null

    try {
        $resp = Invoke-RestMethod -Uri "$Api/api/v1/healthz" -TimeoutSec 2
        if ($resp.status -ne "ok") { throw "unhealthy" }
    } catch {
        throw "start the server first: .\scripts\demo.ps1 or .\bin\cleanpoint.exe serve"
    }

    Write-Host "==> snapshot capture benchmark (1000 x 32 KiB)"
    .\bin\cleanpoint.exe benchmark snapshots --files 1000 --file-size-kb 32 `
        --iterations 3 --api $Api --output results\snapshot-1000.json
    if ($LASTEXITCODE -ne 0) { throw "snapshot benchmark failed" }

    Write-Host "==> restore benchmark (1000 x 32 KiB)"
    .\bin\cleanpoint.exe benchmark restore --files 1000 --file-size-kb 32 `
        --iterations 3 --api $Api --output results\restore-1000.json
    if ($LASTEXITCODE -ne 0) { throw "restore benchmark failed" }

    Write-Host "==> API latency"
    $tokLine = Get-Content "sandbox\.tokens" | Where-Object { $_ -match '^operator=' } | Select-Object -First 1
    if ($tokLine) {
        $tok = ($tokLine -split '=', 2)[1].Trim()
        $wls = Invoke-RestMethod -Uri "$Api/api/v1/workloads" -Headers @{ Authorization = "Bearer $tok" }
        if ($wls.workloads -and $wls.workloads.Count -gt 0) {
            $wlId = $wls.workloads[-1].id
            python benchmarks\latency.py --api $Api --workload $wlId --samples 20 --output results\latency.json
            if ($LASTEXITCODE -ne 0) { throw "latency probe failed" }
        } else {
            Write-Host "(no workloads yet; skipping latency probe)"
        }
    }

    Write-Host "benchmarks complete; see results\"
} finally {
    Pop-Location
}
