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

function Get-JsonStringProperty {
  param(
    [object]$Value,
    [string]$Name
  )

  $property = $Value.PSObject.Properties[$Name]
  if ($null -eq $property) {
    return ""
  }
  return [string]$property.Value
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
$fixtureWorkspace = Join-Path $env:TEMP ("codex_oracle-installed-workspace-" + [guid]::NewGuid().ToString())
$fixtureWorkspaceFile = "workspace-fixture.md"
$outsideRoot = Join-Path $env:TEMP ("codex_oracle-installed-outside-" + [guid]::NewGuid().ToString())
$outsideDescendant = "outside-descendant.md"
$outsideGlob = Join-Path $outsideRoot "**\*.md"
New-Item -ItemType Directory -Force -Path $fixtureWorkspace | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $outsideRoot "nested") | Out-Null
[System.IO.File]::WriteAllText(
  (Join-Path $fixtureWorkspace $fixtureWorkspaceFile),
  "workspace fixture",
  (New-Object System.Text.UTF8Encoding -ArgumentList $false)
)
[System.IO.File]::WriteAllText(
  (Join-Path $outsideRoot (Join-Path "nested" $outsideDescendant)),
  "outside",
  (New-Object System.Text.UTF8Encoding -ArgumentList $false)
)
$oldHome = $env:ORACLE_HOME_DIR
try {
  $env:ORACLE_HOME_DIR = $homeDir
  $requests = @(
    @{ jsonrpc = "2.0"; id = 1; method = "tools/list"; params = @{} },
    @{ jsonrpc = "2.0"; id = 2; method = "tools/call"; params = @{ name = "consult"; arguments = @{ preset = "chatgpt-pro-heavy"; prompt = "installed plugin dry-run check"; files = @($fixtureWorkspaceFile, $outsideRoot, $outsideGlob); workspaceRoot = $fixtureWorkspace; dryRun = $true } } },
    @{ jsonrpc = "2.0"; id = 3; method = "tools/call"; params = @{ name = "consult_prepare"; arguments = @{ preset = "chatgpt-pro-heavy"; prompt = "installed prepare check"; files = @($fixtureWorkspaceFile); workspaceRoot = $fixtureWorkspace; browserThinkingTime = "extended" } } }
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
  foreach ($name in @("consult", "consult_prepare")) {
    $tool = $responses[0].result.tools | Where-Object { $_.name -eq $name } | Select-Object -First 1
    $workspaceRootProperty = $tool.inputSchema.properties.PSObject.Properties["workspaceRoot"]
    if ($null -eq $workspaceRootProperty -or $workspaceRootProperty.Value.type -ne "string") {
      throw "installed $name input schema missing string workspaceRoot"
    }
  }
  if ($responses[1].result.structuredContent.status -ne "dry-run") {
    throw "installed consult dry-run did not return dry-run"
  }
  if (@($responses[1].result.structuredContent.resolved.files) -notcontains $fixtureWorkspaceFile) {
    throw "installed consult did not resolve fixture from workspaceRoot"
  }
  $outsideIgnored = @($responses[1].result.structuredContent.ignored)
  if ($outsideIgnored.Count -ne 2) {
    throw "installed consult did not reject both outside directory and glob"
  }
  foreach ($ignored in $outsideIgnored) {
    $pattern = Get-JsonStringProperty -Value $ignored -Name "pattern"
    $path = Get-JsonStringProperty -Value $ignored -Name "path"
    $reason = Get-JsonStringProperty -Value $ignored -Name "reason"
    if (@($outsideRoot, $outsideGlob) -notcontains $pattern) {
      throw "installed consult returned an unexpected outside rejection pattern: $pattern"
    }
    if ($reason -ne "outside root") {
      throw "installed consult did not mark outside input as outside root: $reason"
    }
    if ((@($responses[1].result.structuredContent.resolved.files) + @($pattern, $path, $reason) -join "`n") -match [regex]::Escape($outsideDescendant)) {
      throw "installed consult leaked an outside descendant path"
    }
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
  if (@($responses[2].result.structuredContent.resolved.files) -notcontains $fixtureWorkspaceFile) {
    throw "installed consult_prepare did not resolve fixture from workspaceRoot"
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
  $preparedSessionId = $responses[2].result.structuredContent.sessionId
  $preparedMeta = Get-Content -LiteralPath (Join-Path $homeDir ("sessions\" + $preparedSessionId + "\meta.json")) -Raw | ConvertFrom-Json
  if ($preparedMeta.cwd -ne $fixtureWorkspace) {
    throw "installed consult_prepare session cwd did not match workspaceRoot"
  }
} finally {
  $env:ORACLE_HOME_DIR = $oldHome
  if (Test-Path -LiteralPath $homeDir) {
    Remove-Item -LiteralPath $homeDir -Recurse -Force
  }
  if (Test-Path -LiteralPath $fixtureWorkspace) {
    Remove-Item -LiteralPath $fixtureWorkspace -Recurse -Force
  }
  if (Test-Path -LiteralPath $outsideRoot) {
    Remove-Item -LiteralPath $outsideRoot -Recurse -Force
  }
}

Write-Output "Installed plugin verification passed: $PluginRoot"
