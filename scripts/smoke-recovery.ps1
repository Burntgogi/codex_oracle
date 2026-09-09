param([string]$BinaryPath = "")

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
if (-not $BinaryPath) { $BinaryPath = Join-Path $repoRoot "bin/codex_oracle.exe" }
$BinaryPath = (Resolve-Path -LiteralPath $BinaryPath).Path
$testHome = Join-Path ([IO.Path]::GetTempPath()) ("oracle-recovery-" + [guid]::NewGuid())
$previousHome = $env:ORACLE_HOME_DIR
$previousMode = $env:ORACLE_SESSION_CONTENT_MODE

function Invoke-RecoveryTool([string]$Name, [hashtable]$Arguments) {
  $request = @{ jsonrpc = "2.0"; id = 1; method = "tools/call"; params = @{ name = $Name; arguments = $Arguments } }
  $output = ($request | ConvertTo-Json -Depth 20 -Compress) | & $BinaryPath
  if ($LASTEXITCODE -ne 0) { throw "MCP process failed: $LASTEXITCODE" }
  $response = $output | ConvertFrom-Json
  if ($response.PSObject.Properties.Name -contains "error") { throw ($response | ConvertTo-Json -Depth 20) }
  if (($response.result.PSObject.Properties.Name -contains "isError") -and $response.result.isError) { throw ($response | ConvertTo-Json -Depth 20) }
  return $response.result.structuredContent
}

try {
  $env:ORACLE_HOME_DIR = $testHome
  $env:ORACLE_SESSION_CONTENT_MODE = "metadata"
  $prepared = Invoke-RecoveryTool "consult_prepare" @{ prompt = "Synthetic recovery smoke prompt"; preset = "chatgpt-pro-heavy"; workspaceRoot = $repoRoot }
  if ($prepared.status -ne "handoff-required") { throw "Prepare did not return a handoff" }
  $plan = Invoke-RecoveryTool "reattach" @{ id = $prepared.sessionId; dryRun = $false }
  if ($plan.status -ne "chrome-recovery-required" -or $plan.reattachable -or $plan.requiresForce) { throw "Incorrect new-session recovery plan" }
  if ($plan.finalize.handoffDigest -ne $prepared.handoff.handoffDigest -or $plan.finalize.handoffNonce -ne $prepared.handoff.handoffNonce) { throw "Recovery lost handoff identity" }
  $checkpoint = Invoke-RecoveryTool "consult_checkpoint" @{
    sessionId = $prepared.checkpoint.arguments.sessionId
    handoffDigest = $prepared.checkpoint.arguments.handoffDigest
    handoffNonce = $prepared.checkpoint.arguments.handoffNonce
    conversationUrl = "https://chatgpt.com/c/synthetic-recovery-smoke"
  }
  if (-not $checkpoint.checkpointed -or $checkpoint.status -ne "running") { throw "Checkpoint failed or completed session prematurely" }
  # Each invocation launches a fresh process: this verifies restart recovery.
  $plan = Invoke-RecoveryTool "reattach" @{ id = $prepared.sessionId }
  if (-not $plan.reattachable -or $plan.runtime.conversationUrl -ne $checkpoint.runtime.conversationUrl) { throw "Checkpoint did not survive restart" }
  $completed = Invoke-RecoveryTool "consult_finalize" @{
    sessionId = $plan.finalize.sessionId
    handoffDigest = $plan.finalize.handoffDigest
    handoffNonce = $plan.finalize.handoffNonce
    status = "completed"
    answer = "Synthetic captured answer"
    transcript = "Synthetic transcript"
    runtime = @{ conversationUrl = "https://chatgpt.com/c/synthetic-recovery-smoke" }
  }
  if ($completed.status -ne "completed" -or $completed.output -ne "Synthetic captured answer") { throw "Finalize did not return answer" }
  $detail = Invoke-RecoveryTool "sessions" @{ id = $prepared.sessionId; detail = $true }
  if ($detail.session.metadata.status -ne "completed" -or $detail.session.log -ne "") { throw "Completion or metadata-only privacy failed" }
  $plan = Invoke-RecoveryTool "reattach" @{ id = $prepared.sessionId }
  if (-not $plan.reattachable -or -not $plan.requiresForce) { throw "Saved conversation/terminal recovery failed" }
  $sessionPath = Join-Path $testHome ("sessions/" + $prepared.sessionId)
  $stored = Get-ChildItem -LiteralPath $sessionPath -File -Recurse | ForEach-Object { Get-Content -LiteralPath $_.FullName -Raw }
  if (($stored -join "`n") -match "Synthetic recovery smoke prompt|Synthetic captured answer|Synthetic transcript") { throw "Private content persisted in metadata mode" }
  $deleted = Invoke-RecoveryTool "session_delete" @{ id = $prepared.sessionId }
  if (-not $deleted.deleted -or (Test-Path -LiteralPath $sessionPath)) { throw "Session cleanup failed" }
  Write-Output "Recovery MCP smoke passed: prepare -> checkpoint -> restart/recover -> finalize -> inspect privacy -> recover terminal -> delete. Synthetic content only; no browser submission."
} finally {
  $env:ORACLE_HOME_DIR = $previousHome
  $env:ORACLE_SESSION_CONTENT_MODE = $previousMode
  if (Test-Path -LiteralPath $testHome) {
    $resolvedHome = (Resolve-Path -LiteralPath $testHome).Path
    $tempParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
    if (-not $resolvedHome.StartsWith($tempParent, [StringComparison]::OrdinalIgnoreCase) -or (Split-Path -Leaf $resolvedHome) -notlike "oracle-recovery-*") { throw "Refusing cleanup outside recovery test directory: $resolvedHome" }
    Remove-Item -LiteralPath $resolvedHome -Recurse -Force
  }
}
