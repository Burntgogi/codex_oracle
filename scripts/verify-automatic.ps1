param(
  [string]$BinaryPath = "",
  [switch]$SkipInstalledPlugin
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir

function Invoke-Step {
  param(
    [string]$Name,
    [string]$ScriptPath,
    [hashtable]$Arguments = @{}
  )
  if (-not (Test-Path -LiteralPath $ScriptPath)) {
    throw "Verification step '$Name' script not found: $ScriptPath"
  }
  Write-Output "== $Name =="
  if ($Arguments.Count -gt 0) {
    & $ScriptPath @Arguments
  } else {
    & $ScriptPath
  }
  if ($LASTEXITCODE -ne 0) {
    throw "Verification step '$Name' exited with code $LASTEXITCODE"
  }
}

$buildScript = Join-Path $scriptDir "build.ps1"
$mcpSmokeScript = Join-Path $scriptDir "smoke-mcp.ps1"
$marketplaceSmokeScript = Join-Path $scriptDir "smoke-marketplace-plan.ps1"
$installedVerifyScript = Join-Path $scriptDir "verify-installed.ps1"

Invoke-Step -Name "build" -ScriptPath $buildScript
Write-Output "Build verification passed"

$binaryArgs = @{}
if (-not [string]::IsNullOrWhiteSpace($BinaryPath)) {
  $binaryArgs = @{ BinaryPath = $BinaryPath }
}
Invoke-Step -Name "mcp-smoke" -ScriptPath $mcpSmokeScript -Arguments $binaryArgs
Invoke-Step -Name "recovery-smoke" -ScriptPath (Join-Path $scriptDir "smoke-recovery.ps1") -Arguments $binaryArgs
Invoke-Step -Name "marketplace-plan-smoke" -ScriptPath $marketplaceSmokeScript
Invoke-Step -Name "install-smoke" -ScriptPath (Join-Path $scriptDir "smoke-install.ps1")

$manifest = Get-Content -LiteralPath (Join-Path $repoRoot ".codex-plugin/plugin.json") -Raw | ConvertFrom-Json
$defaultInstalledRoot = Join-Path $env:USERPROFILE (".codex/plugins/cache/oracle-local/codex_oracle/" + $manifest.version)
if ($SkipInstalledPlugin) {
  Write-Output "== installed-plugin =="
  Write-Output "Installed plugin verification skipped by -SkipInstalledPlugin."
} elseif (Test-Path -LiteralPath $defaultInstalledRoot) {
  Invoke-Step -Name "installed-plugin" -ScriptPath $installedVerifyScript -Arguments @{ PluginRoot = $defaultInstalledRoot }
} else {
  Write-Output "== installed-plugin =="
  Write-Output "Installed plugin verification skipped; default install not found: $defaultInstalledRoot"
}

Write-Output "Automatic verification passed"
