param(
  [string]$PluginRoot = "",
  [string]$MarketplacePath = "",
  [string]$PluginDestinationRoot = "",
  [string]$MarketplaceName = "personal",
  [switch]$Apply,
  [switch]$Force
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptDir "marketplace-path.ps1")
$repoRoot = Split-Path -Parent $scriptDir
if ([string]::IsNullOrWhiteSpace($PluginRoot)) {
  $PluginRoot = $repoRoot
}
$PluginRoot = (Resolve-Path -LiteralPath $PluginRoot).Path

if ([string]::IsNullOrWhiteSpace($MarketplacePath)) {
  $MarketplacePath = Join-Path $HOME ".codex\local-marketplaces\$MarketplaceName\.agents\plugins\marketplace.json"
}
if ([string]::IsNullOrWhiteSpace($PluginDestinationRoot)) {
  $PluginDestinationRoot = Join-Path $HOME ".codex\local-marketplaces\$MarketplaceName\plugins"
}
$PluginDestinationRoot = [System.IO.Path]::GetFullPath($PluginDestinationRoot)

function Get-Prop {
  param([object]$Object, [string]$Name)
  if ($null -eq $Object) { return $null }
  $prop = $Object.PSObject.Properties[$Name]
  if ($null -eq $prop) { return $null }
  return $prop.Value
}

