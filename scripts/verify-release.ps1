param(
  [string]$Root = "",
  [string]$Version = "",
  [switch]$SkipManualChromeEvidence
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
if ([string]::IsNullOrWhiteSpace($Root)) {
  $Root = Split-Path -Parent $scriptDir
}
$Root = (Resolve-Path -LiteralPath $Root).Path
$manifest = Get-Content -LiteralPath (Join-Path $Root ".codex-plugin/plugin.json") -Raw | ConvertFrom-Json
if (-not $Version) { $Version = $manifest.version }
if ($Version -ne $manifest.version) { throw "Version does not match manifest: $($manifest.version)" }

$expectedDistName = "codex_oracle-" + $Version
$rootName = Split-Path -Leaf $Root
if ($rootName -eq $expectedDistName -and (Test-Path -LiteralPath (Join-Path $Root "bin\codex_oracle.exe"))) {
  $distRoot = $Root
  $sourceRoot = $null
} else {
  $sourceRoot = $Root
  $distRoot = Join-Path $Root ("dist\" + $expectedDistName)
}

if (-not (Test-Path -LiteralPath $distRoot)) {
  throw "Release package not found: $distRoot. Run scripts\build-dist.ps1 first."
}
$distRoot = (Resolve-Path -LiteralPath $distRoot).Path

$checkDocs = Join-Path $scriptDir "check-release-docs.ps1"
$verifyInstalled = Join-Path $scriptDir "verify-installed.ps1"
if (-not (Test-Path -LiteralPath $checkDocs)) {
  throw "check-release-docs.ps1 not found: $checkDocs"
}
if (-not (Test-Path -LiteralPath $verifyInstalled)) {
  throw "verify-installed.ps1 not found: $verifyInstalled"
}

if ($null -ne $sourceRoot) {
  & $checkDocs -Root $sourceRoot
}
& $checkDocs -Root $distRoot

foreach ($relative in @("mcp-go", "legacy", ".tmp", "dist", "doc\plans", "bin\codex-oracle-go.exe")) {
  $path = Join-Path $distRoot $relative
  if (Test-Path -LiteralPath $path) {
    throw "Forbidden release path included: $relative"
  }
}

$sumPath = Join-Path $distRoot "SHA256SUMS.txt"
if (-not (Test-Path -LiteralPath $sumPath)) { throw "SHA256SUMS.txt missing from release package" }
$checked = @{}
foreach ($line in Get-Content -LiteralPath $sumPath) {
  if ($line -notmatch '^([a-f0-9]{64})  (.+)$') { throw "Malformed checksum: $line" }
  $expected, $relative = $Matches[1], $Matches[2]
  $path = [IO.Path]::GetFullPath((Join-Path $distRoot $relative))
  if (-not $path.StartsWith($distRoot.TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or $checked.ContainsKey($path)) { throw "Invalid or duplicate checksum path: $relative" }
  if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) { throw "Checksum mismatch: $relative" }
  $checked[$path] = $true
}
foreach ($file in Get-ChildItem -LiteralPath $distRoot -Recurse -File) {
  if ($file.FullName -ne $sumPath -and -not $checked.ContainsKey($file.FullName)) { throw "Untracked package file: $($file.FullName)" }
}
$provenance = Get-Content -LiteralPath (Join-Path $distRoot "provenance.json") -Raw | ConvertFrom-Json
if ($provenance.version -ne $Version -or $provenance.sourceDirectory -ne "source") { throw "Package provenance/version mismatch" }
foreach ($source in $provenance.sourceFiles) {
  $sourcePath = Join-Path $distRoot ("source/" + $source.path)
  if ((Get-FileHash -LiteralPath $sourcePath -Algorithm SHA256).Hash.ToLowerInvariant() -ne $source.sha256) { throw "Source provenance mismatch: $($source.path)" }
}
foreach ($arch in @("amd64", "arm64")) {
  $platformRoot = Join-Path $distRoot "platforms/darwin-$arch"
  & $checkDocs -Root $platformRoot
  $config = Get-Content -LiteralPath (Join-Path $platformRoot ".mcp.json") -Raw | ConvertFrom-Json
  if ($config.mcpServers.codex_oracle.command -ne "./bin/codex_oracle") { throw "Incorrect macOS launch command: $arch" }
  $macBinary = Join-Path $platformRoot "bin/codex_oracle"
  if ((Get-FileHash -LiteralPath $macBinary).Hash -ne (Get-FileHash -LiteralPath (Join-Path $distRoot "bin/codex_oracle_darwin_$arch")).Hash) { throw "macOS binary copy mismatch: $arch" }
  $header = [IO.File]::ReadAllBytes($macBinary)
  $cpu = if ($arch -eq "amd64") { 0x01000007 } else { 0x0100000c }
  if ([BitConverter]::ToUInt32($header, 0) -ne 4277009103 -or [BitConverter]::ToUInt32($header, 4) -ne $cpu) { throw "Not the expected macOS $arch executable" }
}
$binary = Join-Path $distRoot "bin/codex_oracle.exe"
$response = '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | & $binary | ConvertFrom-Json
if ($LASTEXITCODE -ne 0 -or $response.result.serverInfo.version -ne $Version) { throw "Binary/manifest version mismatch" }

& $verifyInstalled -PluginRoot $distRoot -SkipCodexList
& (Join-Path $distRoot "scripts/smoke-recovery.ps1") -BinaryPath $binary

if (-not $SkipManualChromeEvidence) {
  $evidencePath = Join-Path $distRoot "doc\evidence\manual-chrome-smoke.md"
  if (-not (Test-Path -LiteralPath $evidencePath)) {
    throw "Manual @Chrome evidence missing: $evidencePath. Use -SkipManualChromeEvidence only for source iteration, not broad rollout."
  }
}

Write-Output "Release verification passed: $distRoot"
