<#
.SYNOPSIS
  Builds picframer.exe. Used locally and by the release workflow.

.DESCRIPTION
  Runs go vet and the tests, generates the Windows resources (icon,
  manifest, version info) and builds dist\picframer.exe.

.PARAMETER SkipChecks
  Skip go vet and go test for a quick build.

.EXAMPLE
  .\build.ps1
.EXAMPLE
  .\build.ps1 -SkipChecks
#>
param(
    [switch]$SkipChecks
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

function Invoke-Step([string]$Name, [scriptblock]$Command) {
    Write-Host "==> $Name" -ForegroundColor Cyan
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$Name failed (exit code $LASTEXITCODE)"
    }
}

if (-not $SkipChecks) {
    Invoke-Step "go vet" { go vet ./... }
    Invoke-Step "go test" { go test ./... }
}
Invoke-Step "go generate (icon, manifest, version info)" { go generate . }
Invoke-Step "go build" { go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/picframer.exe . }

Write-Host "Built $(Join-Path $PSScriptRoot 'dist\picframer.exe')" -ForegroundColor Green
