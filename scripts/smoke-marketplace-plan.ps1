param(
  [string]$PlannerPath = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir
if ([string]::IsNullOrWhiteSpace($PlannerPath)) {
  $PlannerPath = Join-Path $scriptDir "marketplace-plan.ps1"
}
if (-not (Test-Path -LiteralPath $PlannerPath)) {
  throw "Marketplace planner not found: $PlannerPath"
}

$tempRoot = Join-Path $env:TEMP ("codex_oracle-marketplace-smoke-" + [guid]::NewGuid().ToString())
$marketplacePath = Join-Path $tempRoot ".agents\plugins\marketplace.json"
$pluginDestinationRoot = Join-Path $tempRoot "plugins"

try {
  $json = & $PlannerPath `
    -PluginRoot $repoRoot `
    -MarketplacePath $marketplacePath `
    -PluginDestinationRoot $pluginDestinationRoot `
    -AsJson
  if ($LASTEXITCODE -ne 0) {
    throw "Marketplace planner exited with code $LASTEXITCODE"
  }
  $plan = $json | ConvertFrom-Json

  if ($plan.status -ne "dry-run") {
    throw "status is $($plan.status); expected dry-run"
  }
  if ($plan.willWrite -ne $false) {
    throw "willWrite is $($plan.willWrite); expected false"
  }
  if ($plan.approvalRequired -ne $true) {
    throw "approvalRequired is $($plan.approvalRequired); expected true"
  }
  if ($plan.pluginName -ne "codex_oracle") {
    throw "pluginName is $($plan.pluginName); expected codex_oracle"
  }
  if ($plan.marketplacePath -ne $marketplacePath) {
    throw "marketplacePath is $($plan.marketplacePath); expected $marketplacePath"
  }
  if ($plan.pluginCopyTarget -ne (Join-Path $pluginDestinationRoot "codex_oracle")) {
    throw "pluginCopyTarget is $($plan.pluginCopyTarget); expected destination under temp plugins"
  }
  if ($plan.entry.name -ne "codex_oracle") {
    throw "entry.name is $($plan.entry.name); expected codex_oracle"
  }
  if ($plan.entry.source.source -ne "local") {
    throw "entry.source.source is $($plan.entry.source.source); expected local"
  }
  if ($plan.entry.source.path -ne "./plugins/codex_oracle") {
    throw "entry.source.path is $($plan.entry.source.path); expected ./plugins/codex_oracle"
  }
  if ($plan.entry.policy.installation -ne "AVAILABLE") {
    throw "installation policy is $($plan.entry.policy.installation); expected AVAILABLE"
  }
  if ($plan.entry.policy.authentication -ne "ON_INSTALL") {
    throw "authentication policy is $($plan.entry.policy.authentication); expected ON_INSTALL"
  }
  if ($plan.proposedMarketplace.name -ne "personal") {
    throw "marketplace name is $($plan.proposedMarketplace.name); expected personal"
  }
  if (Test-Path -LiteralPath $marketplacePath) {
    throw "planner wrote marketplace file during dry-run: $marketplacePath"
  }
  if (Test-Path -LiteralPath (Join-Path $pluginDestinationRoot "codex_oracle")) {
    throw "planner copied plugin during dry-run"
  }

  Write-Output "Marketplace plan smoke passed"
} finally {
  if (Test-Path -LiteralPath $tempRoot) {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force
  }
}
