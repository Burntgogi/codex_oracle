param(
  [string]$GoExe = "",
  [string]$ModuleRoot = "",
  [string]$OutputDirectory = "",
  [switch]$AllPlatforms,
  [string]$PythonExe = "python",
  [string]$ValidatePlugin = (Join-Path $env:USERPROFILE ".codex/skills/.system/plugin-creator/scripts/validate_plugin.py")
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir
$goModule = Join-Path $repoRoot "mcp-go"
if ($ModuleRoot) { $goModule = (Resolve-Path -LiteralPath $ModuleRoot).Path }
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $repoRoot "bin" }
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null

if ([string]::IsNullOrWhiteSpace($GoExe)) {
  $workspaceGo = Join-Path (Split-Path -Parent $repoRoot) ".tools\go\bin\go.exe"
  if (Test-Path -LiteralPath $workspaceGo) {
    $GoExe = $workspaceGo
  } else {
    $GoExe = "go"
  }
}

function Invoke-Checked {
  param(
    [string]$WorkingDirectory,
    [string]$FilePath,
    [string[]]$Arguments
  )

  Push-Location $WorkingDirectory
  try {
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
      throw "Command failed with exit code ${LASTEXITCODE}: $FilePath $($Arguments -join ' ')"
    }
  } finally {
    Pop-Location
  }
}

$savedEnv = @{}
foreach ($key in @("GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS")) {
  $savedEnv[$key] = [Environment]::GetEnvironmentVariable($key, "Process")
}
try {
  $env:GOOS = "windows"
  $env:GOARCH = "amd64"
  $env:CGO_ENABLED = "0"
  $env:GOFLAGS = ""
  Invoke-Checked -WorkingDirectory $goModule -FilePath $GoExe -Arguments @("test", "./...")
  Invoke-Checked -WorkingDirectory $goModule -FilePath $GoExe -Arguments @("vet", "./...")
  $targets = @(@{ OS = "windows"; Arch = "amd64"; File = "codex_oracle.exe" })
  if ($AllPlatforms) {
    $targets += @{ OS = "darwin"; Arch = "amd64"; File = "codex_oracle_darwin_amd64" }
    $targets += @{ OS = "darwin"; Arch = "arm64"; File = "codex_oracle_darwin_arm64" }
  }
  foreach ($target in $targets) {
    $env:GOOS = $target.OS
    $env:GOARCH = $target.Arch
    $output = Join-Path $OutputDirectory $target.File
    Invoke-Checked -WorkingDirectory $goModule -FilePath $GoExe -Arguments @("build", "-trimpath", "-buildvcs=false", "-o", $output, "./cmd/codex_oracle")
    Write-Output "Built $output ($($target.OS)/$($target.Arch))"
  }
} finally {
  foreach ($key in $savedEnv.Keys) { [Environment]::SetEnvironmentVariable($key, $savedEnv[$key], "Process") }
}

if ($ValidatePlugin -and (Test-Path -LiteralPath $ValidatePlugin)) {
  Invoke-Checked -WorkingDirectory $repoRoot -FilePath $PythonExe -Arguments @($ValidatePlugin, $repoRoot)
} else {
  Write-Warning "Plugin validator not found: $ValidatePlugin"
}
