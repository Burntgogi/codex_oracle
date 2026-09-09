param([string]$Version = "", [string]$GoExe = "")

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
$repoRoot = (Resolve-Path -LiteralPath (Split-Path -Parent $PSScriptRoot)).Path
$manifest = Get-Content -Raw -LiteralPath (Join-Path $repoRoot ".codex-plugin/plugin.json") | ConvertFrom-Json
if (-not $Version) { $Version = $manifest.version }
if ($Version -ne $manifest.version -or $Version -notmatch '^\d+\.\d+\.\d+$') { throw "Version must match the manifest's release version: $($manifest.version)" }
$finalRoot = Join-Path $repoRoot ("dist/codex_oracle-" + $Version)
if (Test-Path -LiteralPath $finalRoot) { throw "Release candidate already exists; refusing to overwrite: $finalRoot" }
$distRoot = Join-Path $repoRoot ("dist/.building-codex_oracle-" + $Version + "-" + [guid]::NewGuid())
Write-Output "Staging release at $distRoot; if building fails this directory is retained for diagnosis."
if (-not $GoExe) {
  $GoExe = Join-Path (Split-Path -Parent $repoRoot) ".tools/go/bin/go.exe"
  if (-not (Test-Path -LiteralPath $GoExe)) { $GoExe = "go" }
}

function Copy-Relative([string]$From, [string]$To, [string]$Relative) {
  $target = Join-Path $To $Relative
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
  Copy-Item -LiteralPath (Join-Path $From $Relative) -Destination $target
}

# Active product documentation only; dated reviews and historical plans stay in the workspace.
$releaseFiles = @("README.md", "README.en.md", "CHANGELOG.md", "RELEASE_NOTES.md", "RELEASE_NOTES.en.md", "LICENSE", "SECURITY.md", ".mcp.json", ".mcp.windows.json", ".mcp.macos-amd64.json", ".mcp.macos-arm64.json", "doc/workflow.md", "doc/chrome-assisted-workflow.md", "doc/install.md")
foreach ($folder in @(".codex-plugin", "skills", "assets", "docs")) {
  $releaseFiles += Get-ChildItem -LiteralPath (Join-Path $repoRoot $folder) -Recurse -File | ForEach-Object { [IO.Path]::GetRelativePath($repoRoot, $_.FullName) }
}
foreach ($script in Get-ChildItem -LiteralPath (Join-Path $repoRoot "scripts") -File) {
  $releaseFiles += "scripts/$($script.Name)"
}
$snapshot = Join-Path $distRoot "source"
foreach ($relative in $releaseFiles) { Copy-Relative $repoRoot $snapshot $relative }
Get-ChildItem -LiteralPath (Join-Path $repoRoot "mcp-go") -Recurse -File |
  Where-Object { $_.Extension -eq ".go" -or $_.Name -in @("go.mod", "go.sum") } |
  ForEach-Object { Copy-Relative $repoRoot $snapshot ([IO.Path]::GetRelativePath($repoRoot, $_.FullName)) }
foreach ($relative in $releaseFiles) { Copy-Relative $snapshot $distRoot $relative }

# Build only the frozen snapshot, never pre-existing binaries or changing workspace sources.
& (Join-Path $snapshot "scripts/build.ps1") -GoExe $GoExe -ModuleRoot (Join-Path $snapshot "mcp-go") -OutputDirectory (Join-Path $distRoot "bin") -AllPlatforms -ValidatePlugin ""
foreach ($arch in @("amd64", "arm64")) {
  $platformRoot = Join-Path $distRoot "platforms/darwin-$arch"
  foreach ($relative in $releaseFiles) { Copy-Relative $snapshot $platformRoot $relative }
  New-Item -ItemType Directory -Force -Path (Join-Path $platformRoot "bin") | Out-Null
  Copy-Item -LiteralPath (Join-Path $distRoot "bin/codex_oracle_darwin_$arch") -Destination (Join-Path $platformRoot "bin/codex_oracle")
  $config = Get-Content -Raw -LiteralPath (Join-Path $platformRoot ".mcp.json") | ConvertFrom-Json
  $config.mcpServers.codex_oracle.command = "./bin/codex_oracle"
  $config | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath (Join-Path $platformRoot ".mcp.json") -Encoding utf8NoBOM
}
$goVersion = & $GoExe version
if ($LASTEXITCODE -ne 0) { throw "Could not read Go version" }
$sourceHashes = @(Get-ChildItem -LiteralPath $snapshot -Recurse -File | Sort-Object FullName | ForEach-Object {
  @{ path = [IO.Path]::GetRelativePath($snapshot, $_.FullName).Replace('\', '/'); sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() }
})
@{
  version = $Version
  goVersion = "$goVersion"
  sourceDirectory = "source"
  sourceFiles = $sourceHashes
  buildFlags = @("-trimpath", "-buildvcs=false")
  cgoEnabled = "0"
  targets = @("windows/amd64", "darwin/amd64", "darwin/arm64")
  validation = "Windows tests/vet; macOS cross-compiled only. Run chmod +x bin/codex_oracle after copying a macOS plugin from Windows. Run verify-release.ps1 for native Windows smoke."
} | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath (Join-Path $distRoot "provenance.json") -Encoding utf8NoBOM
$checksums = @(Get-ChildItem -LiteralPath $distRoot -Recurse -File | Sort-Object FullName | ForEach-Object {
  "{0}  {1}" -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), [IO.Path]::GetRelativePath($distRoot, $_.FullName).Replace('\', '/')
})
$checksums | Set-Content -LiteralPath (Join-Path $distRoot "SHA256SUMS.txt") -Encoding ascii
$distParent = [IO.Path]::GetFullPath((Join-Path $repoRoot "dist")) + [IO.Path]::DirectorySeparatorChar
foreach ($path in @($distRoot, $finalRoot)) {
  if (-not [IO.Path]::GetFullPath($path).StartsWith($distParent, [StringComparison]::OrdinalIgnoreCase)) { throw "Release path outside dist: $path" }
}
if (Test-Path -LiteralPath $finalRoot) { throw "Release candidate appeared during build; preserving staging at $distRoot" }
# Directory.Move renames the folder rather than merging children into a partial candidate.
for ($attempt = 0; $attempt -lt 11; $attempt++) {
  try { [IO.Directory]::Move($distRoot, $finalRoot); break }
  catch [IO.IOException] {
    if ((Test-Path -LiteralPath $finalRoot) -or $attempt -eq 10) { throw }
    Start-Sleep -Milliseconds 500
  }
}
Write-Output "Built local release candidate: $finalRoot"
