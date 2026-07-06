param(
  [string]$BinaryPath = "",
  [string]$FixtureFile = "doc/implementation-plan.md"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir
if ([string]::IsNullOrWhiteSpace($BinaryPath)) {
  $BinaryPath = Join-Path $repoRoot "bin\codex_oracle.exe"
}
if (-not (Test-Path -LiteralPath $BinaryPath)) {
  throw "MCP binary not found: $BinaryPath"
}
if (-not (Test-Path -LiteralPath (Join-Path $repoRoot $FixtureFile)) -and
    $FixtureFile -eq "doc/implementation-plan.md" -and
    (Test-Path -LiteralPath (Join-Path $repoRoot "README.md"))) {
  $FixtureFile = "README.md"
}
if (-not (Test-Path -LiteralPath (Join-Path $repoRoot $FixtureFile))) {
  throw "Fixture file not found under plugin root: $FixtureFile"
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

function Set-Utf8NoBomText {
  param(
    [string]$Path,
    [AllowEmptyString()][string]$Value
  )

  $encoding = New-Object System.Text.UTF8Encoding -ArgumentList $false
  [System.IO.File]::WriteAllText($Path, $Value, $encoding)
}

$homeDir = Join-Path $env:TEMP ("codex_oracle-smoke-" + [guid]::NewGuid().ToString())
$sessionId = "smoke-session"
$sessionDir = Join-Path $homeDir ("sessions\" + $sessionId)
$finalizeSessionId = "smoke-chrome-assisted"
$finalizeSessionDir = Join-Path $homeDir ("sessions\" + $finalizeSessionId)
$profileDir = Join-Path $homeDir "profile"
$chromePath = Join-Path $homeDir "chrome.exe"
New-Item -ItemType Directory -Force -Path $sessionDir | Out-Null
New-Item -ItemType Directory -Force -Path $finalizeSessionDir | Out-Null
New-Item -ItemType Directory -Force -Path $profileDir | Out-Null
Set-Utf8NoBomText -Path $chromePath -Value "fake"

$metadata = @{
  id = $sessionId
  createdAt = "2026-07-05T00:00:00Z"
  lastHeartbeatAt = "2026-07-05T00:00:10Z"
  status = "running"
  model = "gpt-5.5-pro"
  mode = "browser"
  runtime = @{
    endpoint = "ws://127.0.0.1:9222/devtools/browser/smoke"
    targetId = "target-1"
    conversationUrl = "https://chatgpt.com/c/smoke"
    profileDir = $profileDir
  }
} | ConvertTo-Json -Depth 6
Set-Utf8NoBomText -Path (Join-Path $sessionDir "meta.json") -Value $metadata
Set-Utf8NoBomText -Path (Join-Path $sessionDir "request.json") -Value "{}"
Set-Utf8NoBomText -Path (Join-Path $sessionDir "output.log") -Value "partial"

$finalizeMetadata = @{
  id = $finalizeSessionId
  createdAt = "2026-07-05T00:01:00Z"
  status = "running"
  model = "gpt-5.5-pro"
  mode = "chrome-assisted"
} | ConvertTo-Json -Depth 6
$finalizeRequest = @{
  executionMode = "chrome-assisted"
  handoffDigest = "digest-smoke"
  handoffNonce = "nonce-smoke"
} | ConvertTo-Json -Depth 6
Set-Utf8NoBomText -Path (Join-Path $finalizeSessionDir "meta.json") -Value $finalizeMetadata
Set-Utf8NoBomText -Path (Join-Path $finalizeSessionDir "request.json") -Value $finalizeRequest
Set-Utf8NoBomText -Path (Join-Path $finalizeSessionDir "output.log") -Value ""

$oldHome = $env:ORACLE_HOME_DIR
$oldChrome = $env:ORACLE_BROWSER_CHROME_PATH
$oldProfile = $env:ORACLE_BROWSER_PROFILE_DIR
$oldContentMode = $env:ORACLE_SESSION_CONTENT_MODE
try {
  $env:ORACLE_HOME_DIR = $homeDir
  $env:ORACLE_BROWSER_CHROME_PATH = $chromePath
  $env:ORACLE_BROWSER_PROFILE_DIR = $profileDir
  $env:ORACLE_SESSION_CONTENT_MODE = ""

  $consultArgs = @{
    preset = "chatgpt-pro-heavy"
    prompt = "smoke review"
    files = @($FixtureFile)
    browserAttachments = "always"
    browserBundleFormat = "zip"
    dryRun = $true
  }
  $prepareArgs = @{
    preset = "chatgpt-pro-heavy"
    prompt = "smoke chrome handoff"
    files = @($FixtureFile)
    browserThinkingTime = "extended"
    browserFollowUps = @("summarize risk")
  }
  $finalizeArgs = @{
    sessionId = $finalizeSessionId
    status = "completed"
    handoffDigest = "digest-smoke"
    handoffNonce = "nonce-smoke"
    answer = "smoke chrome assisted answer"
    transcript = "# Transcript`n`nsmoke chrome assisted answer`n"
    runtime = @{
      runner = "chrome-plugin"
      conversationUrl = "https://chatgpt.com/c/smoke"
      cookie = "must-not-persist"
    }
  }
  $requests = @(
    @{ jsonrpc = "2.0"; id = 1; method = "tools/list"; params = @{} },
    @{ jsonrpc = "2.0"; id = 2; method = "tools/call"; params = @{ name = "consult"; arguments = $consultArgs } },
    @{ jsonrpc = "2.0"; id = 3; method = "tools/call"; params = @{ name = "reattach"; arguments = @{ id = $sessionId; dryRun = $true } } },
    @{ jsonrpc = "2.0"; id = 4; method = "tools/call"; params = @{ name = "smoke_check"; arguments = @{ dryRun = $true } } },
    @{ jsonrpc = "2.0"; id = 5; method = "tools/call"; params = @{ name = "consult_prepare"; arguments = $prepareArgs } },
    @{ jsonrpc = "2.0"; id = 6; method = "tools/call"; params = @{ name = "consult_finalize"; arguments = $finalizeArgs } },
    @{ jsonrpc = "2.0"; id = 7; method = "tools/call"; params = @{ name = "session_delete"; arguments = @{ id = $sessionId } } }
  )
  $inputJson = ($requests | ForEach-Object { $_ | ConvertTo-Json -Depth 12 -Compress }) -join "`n"
  $responses = @(Invoke-McpJsonLines -ExecutablePath $BinaryPath -InputJson $inputJson -WorkingDirectory $repoRoot)

  $toolNames = @($responses[0].result.tools | ForEach-Object { $_.name })
  foreach ($name in @("consult", "consult_prepare", "consult_finalize", "sessions", "session_delete", "login_setup", "smoke_check", "reattach")) {
    if ($toolNames -notcontains $name) {
      throw "missing tool $name"
    }
  }
  if ($responses[1].result.structuredContent.status -ne "dry-run") {
    throw "consult dryRun did not return dry-run"
  }
  if ($responses[1].result.structuredContent.attachmentPlan.mode -ne "zip") {
    throw "forced attachment dryRun did not choose zip"
  }
  if ($responses[1].result.structuredContent.bundle.preview -notmatch "untrusted reference material") {
    throw "consult dryRun bundle preview missing untrusted-context preamble"
  }
  if ($responses[2].result.structuredContent.reattachable -ne $true) {
    throw "reattach dryRun did not report reattachable"
  }
  if ($responses[3].result.structuredContent.status -ne "dry-run") {
    throw "smoke_check dryRun did not return dry-run"
  }
  if ($responses[4].result.structuredContent.status -ne "handoff-required") {
    throw "consult_prepare did not return handoff-required"
  }
  if ($responses[4].result.structuredContent.handoff.model -ne "gpt-5.5-pro") {
    throw "consult_prepare did not return gpt-5.5-pro handoff"
  }
  if ($responses[4].result.structuredContent.handoff.modelLabel -ne "GPT-5.5") {
    throw "consult_prepare did not return GPT-5.5 model label"
  }
  if ($responses[4].result.structuredContent.handoff.thinkingLabel -ne "Pro 확장") {
    throw "consult_prepare did not return Pro 확장 thinking label"
  }
  if ([string]::IsNullOrWhiteSpace($responses[4].result.structuredContent.handoff.handoffDigest)) {
    throw "consult_prepare did not return handoff digest"
  }
  if ([string]::IsNullOrWhiteSpace($responses[4].result.structuredContent.handoff.handoffNonce)) {
    throw "consult_prepare did not return handoff nonce"
  }
  if ($responses[4].result.structuredContent.handoff.privacy.targetHost -ne "chatgpt.com") {
    throw "consult_prepare did not return chatgpt.com privacy target"
  }
  if ($responses[4].result.structuredContent.handoff.orchestration.requiredPlugin -ne "chrome@openai-bundled") {
    throw "consult_prepare did not declare chrome@openai-bundled dependency"
  }
  if ($responses[4].result.structuredContent.handoff.orchestration.ownsBrowserAutomation -ne $false) {
    throw "consult_prepare incorrectly claims browser automation ownership"
  }
  if ($responses[4].result.structuredContent.handoff.orchestration.allowAutomaticSubmission -ne $true) {
    throw "consult_prepare did not allow automatic Chrome submission"
  }
  if ($responses[4].result.structuredContent.handoff.orchestration.requiresSeparateSendConfirmation -ne $false) {
    throw "consult_prepare still requires a separate send confirmation"
  }
  if ($responses[4].result.structuredContent.handoff.privacy.requiresUserApprovalForExternalSubmission -ne $false) {
    throw "consult_prepare still requires separate external submission approval"
  }
  if ($responses[4].result.structuredContent.handoff.privacy.externalSubmissionAuthorizedByInvocation -ne $true) {
    throw "consult_prepare did not mark invocation-based submission authorization"
  }
  if ([string]::IsNullOrWhiteSpace($responses[4].result.structuredContent.handoff.submission.mode)) {
    throw "consult_prepare did not return handoff submission mode"
  }
  if ([string]::IsNullOrWhiteSpace($responses[4].result.structuredContent.handoff.submission.promptText)) {
    throw "consult_prepare did not return handoff submission promptText"
  }
  $preparedSessionId = $responses[4].result.structuredContent.sessionId
  $handoffPath = Join-Path $homeDir ("sessions\" + $preparedSessionId + "\artifacts\handoff.json")
  if (Test-Path -LiteralPath $handoffPath) {
    throw "consult_prepare wrote handoff artifact in default metadata mode"
  }
  if ($responses[5].result.structuredContent.status -ne "completed") {
    throw "consult_finalize did not complete the session"
  }
  $finalizedMeta = Get-Content -LiteralPath (Join-Path $finalizeSessionDir "meta.json") -Raw | ConvertFrom-Json
  if ($finalizedMeta.status -ne "completed") {
    throw "consult_finalize did not persist completed status"
  }
  if ($finalizedMeta.runtime.PSObject.Properties.Name -contains "cookie") {
    throw "consult_finalize persisted a cookie runtime field"
  }
  if ($responses[6].result.structuredContent.deleted -ne $true) {
    throw "session_delete did not report deletion"
  }
  if (Test-Path -LiteralPath $sessionDir) {
    throw "session_delete did not remove session directory"
  }
  Write-Output "MCP smoke passed"
} finally {
  $env:ORACLE_HOME_DIR = $oldHome
  $env:ORACLE_BROWSER_CHROME_PATH = $oldChrome
  $env:ORACLE_BROWSER_PROFILE_DIR = $oldProfile
  $env:ORACLE_SESSION_CONTENT_MODE = $oldContentMode
  if (Test-Path -LiteralPath $homeDir) {
    Remove-Item -LiteralPath $homeDir -Recurse -Force
  }
}