function Assert-SafeRecursiveTarget {
  param([string]$Destination, [string]$AllowedRoot)

  $fullDestination = [System.IO.Path]::GetFullPath($Destination)
  $fullAllowedRoot = [System.IO.Path]::GetFullPath($AllowedRoot)
  $comparison = [System.StringComparison]::OrdinalIgnoreCase
  if ([string]::IsNullOrWhiteSpace($fullDestination)) {
    throw "Refusing to remove an empty destination path."
  }
  if ($fullDestination.Equals($fullAllowedRoot, $comparison)) {
    throw "Refusing to remove the plugin destination root itself: $fullDestination"
  }
  $allowedPrefix = $fullAllowedRoot.TrimEnd([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
  if (-not $fullDestination.StartsWith($allowedPrefix, $comparison)) {
    throw "Refusing to remove a path outside the approved plugin destination root: $fullDestination"
  }
}

function Copy-DirectoryClean {
  param([string]$Source, [string]$Destination, [string]$AllowedRoot)
  $sourcePath = [IO.Path]::GetFullPath($Source).TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
  $destinationPath = [IO.Path]::GetFullPath($Destination).TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
  if ($sourcePath.StartsWith($destinationPath, [StringComparison]::OrdinalIgnoreCase) -or $destinationPath.StartsWith($sourcePath, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Plugin source and destination must not overlap.'
  }
  if (Test-Path -LiteralPath $Destination) { throw 'Staging directory already exists.' }
  New-Item -ItemType Directory -Force -Path $Destination | Out-Null
  $includedNames = @(".codex-plugin", "assets", "bin", "doc", "docs", "scripts", "skills", ".mcp.json", ".mcp.windows.json", ".mcp.macos-amd64.json", ".mcp.macos-arm64.json", "README.md", "README.en.md", "RELEASE_NOTES.md", "RELEASE_NOTES.en.md", "CHANGELOG.md", "LICENSE", "SECURITY.md", "SHA256SUMS.txt")
  Get-ChildItem -LiteralPath $Source -Force | Where-Object {
    $includedNames -contains $_.Name
  } | ForEach-Object {
    Copy-Item -LiteralPath $_.FullName -Destination $Destination -Recurse -Force
  }
}

$manifestPath = Join-Path $PluginRoot ".codex-plugin\plugin.json"
if (-not (Test-Path -LiteralPath $manifestPath)) {
  throw "Plugin manifest not found: $manifestPath"
}
$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
$pluginName = [string]$manifest.name
if ($pluginName -ne "codex_oracle") {
  throw "Unexpected plugin name '$pluginName'; expected codex_oracle"
}

$pluginCopyTarget = Join-Path $PluginDestinationRoot $pluginName
$pluginCopyTarget = [System.IO.Path]::GetFullPath($pluginCopyTarget)
$pluginSourcePath = Get-LocalPluginSourcePath -MarketplacePath $MarketplacePath -PluginTarget $pluginCopyTarget
$mcpTemplate = Join-Path $PluginRoot ".mcp.windows.json"
if (-not (Test-Path -LiteralPath $mcpTemplate)) {
  throw "Windows MCP template not found: $mcpTemplate"
}

$entry = [ordered]@{
  name = $pluginName
  source = [ordered]@{
    source = "local"
    path = $pluginSourcePath
  }
  policy = [ordered]@{
    installation = "AVAILABLE"
    authentication = "ON_INSTALL"
  }
  category = "Productivity"
}

$plan = [ordered]@{
  status = if ($Apply) { "apply" } else { "dry-run" }
  willWrite = [bool]$Apply
  pluginRoot = $PluginRoot
  pluginCopyTarget = $pluginCopyTarget
  marketplacePath = $MarketplacePath
  marketplaceName = $MarketplaceName
  mcpTemplate = $mcpTemplate
  entry = $entry
  approvalRequired = -not $Apply
}

if (-not $Apply) {
  $plan | ConvertTo-Json -Depth 12
  exit 0
}

$marketplaceDir = Split-Path -Parent $MarketplacePath
$sourcePrefix = $PluginRoot.TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
$targetPrefix = $pluginCopyTarget.TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
if ($sourcePrefix.StartsWith($targetPrefix, [StringComparison]::OrdinalIgnoreCase) -or $targetPrefix.StartsWith($sourcePrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Plugin source and destination must not overlap.' }
Assert-SafeRecursiveTarget -Destination $pluginCopyTarget -AllowedRoot $PluginDestinationRoot
if ((Test-Path -LiteralPath $pluginCopyTarget) -and -not $Force) { throw "Target already exists. Re-run with -Force after approval: $pluginCopyTarget" }
$replaced = $false

if (Test-Path -LiteralPath $MarketplacePath) {
  $existing = Get-Content -Raw -LiteralPath $MarketplacePath | ConvertFrom-Json
  $resolvedName = [string](Get-Prop -Object $existing -Name "name")
  if ([string]::IsNullOrWhiteSpace($resolvedName)) {
    $resolvedName = $MarketplaceName
  }
  $interface = Get-Prop -Object $existing -Name "interface"
  $displayName = [string](Get-Prop -Object $interface -Name "displayName")
  if ([string]::IsNullOrWhiteSpace($displayName)) {
    $displayName = "Personal"
  }
  $plugins = @()
  foreach ($plugin in @((Get-Prop -Object $existing -Name "plugins"))) {
    $name = [string](Get-Prop -Object $plugin -Name "name")
    if ($name -ne $pluginName) {
      $plugins += $plugin
    } elseif (-not $replaced) {
      $plugins += $entry
      $replaced = $true
    }
  }
} else {
  $resolvedName = $MarketplaceName
  $displayName = "Personal"
  $interface = $null
  $plugins = @()
}
if (-not $replaced) { $plugins += $entry }

$marketplace = if (Test-Path -LiteralPath $MarketplacePath) { $existing } else { [pscustomobject]@{} }
if ($null -eq $interface) { $interface = [pscustomobject]@{ displayName = $displayName } }
$marketplace | Add-Member -NotePropertyName name -NotePropertyValue $resolvedName -Force
$marketplace | Add-Member -NotePropertyName interface -NotePropertyValue $interface -Force
$marketplace | Add-Member -NotePropertyName plugins -NotePropertyValue @($plugins) -Force
$marketplaceJson = ($marketplace | ConvertTo-Json -Depth 20) + "`n"
$utf8NoBom = New-Object System.Text.UTF8Encoding -ArgumentList $false
$token = [guid]::NewGuid().ToString('N')
$stage = Join-Path $PluginDestinationRoot ('.oracle-stage-' + $token)
$backup = Join-Path $PluginDestinationRoot ('.oracle-backup-' + $token)
$marketplaceTemp = Join-Path $marketplaceDir ('.oracle-marketplace-' + $token + '.json')
$marketplaceBackup = Join-Path $marketplaceDir ('.oracle-marketplace-backup-' + $token + '.json')
$movedOld = $false
$installedNew = $false
try {
  Copy-DirectoryClean -Source $PluginRoot -Destination $stage -AllowedRoot $PluginDestinationRoot
  foreach ($file in Get-ChildItem -LiteralPath $stage -Recurse -File -Force) {
    $relative = $file.FullName.Substring($stage.Length + 1)
    if ((Get-FileHash -LiteralPath $file.FullName).Hash -ne (Get-FileHash -LiteralPath (Join-Path $PluginRoot $relative)).Hash) { throw "Staged file differs: $relative" }
  }
  $null = Get-Content -LiteralPath (Join-Path $stage '.mcp.windows.json') -Raw | ConvertFrom-Json
  Copy-Item -LiteralPath (Join-Path $stage '.mcp.windows.json') -Destination (Join-Path $stage '.mcp.json') -Force
  New-Item -ItemType Directory -Force -Path $marketplaceDir | Out-Null
  [IO.File]::WriteAllText($marketplaceTemp, $marketplaceJson, $utf8NoBom)
  if (Test-Path -LiteralPath $pluginCopyTarget) {
    Move-Item -LiteralPath $pluginCopyTarget -Destination $backup
    $movedOld = $true
  }
  Move-Item -LiteralPath $stage -Destination $pluginCopyTarget
  $installedNew = $true
  if (Test-Path -LiteralPath $MarketplacePath) {
    [IO.File]::Replace($marketplaceTemp, $MarketplacePath, $marketplaceBackup)
  } else {
    [IO.File]::Move($marketplaceTemp, $MarketplacePath)
  }
} catch {
  if ($installedNew) {
    Assert-SafeRecursiveTarget -Destination $pluginCopyTarget -AllowedRoot $PluginDestinationRoot
    Remove-Item -LiteralPath $pluginCopyTarget -Recurse -Force
  }
  if ($movedOld) { Move-Item -LiteralPath $backup -Destination $pluginCopyTarget }
  throw
} finally {
  if (Test-Path -LiteralPath $stage) {
    Assert-SafeRecursiveTarget -Destination $stage -AllowedRoot $PluginDestinationRoot
    Remove-Item -LiteralPath $stage -Recurse -Force
  }
  if (Test-Path -LiteralPath $marketplaceTemp) { Remove-Item -LiteralPath $marketplaceTemp -Force }
}
if ($movedOld) { Write-Output "Previous installation retained at $backup" }

Write-Output "Installed codex_oracle plugin files to $pluginCopyTarget"
Write-Output "Updated marketplace file $MarketplacePath"
Write-Output "Next: codex plugin add codex_oracle@$resolvedName"
