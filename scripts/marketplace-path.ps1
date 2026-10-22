function Get-LocalPluginSourcePath {
  param([string]$MarketplacePath, [string]$PluginTarget)

  $marketplaceDirectory = Split-Path -Parent ([IO.Path]::GetFullPath($MarketplacePath))
  $marketplaceRoot = $marketplaceDirectory
  if ((Split-Path -Leaf $marketplaceDirectory) -eq "plugins" -and
      (Split-Path -Leaf (Split-Path -Parent $marketplaceDirectory)) -eq ".agents") {
    $marketplaceRoot = Split-Path -Parent (Split-Path -Parent $marketplaceDirectory)
  }
  $rootPrefix = $marketplaceRoot.TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
  $target = [IO.Path]::GetFullPath($PluginTarget)
  if (-not $target.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Plugin destination must be inside the marketplace root: $marketplaceRoot"
  }
  return "./" + $target.Substring($rootPrefix.Length).Replace('\', '/')
}
