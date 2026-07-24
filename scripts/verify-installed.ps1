param(
  [string]$PluginRoot = "",
  [string]$PluginId = "codex_oracle@personal",
  [switch]$SkipCodexList
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if ([string]::IsNullOrWhiteSpace($PluginRoot)) {
  $PluginRoot = Join-Path $env:USERPROFILE ".codex\local-marketplaces\personal\plugins\codex_oracle"
}

if (-not (Test-Path -LiteralPath $PluginRoot)) {
  throw "Plugin root not found: $PluginRoot"
}
$PluginRoot = (Resolve-Path -LiteralPath $PluginRoot).Path

$binaryPath = Join-Path $PluginRoot "bin\codex_oracle.exe"
$mcpPath = Join-Path $PluginRoot ".mcp.json"
$readmePath = Join-Path $PluginRoot "README.md"

if (-not (Test-Path -LiteralPath $binaryPath)) {
  throw "Installed binary not found: $binaryPath"
}
if (-not (Test-Path -LiteralPath $mcpPath)) {
  throw "Installed .mcp.json not found: $mcpPath"
}
if (-not (Test-Path -LiteralPath $readmePath)) {
  throw "README fixture not found under plugin root: $readmePath"
}

$mcpText = Get-Content -LiteralPath $mcpPath -Raw
$mcp = $mcpText | ConvertFrom-Json
$server = $mcp.mcpServers.'codex_oracle'
if ($null -eq $server) {
  throw ".mcp.json does not define mcpServers.codex_oracle"
}
if ($server.command -ne "./bin/codex_oracle.exe") {
  throw ".mcp.json command is $($server.command); expected ./bin/codex_oracle.exe"
}
$envVars = @($server.env_vars)
if ($envVars -contains "OPENAI_API_KEY") {
  throw ".mcp.json forwards OPENAI_API_KEY; browser-only plugin must not expose it"
}
foreach ($required in @("ORACLE_HOME_DIR", "ORACLE_BROWSER_PROFILE_DIR", "ORACLE_CHATGPT_URL", "ORACLE_ENGINE", "ORACLE_SESSION_CONTENT_MODE")) {
  if ($envVars -notcontains $required) {
    throw ".mcp.json missing env var allowlist entry: $required"
  }
}

function Invoke-McpJsonLines {
  param(
    [string]$ExecutablePath,
    [string]$InputJson,
    [string]$WorkingDirectory = ""
  )

  $inputFile = Join-Path ([System.IO.Path]::GetTempPath()) ("codex_oracle-mcp-" + [guid]::NewGuid().ToString() + ".jsonl")
  $encoding = New-Object System.Text.UTF8Encoding -ArgumentList $false
  [System.IO.File]::WriteAllText($inputFile, $InputJson + "`n", $encoding)

  $oldLocation = (Get-Location).Path
  try {
    if (-not [string]::IsNullOrWhiteSpace($WorkingDirectory)) {
      Set-Location -LiteralPath $WorkingDirectory
    }
    $command = '"' + $ExecutablePath + '" < "' + $inputFile + '"'
    $output = & cmd.exe /d /c $command 2>&1
    $exitCode = $LASTEXITCODE
  } finally {
    Set-Location -LiteralPath $oldLocation
    if (Test-Path -LiteralPath $inputFile) {
      Remove-Item -LiteralPath $inputFile -Force
    }
  }

  if ($exitCode -ne 0) {
    $message = ($output | Out-String).Trim()
    if (-not [string]::IsNullOrWhiteSpace($message)) {
      Write-Error $message
    }
    throw "MCP binary exited with code $exitCode"
  }
  if ($null -eq $output -or $output.Count -eq 0) {
    return @()
  }
  return $output | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | ForEach-Object { $_ | ConvertFrom-Json }
}

if (-not $SkipCodexList) {
  $pluginList = & codex plugin list
  if ($LASTEXITCODE -ne 0) {
    throw "codex plugin list exited with code $LASTEXITCODE"
  }
  $matchingLine = $pluginList | Where-Object { $_ -match [regex]::Escape($PluginId) } | Select-Object -First 1
  if ([string]::IsNullOrWhiteSpace($matchingLine)) {
    throw "codex plugin list does not show $PluginId"
  }
  if ($matchingLine -notmatch "installed,\s*enabled") {
    throw "$PluginId is not installed and enabled: $matchingLine"
  }
}

$homeDir = Join-Path $env:TEMP ("codex_oracle-installed-verify-" + [guid]::NewGuid().ToString())
$oldHome = $env:ORACLE_HOME_DIR
try {
  $env:ORACLE_HOME_DIR = $homeDir
  $requests = @(
    @{ jsonrpc = "2.0"; id = 1; method = "tools/list"; params = @{} },
    @{ jsonrpc = "2.0"; id = 2; method = "tools/call"; params = @{ name = "consult"; arguments = @{ preset = "chatgpt-pro-heavy"; prompt = "installed plugin dry-run check"; files = @("README.md"); dryRun = $true } } },
    @{ jsonrpc = "2.0"; id = 3; method = "tools/call"; params = @{ name = "consult_prepare"; arguments = @{ preset = "chatgpt-pro-heavy"; prompt = "installed prepare check"; files = @("README.md"); browserThinkingTime = "extended" } } }
  )
  $inputJson = ($requests | ForEach-Object { $_ | ConvertTo-Json -Depth 12 -Compress }) -join "`n"
  Push-Location $PluginRoot
  try {
    $responses = @(Invoke-McpJsonLines -ExecutablePath $binaryPath -InputJson $inputJson -WorkingDirectory $PluginRoot)
  } finally {
    Pop-Location
  }

  $toolNames = @($responses[0].result.tools | ForEach-Object { $_.name })
  foreach ($name in @("consult", "consult_prepare", "consult_finalize", "sessions", "session_delete", "login_setup", "smoke_check", "reattach")) {
    if ($toolNames -notcontains $name) {
      throw "installed tools/list missing $name"
    }
  }
  if ($responses[1].result.structuredContent.status -ne "dry-run") {
    throw "installed consult dry-run did not return dry-run"
  }
  if ($responses[1].result.structuredContent.bundle.preview -notmatch "untrusted reference material") {
    throw "installed consult dry-run preview missing untrusted-context preamble"
  }
  if ($responses[1].result.structuredContent.resolved.model -ne "gpt-5.6-sol-pro") {
    throw "installed consult dry-run did not resolve to gpt-5.6-sol-pro"
  }
  if ($responses[1].result.structuredContent.resolved.browser.desiredModel -ne "GPT-5.6 Sol") {
    throw "installed consult dry-run did not return GPT-5.6 Sol desired model"
  }
  if ($responses[1].result.structuredContent.resolved.browser.thinkingLabel -ne "Pro") {
    throw "installed consult dry-run did not return Pro thinking label"
  }
  if ($responses[2].result.structuredContent.status -ne "handoff-required") {
    throw "installed consult_prepare did not return handoff-required"
  }
  if ($responses[2].result.structuredContent.handoff.model -ne "gpt-5.6-sol-pro") {
    throw "installed consult_prepare did not return gpt-5.6-sol-pro handoff"
  }
  if ($responses[2].result.structuredContent.handoff.modelLabel -ne "GPT-5.6 Sol") {
    throw "installed consult_prepare did not return GPT-5.6 Sol model label"
  }
  if ($responses[2].result.structuredContent.handoff.thinkingLabel -ne "Pro") {
    throw "installed consult_prepare did not return Pro thinking label"
  }
  if ([string]::IsNullOrWhiteSpace($responses[2].result.structuredContent.handoff.handoffDigest)) {
    throw "installed consult_prepare did not return handoff digest"
  }
  if ([string]::IsNullOrWhiteSpace($responses[2].result.structuredContent.handoff.handoffNonce)) {
    throw "installed consult_prepare did not return handoff nonce"
  }
  if ($responses[2].result.structuredContent.handoff.orchestration.requiredPlugin -ne "chrome@openai-bundled") {
    throw "installed consult_prepare did not declare chrome@openai-bundled dependency"
  }
  if ($responses[2].result.structuredContent.handoff.orchestration.allowAutomaticSubmission -ne $true) {
    throw "installed consult_prepare did not allow automatic Chrome submission"
  }
  if ($responses[2].result.structuredContent.handoff.orchestration.requiresSeparateSendConfirmation -ne $false) {
    throw "installed consult_prepare still requires a separate send confirmation"
  }
  if ($responses[2].result.structuredContent.handoff.privacy.requiresUserApprovalForExternalSubmission -ne $false) {
    throw "installed consult_prepare still requires separate external submission approval"
  }
  if ($responses[2].result.structuredContent.handoff.privacy.externalSubmissionAuthorizedByInvocation -ne $true) {
    throw "installed consult_prepare did not mark invocation-based submission authorization"
  }
  if ([string]::IsNullOrWhiteSpace($responses[2].result.structuredContent.handoff.submission.mode)) {
    throw "installed consult_prepare did not return handoff submission mode"
  }
  if ([string]::IsNullOrWhiteSpace($responses[2].result.structuredContent.handoff.submission.promptText)) {
    throw "installed consult_prepare did not return handoff submission promptText"
  }
} finally {
  $env:ORACLE_HOME_DIR = $oldHome
  if (Test-Path -LiteralPath $homeDir) {
    Remove-Item -LiteralPath $homeDir -Recurse -Force
  }
}

Write-Output "Installed plugin verification passed: $PluginRoot"
