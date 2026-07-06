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
  if (Test-Path -LiteralPath $Destination) {
    if (-not $Force) {
      throw "Target already exists. Re-run with -Force after approval: $Destination"
    }
    Assert-SafeRecursiveTarget -Destination $Destination -AllowedRoot $AllowedRoot
    Remove-Item -LiteralPath $Destination -Recurse -Force
  }
  New-Item -ItemType Directory -Force -Path $Destination | Out-Null
  $excludedNames = @(".git", ".codex_oracle", ".tmp", "tmp", "sessions", "Docs_local", "docs_local")
  Get-ChildItem -LiteralPath $Source -Force | Where-Object {
    $excludedNames -notcontains $_.Name
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
$mcpTemplate = Join-Path $PluginRoot ".mcp.windows.json"
if (-not (Test-Path -LiteralPath $mcpTemplate)) {
  throw "Windows MCP template not found: $mcpTemplate"
}

$entry = [ordered]@{
  name = $pluginName
  source = [ordered]@{
    source = "local"
    path = "./plugins/$pluginName"
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
  approvalRequired = -not $Apply
}

if (-not $Apply) {
  $plan | ConvertTo-Json -Depth 12
  exit 0
}

Copy-DirectoryClean -Source $PluginRoot -Destination $pluginCopyTarget -AllowedRoot $PluginDestinationRoot
Copy-Item -LiteralPath (Join-Path $pluginCopyTarget ".mcp.windows.json") -Destination (Join-Path $pluginCopyTarget ".mcp.json") -Force

$marketplaceDir = Split-Path -Parent $MarketplacePath
New-Item -ItemType Directory -Force -Path $marketplaceDir | Out-Null

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
    }
  }
} else {
  $resolvedName = $MarketplaceName
  $displayName = "Personal"
  $plugins = @()
}
$plugins += $entry

$marketplace = [ordered]@{
  name = $resolvedName
  interface = [ordered]@{
    displayName = $displayName
  }
  plugins = @($plugins)
}
$marketplaceJson = ($marketplace | ConvertTo-Json -Depth 20) + "`n"
$utf8NoBom = New-Object System.Text.UTF8Encoding -ArgumentList $false
[System.IO.File]::WriteAllText($MarketplacePath, $marketplaceJson, $utf8NoBom)

Write-Output "Installed codex_oracle plugin files to $pluginCopyTarget"
Write-Output "Updated marketplace file $MarketplacePath"
Write-Output "Next: codex plugin add codex_oracle@$resolvedName"
