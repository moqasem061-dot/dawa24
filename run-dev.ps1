# =============================================================================
# Dawa24 Store - Local Development Runner (PowerShell)
# =============================================================================

$env:GOTMPDIR = "F:\temp"
$env:GOCACHE = "F:\temp\gocache"

# Default Local Dev Environment Variables (can be overridden by .env)
if (-not $env:APP_ENV) { $env:APP_ENV = "dev" }
if (-not $env:PORT) { $env:PORT = "8080" }
if (-not $env:SESSION_SECRET) { $env:SESSION_SECRET = "dawa24-local-dev-session-secret-key-must-be-at-least-32-chars-long" }
if (-not $env:DATABASE_URL) { $env:DATABASE_URL = "postgres://dawa24:dawa24@localhost:5432/dawa24_store?sslmode=disable" }
if (-not $env:REDIS_URL) { $env:REDIS_URL = "redis://localhost:6379/0" }
if (-not $env:APP_BASE_URL) { $env:APP_BASE_URL = "http://localhost:8080" }

# Load .env file if it exists
if (Test-Path ".env") {
    Write-Host "[*] Loading environment variables from .env..." -ForegroundColor Cyan
    Get-Content .env | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line.Contains("=")) {
            $key, $value = $line.Split("=", 2)
            [System.Environment]::SetEnvironmentVariable($key.Trim(), $value.Trim())
        }
    }
}

Write-Host "======================================================" -ForegroundColor Green
Write-Host " 🚀 Starting Dawa24 Store on http://localhost:$env:PORT" -ForegroundColor Green
Write-Host "======================================================" -ForegroundColor Green

Write-Host "[1/3] Generating Templ UI components..." -ForegroundColor Yellow
templ generate

if ($LASTEXITCODE -ne 0) {
    Write-Host "[!] Templ generation failed. Please check templates." -ForegroundColor Red
    exit $LASTEXITCODE
}

# Apply pending migrations before serving. The server also tries this on startup,
# but it can only log a failure (under the least-privilege role DDL is refused)
# and keeps serving - and then every screen that reads a newly added column
# answers 500. Running it here either applies them or stops with the reason.
Write-Host "[2/3] Applying database migrations..." -ForegroundColor Yellow
go run ./cmd/cli migrate

if ($LASTEXITCODE -ne 0) {
    Write-Host "[!] Migrations failed. Fix the error above before starting the server." -ForegroundColor Red
    exit $LASTEXITCODE
}

Write-Host "[3/3] Running Go HTTP Server..." -ForegroundColor Yellow
go run ./cmd/server
