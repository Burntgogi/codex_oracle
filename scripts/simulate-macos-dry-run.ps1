param(
  [ValidateSet("arm64", "aarch64", "x86_64", "amd64")]
  [string]$Arch = "arm64",
  [string]$PluginRoot = "",
  [string]$MacHome = "/Users/example",
  [string]$MarketplaceName = "personal"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir
if ([string]::IsNullOrWhiteSpace($PluginRoot)) {
  $PluginRoot = $repoRoot
}
$PluginRoot = (Resolve-Path -LiteralPath $PluginRoot).Path

$manifestPath = Join-Path $PluginRoot ".codex-plugin\plugin.json"
if (-not (Test-Path -LiteralPath $manifestPath)) {
  throw "Plugin manifest not found: $manifestPath"
}
$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
$pluginName = [string]$manifest.name
if ($pluginName -ne "codex_oracle") {
  throw "Unexpected plugin name '$pluginName'; expected codex_oracle"
}

switch ($Arch) {
  "arm64" {
    $mcpTemplate = ".mcp.macos-arm64.json"
    $binaryRelative = "bin/codex_oracle_darwin_arm64"
  }
  "aarch64" {
    $mcpTemplate = ".mcp.macos-arm64.json"
    $binaryRelative = "bin/codex_oracle_darwin_arm64"
  }
  default {
    $mcpTemplate = ".mcp.macos-amd64.json"
    $binaryRelative = "bin/codex_oracle_darwin_amd64"
  }
}

$mcpPath = Join-Path $PluginRoot $mcpTemplate
$binaryPath = Join-Path $PluginRoot $binaryRelative
if (-not (Test-Path -LiteralPath $mcpPath)) {
  throw "MCP template not found: $mcpPath"
}
if (-not (Test-Path -LiteralPath $binaryPath)) {
  throw "macOS binary not found: $binaryPath"
}

$mcp = Get-Content -Raw -LiteralPath $mcpPath | ConvertFrom-Json
$command = [string]$mcp.mcpServers.codex_oracle.command
$expectedCommand = "./$binaryRelative"
if ($command -ne $expectedCommand) {
  throw "$mcpTemplate command is '$command'; expected '$expectedCommand'"
}

$bytes = [System.IO.File]::ReadAllBytes($binaryPath)
if ($bytes.Length -lt 4) {
  throw "macOS binary is too small: $binaryPath"
}
$magic = ($bytes[0..3] | ForEach-Object { $_.ToString("X2") }) -join ""
if ($magic -ne "CFFAEDFE") {
  throw "$binaryRelative does not look like a little-endian Mach-O executable; magic=$magic"
}

[ordered]@{
  status = "dry-run"
  willWrite = $false
  approvalRequired = $true
  simulatedOS = "Darwin"
  arch = $Arch
  pluginRoot = $PluginRoot
  pluginCopyTarget = "$MacHome/.codex/local-marketplaces/$MarketplaceName/plugins/$pluginName"
  marketplacePath = "$MacHome/.codex/local-marketplaces/$MarketplaceName/.agents/plugins/marketplace.json"
  marketplaceName = $MarketplaceName
  mcpTemplate = $mcpTemplate
  mcpCommand = $command
  binaryFormat = "Mach-O"
} | ConvertTo-Json -Depth 12
