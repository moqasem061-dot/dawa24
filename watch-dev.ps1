# =============================================================================
# Dawa24 Store - Live Hot-Reload Watcher (PowerShell)
# =============================================================================

$env:GOTMPDIR = "F:\temp"
$env:GOCACHE = "F:\temp\gocache"

# Default Local Dev Environment Variables
if (-not $env:APP_ENV) { $env:APP_ENV = "dev" }
if (-not $env:PORT) { $env:PORT = "8080" }
if (-not $env:SESSION_SECRET) { $env:SESSION_SECRET = "dawa24-local-dev-session-secret-key-must-be-at-least-32-chars-long" }
if (-not $env:DATABASE_URL) { $env:DATABASE_URL = "postgres://dawa24:dawa24@localhost:5432/dawa24_store?sslmode=disable" }
if (-not $env:REDIS_URL) { $env:REDIS_URL = "redis://localhost:6379/0" }
if (-not $env:APP_BASE_URL) { $env:APP_BASE_URL = "http://localhost:8080" }

if (Test-Path ".env") {
    Get-Content .env | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line.Contains("=")) {
            $key, $value = $line.Split("=", 2)
            [System.Environment]::SetEnvironmentVariable($key.Trim(), $value.Trim())
        }
    }
}

Write-Host "======================================================" -ForegroundColor Cyan
Write-Host " 🔄 Starting Live Watcher (Auto-reloads on edits)" -ForegroundColor Cyan
Write-Host " Browser Proxy: http://localhost:7331 (or http://localhost:8080)" -ForegroundColor Cyan
Write-Host "======================================================" -ForegroundColor Cyan

templ generate --watch --proxy="http://localhost:8080" --cmd="go run ./cmd/server"
