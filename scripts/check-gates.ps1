# Runs the static gates that do not need make or bash, for Windows developers.
# It covers a subset of `make gates`: formatting, vet, the 400-line file limit,
# and the dead-code ratchet. CI runs the full `make gates` on Linux.
#
# Usage (from the repo root):  powershell -ExecutionPolicy Bypass -File scripts/check-gates.ps1
$ErrorActionPreference = 'Stop'
$failed = $false

Write-Host '==> gofmt'
$unformatted = gofmt -l cmd internal test
if ($unformatted) { Write-Host "unformatted files:`n$unformatted"; $failed = $true }

Write-Host '==> go vet'
go vet ./...
if ($LASTEXITCODE -ne 0) { $failed = $true }

Write-Host '==> file size (400 lines)'
Get-ChildItem -Path cmd, internal -Recurse -Filter *.go |
    Where-Object { $_.Name -notlike '*_templ.go' } |
    ForEach-Object { [pscustomobject]@{ File = $_.FullName; Lines = (Get-Content $_.FullName).Count } } |
    Where-Object { $_.Lines -gt 400 } |
    ForEach-Object { Write-Host "  $($_.File): $($_.Lines) lines"; $failed = $true }

Write-Host '==> dead code ratchet'
go test -count=1 -run '^TestDeadcodeRatchet$' ./test/
if ($LASTEXITCODE -ne 0) { $failed = $true }

if ($failed) { Write-Host 'FAILED'; exit 1 }
Write-Host 'All checks passed.'
