$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('oracle-install-' + [guid]::NewGuid())
$source = Join-Path $testRoot 'source'
$destination = Join-Path $testRoot 'plugins'
$marketplace = Join-Path $testRoot 'marketplace.json'
try {
  New-Item -ItemType Directory -Force "$source/.codex-plugin", "$source/bin", "$source/.work", "$source/unknown-private" | Out-Null
  Copy-Item "$repo/.codex-plugin/plugin.json" "$source/.codex-plugin/plugin.json"
  Copy-Item "$repo/.mcp.windows.json" "$source/.mcp.windows.json"
  Set-Content "$source/bin/fixture.txt" 'public artifact'
  foreach ($name in @('.env', 'config.toml', '.work/private.txt', 'unknown-private/private.txt')) {
    Set-Content (Join-Path $source $name) 'private fixture'
  }
  & "$PSScriptRoot/install-personal.ps1" -PluginRoot $source -PluginDestinationRoot $destination -MarketplacePath $marketplace | Out-Null
  if (Test-Path $destination) { throw 'Dry-run wrote the destination' }
  & "$PSScriptRoot/install-personal.ps1" -PluginRoot $source -PluginDestinationRoot $destination -MarketplacePath $marketplace -Apply | Out-Null
  $installed = Join-Path $destination 'codex_oracle'
  if (-not (Test-Path "$installed/bin/fixture.txt") -or -not (Test-Path "$installed/.mcp.json")) { throw 'Missing installed artifacts' }
  foreach ($name in @('.env', 'config.toml', '.work', 'unknown-private')) {
    if (Test-Path (Join-Path $installed $name)) { throw "Copied private root: $name" }
  }
  if ((Get-Content $marketplace -Raw | ConvertFrom-Json).plugins[0].name -ne 'codex_oracle') { throw 'Invalid marketplace entry' }
  $custom = Get-Content $marketplace -Raw | ConvertFrom-Json
  $custom | Add-Member -NotePropertyName customSetting -NotePropertyValue 'preserve'
  $custom.interface | Add-Member -NotePropertyName brandColor -NotePropertyValue '#123456'
  $custom.plugins = @([pscustomobject]@{name='before'}, $custom.plugins[0], [pscustomobject]@{name='after'})
  $custom | ConvertTo-Json -Depth 20 | Set-Content $marketplace
  & "$PSScriptRoot/install-personal.ps1" -PluginRoot $source -PluginDestinationRoot $destination -MarketplacePath $marketplace -Apply -Force | Out-Null
  $updated = Get-Content $marketplace -Raw | ConvertFrom-Json
  if ($updated.customSetting -ne 'preserve' -or $updated.interface.brandColor -ne '#123456') { throw 'Lost marketplace settings' }
  if (($updated.plugins.name -join ',') -ne 'before,codex_oracle,after') { throw 'Plugin order changed' }
  if (@(Get-ChildItem $destination -Directory -Filter '.oracle-backup-*').Count -ne 1) { throw 'Previous installation backup missing' }
  $oldMarketHash = (Get-FileHash $marketplace).Hash
  Set-Content "$source/bin/fixture.txt" 'new version fixture'
  # Allow reads but reject the atomic marketplace replacement after directory swap.
  $lock = [IO.File]::Open($marketplace, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
  $failed = $false
  try {
    try { & "$PSScriptRoot/install-personal.ps1" -PluginRoot $source -PluginDestinationRoot $destination -MarketplacePath $marketplace -Apply -Force | Out-Null }
    catch { $failed = $true }
  } finally { $lock.Dispose() }
  if (-not $failed) { throw 'Locked marketplace unexpectedly replaced' }
  if ((Get-Content "$installed/bin/fixture.txt" -Raw).Trim() -ne 'public artifact') { throw 'Previous installation was not restored' }
  if ((Get-FileHash $marketplace).Hash -ne $oldMarketHash) { throw 'Failed update changed marketplace' }
  if (@(Get-ChildItem $destination -Directory -Filter '.oracle-stage-*').Count) { throw 'Staging folder leaked' }
  Set-Content "$source/.mcp.windows.json" 'invalid JSON'
  $failed = $false
  try { & "$PSScriptRoot/install-personal.ps1" -PluginRoot $source -PluginDestinationRoot $destination -MarketplacePath $marketplace -Apply -Force | Out-Null }
  catch { $failed = $true }
  if (-not $failed -or (Get-Content "$installed/bin/fixture.txt" -Raw).Trim() -ne 'public artifact') { throw 'Invalid source damaged installed version' }
  $rejected = $false
  try {
    & "$PSScriptRoot/install-personal.ps1" -PluginRoot $installed -PluginDestinationRoot $destination -MarketplacePath $marketplace -Apply -Force | Out-Null
  } catch {
    if ($_.Exception.Message -notmatch 'must not overlap') { throw }
    $rejected = $true
  }
  if (-not $rejected -or -not (Test-Path "$installed/.codex-plugin/plugin.json")) { throw 'Self-install damaged its source' }
  Write-Output 'Install smoke passed: dry-run, private exclusions, settings/order preservation, retained backup, failed replacement rollback, invalid source rejection, self-install rejection.'
} finally {
  if (Test-Path $testRoot) {
    $resolved = (Resolve-Path $testRoot).Path
    $tempPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
    if (-not $resolved.StartsWith($tempPrefix, [StringComparison]::OrdinalIgnoreCase) -or (Split-Path $resolved -Leaf) -notlike 'oracle-install-*') { throw 'Refusing unsafe test cleanup' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
  }
}
