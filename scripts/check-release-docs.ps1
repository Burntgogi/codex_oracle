param(
  [string]$Root = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
if ([string]::IsNullOrWhiteSpace($Root)) {
  $Root = Split-Path -Parent $scriptDir
}
$Root = (Resolve-Path -LiteralPath $Root).Path

$manifestPath = Join-Path $Root ".codex-plugin\plugin.json"
$mcpPath = Join-Path $Root ".mcp.json"
if (-not (Test-Path -LiteralPath $manifestPath)) {
  throw "plugin manifest not found: $manifestPath"
}
if (-not (Test-Path -LiteralPath $mcpPath)) {
  throw ".mcp.json not found: $mcpPath"
}

$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
if ($manifest.name -ne "codex_oracle") {
  throw "manifest name is $($manifest.name); expected codex_oracle"
}

$mcp = Get-Content -Raw -LiteralPath $mcpPath | ConvertFrom-Json
$server = $mcp.mcpServers.'codex_oracle'
if ($null -eq $server) {
  throw ".mcp.json does not define mcpServers.codex_oracle"
}
if ($server.command -notin @("./bin/codex_oracle.exe", "./bin/codex_oracle")) {
  throw ".mcp.json command is not a packaged Oracle binary: $($server.command)"
}

$scanRoots = @(
  "README.md",
  ".codex-plugin\plugin.json",
  ".mcp.json",
  "skills",
  "scripts",
  "doc/workflow.md",
  "doc/chrome-assisted-workflow.md",
  "doc/install.md"
)

$files = @()
foreach ($relative in $scanRoots) {
  $path = Join-Path $Root $relative
  if (-not (Test-Path -LiteralPath $path)) {
    continue
  }
  $item = Get-Item -LiteralPath $path
  if ($item.PSIsContainer) {
    $files += Get-ChildItem -LiteralPath $path -Recurse -File |
      Where-Object {
        $_.Name -notin @("check-release-docs.ps1", "verify-release.ps1", "build-dist.ps1")
      }
  } else {
    $files += $item
  }
}

$stalePatterns = @(
  "Codex Oracle Go",
  "codex-oracle-go",
  "bin\codex-oracle-go.exe",
  "cmd\codex-oracle-go",
  ".codex-oracle-go",
  "scripts\login-setup.ps1",
  "scripts\live-smoke.ps1",
  "smoke-live-"
)

foreach ($file in $files) {
  $text = Get-Content -Raw -LiteralPath $file.FullName
  foreach ($pattern in $stalePatterns) {
    if ($text.Contains($pattern)) {
      $relative = [IO.Path]::GetRelativePath($Root, $file.FullName)
      throw "stale release reference '$pattern' in $relative"
    }
  }
}

$readme = Get-Content -Raw -LiteralPath (Join-Path $Root "README.md")
foreach ($required in @("codex_oracle", "chrome@openai-bundled", "ORACLE_SESSION_CONTENT_MODE", "session_delete")) {
  if (-not $readme.Contains($required)) {
    throw "README.md missing required release contract text: $required"
  }
}

Write-Output "Release docs check passed: $Root"
