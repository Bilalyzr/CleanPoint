# One-command Windows PowerShell build: builds M2 web console (if npm available)
# and all Go binaries into .\bin, then runs go vet.
$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $RepoRoot
try {
    Write-Host "==> toolchain"
    go version
    if ($LASTEXITCODE -ne 0) { throw "go toolchain check failed" }
    python --version
    if ($LASTEXITCODE -ne 0) { throw "python toolchain check failed" }

    if (Get-Command npm -ErrorAction SilentlyContinue) {
        Write-Host "==> building M2 web console (web/console -> web/console-dist)"
        Push-Location "web\console"
        try {
            if (-not (Test-Path "node_modules")) {
                npm ci
                if ($LASTEXITCODE -ne 0) { throw "npm ci failed" }
            }
            npm run build
            if ($LASTEXITCODE -ne 0) { throw "npm run build failed" }
        } finally {
            Pop-Location
        }
    } else {
        Write-Host "==> npm not found; embedding prebuilt web/console-dist"
    }

    Write-Host "==> building Go binaries into .\bin"
    New-Item -ItemType Directory -Force -Path "bin" | Out-Null
    Remove-Item -Path "bin\*.exe~" -Force -ErrorAction SilentlyContinue

    go build -o "bin\cleanpoint.exe" .\cmd\cli
    if ($LASTEXITCODE -ne 0) { throw "failed building cmd/cli" }

    go build -o "bin\cleanpoint-server.exe" .\cmd\server
    if ($LASTEXITCODE -ne 0) { throw "failed building cmd/server" }

    go build -o "bin\cleanpoint-simulator.exe" .\cmd\simulator
    if ($LASTEXITCODE -ne 0) { throw "failed building cmd/simulator" }

    Write-Host "==> go vet"
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed" }

    Write-Host "build complete; binaries in .\bin"
} finally {
    Pop-Location
}
