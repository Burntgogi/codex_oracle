param([string]$PluginRoot = '')
$ErrorActionPreference = 'Stop'
$listing = & codex plugin list --json | ConvertFrom-Json
if ($LASTEXITCODE -ne 0) { throw 'Cannot read Codex plugin registration.' }
$registered = @($listing.installed | Where-Object pluginId -eq 'codex_oracle@personal')
if ($registered.Count -ne 1) { throw 'codex_oracle@personal is not registered in this Codex environment.' }
if (-not $PluginRoot) {
  $PluginRoot = Join-Path $env:USERPROFILE ('.codex/plugins/cache/personal/codex_oracle/' + $registered[0].version)
}
$manifest = Get-Content -LiteralPath (Join-Path $PluginRoot '.codex-plugin/plugin.json') -Raw | ConvertFrom-Json
$installedHash = (Get-FileHash -LiteralPath (Join-Path $PluginRoot 'bin/codex_oracle.exe')).Hash
$running = @()
$inspection = 'available'
try {
  foreach ($process in @(Get-CimInstance Win32_Process -Filter "Name = 'codex_oracle.exe'")) {
    if (-not $process.ExecutablePath) { $inspection = 'incomplete'; continue }
    $binary = $process.ExecutablePath
    $runtimeManifest = Join-Path (Split-Path (Split-Path $binary -Parent) -Parent) '.codex-plugin/plugin.json'
    $runtimeVersion = if (Test-Path -LiteralPath $runtimeManifest) { (Get-Content -LiteralPath $runtimeManifest -Raw | ConvertFrom-Json).version } else { 'unknown' }
    $running += [pscustomobject]@{ pid=$process.ProcessId; version=$runtimeVersion; matchesInstalled=($runtimeVersion -eq $manifest.version -and (Get-FileHash -LiteralPath $binary).Hash -eq $installedHash) }
  }
} catch { $inspection = 'unavailable' }
$restart = @($running | Where-Object { -not $_.matchesInstalled }).Count -gt 0
$status = if ($restart) { 'restart-required' } elseif ($inspection -ne 'available') { 'runtime-unverified' } elseif ($running.Count -eq 0) { 'not-running' } else { 'matched' }
[pscustomobject]@{
  registeredVersion=$registered[0].version
  installedVersion=$manifest.version
  enabled=$registered[0].enabled
  running=$running
  processInspection=$inspection
  status=$status
  guidance= if ($restart) { 'Reload Codex MCP connections or restart Codex, then run this diagnostic again.' } elseif ($status -ne 'matched') { 'Invoke the Oracle tool in Codex and retry in an environment that can inspect processes.' } else { 'All observed Oracle processes match the installed version and binary. This does not identify which task owns each process.' }
} | ConvertTo-Json -Depth 6
