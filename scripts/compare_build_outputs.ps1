param(
    [Parameter(Mandatory = $true)][string]$Left,
    [Parameter(Mandatory = $true)][string]$Right
)

$ErrorActionPreference = "Stop"

function Get-FileMap([string]$Root) {
    $map = @{}
    $rootItem = Get-Item $Root
    Get-ChildItem -Recurse -File $Root | ForEach-Object {
        $relative = $_.FullName.Substring($rootItem.FullName.Length).TrimStart('\', '/')
        $hash = (Get-FileHash -Algorithm SHA256 $_.FullName).Hash
        $map[$relative] = $hash
    }
    return $map
}

$leftMap = Get-FileMap $Left
$rightMap = Get-FileMap $Right

$allPaths = ($leftMap.Keys + $rightMap.Keys | Sort-Object -Unique)
$failed = $false

foreach ($path in $allPaths) {
    if (-not $leftMap.ContainsKey($path)) {
        Write-Host "Only in right: $path"
        $failed = $true
        continue
    }
    if (-not $rightMap.ContainsKey($path)) {
        Write-Host "Only in left: $path"
        $failed = $true
        continue
    }
    if ($leftMap[$path] -ne $rightMap[$path]) {
        Write-Host "Hash mismatch: $path"
        $failed = $true
    }
}

if ($failed) {
    exit 1
}

Write-Host "Outputs match."
