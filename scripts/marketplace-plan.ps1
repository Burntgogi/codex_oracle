param(
  [string]$PluginRoot = "",
  [string]$MarketplacePath = "",
  [string]$PluginDestinationRoot = "",
  [string]$MarketplaceName = "personal",
  [switch]$AsJson
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

$manifestPath = Join-Path $PluginRoot ".codex-plugin\plugin.json"
if (-not (Test-Path -LiteralPath $manifestPath)) {
  throw "Plugin manifest not found: $manifestPath"
}
$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
$pluginName = [string]$manifest.name
if ([string]::IsNullOrWhiteSpace($pluginName)) {
  throw "Plugin manifest name is empty: $manifestPath"
}

$category = "Productivity"
if ($null -ne $manifest.interface -and $null -ne $manifest.interface.PSObject.Properties["category"]) {
  $manifestCategory = [string]$manifest.interface.category
  if (-not [string]::IsNullOrWhiteSpace($manifestCategory)) {
    $category = $manifestCategory
  }
}

function Convert-ToDisplayName {
  param([string]$Name)
  $parts = $Name -split "[-_]+"
  $textInfo = [Globalization.CultureInfo]::InvariantCulture.TextInfo
  return (($parts | Where-Object { $_ } | ForEach-Object { $textInfo.ToTitleCase($_.ToLowerInvariant()) }) -join " ")
}

function Get-Prop {
  param(
    [object]$Object,
    [string]$Name
  )
  if ($null -eq $Object) {
    return $null
  }
  $prop = $Object.PSObject.Properties[$Name]
  if ($null -eq $prop) {
    return $null
  }
  return $prop.Value
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
  category = $category
}

$marketplaceExists = Test-Path -LiteralPath $MarketplacePath
$existingPluginNames = @()
if ($marketplaceExists) {
  $existing = Get-Content -Raw -LiteralPath $MarketplacePath | ConvertFrom-Json
  $existingName = [string](Get-Prop -Object $existing -Name "name")
  if ([string]::IsNullOrWhiteSpace($existingName)) {
    throw "Existing marketplace has no top-level name: $MarketplacePath"
  }
  $resolvedMarketplaceName = $existingName
  $interface = Get-Prop -Object $existing -Name "interface"
  $displayName = [string](Get-Prop -Object $interface -Name "displayName")
  if ([string]::IsNullOrWhiteSpace($displayName)) {
    $displayName = Convert-ToDisplayName -Name $resolvedMarketplaceName
  }
  $plugins = @()
  $existingPlugins = Get-Prop -Object $existing -Name "plugins"
  if ($null -ne $existingPlugins) {
    foreach ($plugin in @($existingPlugins)) {
      $name = [string](Get-Prop -Object $plugin -Name "name")
      if (-not [string]::IsNullOrWhiteSpace($name)) {
        $existingPluginNames += $name
      }
      if ($name -ne $pluginName) {
        $plugins += $plugin
      }
    }
  }
} else {
  $resolvedMarketplaceName = $MarketplaceName
  $displayName = Convert-ToDisplayName -Name $resolvedMarketplaceName
  $plugins = @()
}
$plugins += $entry

$pluginCopyTarget = Join-Path $PluginDestinationRoot $pluginName
$proposedMarketplace = [ordered]@{
  name = $resolvedMarketplaceName
  interface = [ordered]@{
    displayName = $displayName
  }
  plugins = @($plugins)
}

$entryAction = "append"
if ($existingPluginNames -contains $pluginName) {
  $entryAction = "replace-existing-entry-after-approval"
}

$plan = [ordered]@{
  status = "dry-run"
  willWrite = $false
  approvalRequired = $true
  pluginName = $pluginName
  pluginRoot = $PluginRoot
  pluginCopyTarget = $pluginCopyTarget
  marketplacePath = $MarketplacePath
  marketplaceExists = $marketplaceExists
  marketplaceName = $resolvedMarketplaceName
  entryAction = $entryAction
  entry = $entry
  proposedMarketplace = $proposedMarketplace
  approvalScope = @(
    "Copy or synchronize $PluginRoot to $pluginCopyTarget",
    "Create or update $MarketplacePath with the proposed marketplace JSON",
    "Install with: codex plugin add $pluginName@$resolvedMarketplaceName"
  )
}

if ($AsJson) {
  $plan | ConvertTo-Json -Depth 20
  exit 0
}

Write-Output "Marketplace install plan only; no files were written."
Write-Output ("Plugin: {0}" -f $plan.pluginName)
Write-Output ("Plugin source: {0}" -f $plan.pluginRoot)
Write-Output ("Plugin copy target: {0}" -f $plan.pluginCopyTarget)
Write-Output ("Marketplace target: {0}" -f $plan.marketplacePath)
Write-Output ("Marketplace exists: {0}" -f $plan.marketplaceExists)
Write-Output ("Entry action: {0}" -f $plan.entryAction)
Write-Output "Approval is required before copying the plugin or writing marketplace.json."
Write-Output ""
$plan.proposedMarketplace | ConvertTo-Json -Depth 20
