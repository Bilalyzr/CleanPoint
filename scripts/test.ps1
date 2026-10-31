# Windows PowerShell test runner: go vet, TypeScript check, mandated >=92-case
# Go test matrix, and Python generator determinism check.
$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $RepoRoot
try {
    Write-Host "==> go vet"
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed" }

    if ((Get-Command npx -ErrorAction SilentlyContinue) -and (Test-Path "web\console\node_modules")) {
        Write-Host "==> TypeScript strict check (web/console)"
        Push-Location "web\console"
        try {
            npx tsc --noEmit
            if ($LASTEXITCODE -ne 0) { throw "tsc --noEmit failed" }
        } finally {
            Pop-Location
        }
    }

    Write-Host "==> go tests (full matrix)"
    $testOut = go test ./tests/... -v -count=1
    if ($LASTEXITCODE -ne 0) {
        $testOut | Write-Host
        throw "go test failed"
    }
    $cases = ($testOut | Select-String -Pattern '^    --- PASS:').Count
    Write-Host "automated cases passing: $cases"
    if ($cases -lt 92) {
        throw "FAIL: below the source-mandated minimum of 92 cases (got $cases)"
    }

    if (Get-Command python -ErrorAction SilentlyContinue) {
        Write-Host "==> python generator determinism check"
        python -c "import sys, json, hashlib; sys.path.insert(0, 'simulator'); from generate import build_timeline; a, _ = build_timeline('rapid_destructive', 77); b, _ = build_timeline('rapid_destructive', 77); ha = hashlib.sha256(json.dumps(a, sort_keys=True).encode()).hexdigest(); hb = hashlib.sha256(json.dumps(b, sort_keys=True).encode()).hexdigest(); assert ha == hb, 'generator is not deterministic!'; print('generator determinism: OK (%d events)' % len(a))"
        if ($LASTEXITCODE -ne 0) { throw "python determinism check failed" }
    }

    Write-Host "ALL TESTS PASS"
} finally {
    Pop-Location
}
