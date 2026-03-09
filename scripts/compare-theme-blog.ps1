param(
    [string]$SourceTheme = "C:\Users\KIIT0001\blogs\themes\blog",
    [string]$SiteTheme = "C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src\themes\blog"
)

$ErrorActionPreference = "Stop"

function Get-ThemeManifest {
    param([string]$Root)

    if (-not (Test-Path $Root)) {
        throw "Theme path not found: $Root"
    }

    Get-ChildItem -Path $Root -Recurse -File |
        Where-Object { $_.FullName -notmatch '[\\/]\.git[\\/]' } |
        ForEach-Object {
            $rel = $_.FullName.Substring($Root.Length).TrimStart('\').Replace('\', '/')
            $hash = (Get-FileHash -Algorithm SHA256 $_.FullName).Hash
            [PSCustomObject]@{
                RelativePath = $rel
                Hash = $hash
                Length = $_.Length
            }
        }
}

$left = Get-ThemeManifest -Root $SourceTheme
$right = Get-ThemeManifest -Root $SiteTheme

$leftMap = @{}
foreach ($item in $left) { $leftMap[$item.RelativePath] = $item }

$rightMap = @{}
foreach ($item in $right) { $rightMap[$item.RelativePath] = $item }

$allPaths = ($leftMap.Keys + $rightMap.Keys | Sort-Object -Unique)
$diffs = foreach ($path in $allPaths) {
    $l = $leftMap[$path]
    $r = $rightMap[$path]

    if ($null -eq $l) {
        [PSCustomObject]@{ Status = 'OnlyInSite'; RelativePath = $path; SourceLength = ''; SiteLength = $r.Length }
        continue
    }
    if ($null -eq $r) {
        [PSCustomObject]@{ Status = 'OnlyInSource'; RelativePath = $path; SourceLength = $l.Length; SiteLength = '' }
        continue
    }
    if ($l.Hash -ne $r.Hash) {
        [PSCustomObject]@{ Status = 'HashMismatch'; RelativePath = $path; SourceLength = $l.Length; SiteLength = $r.Length }
    }
}

if (-not $diffs) {
    Write-Host "themes/blog is fully synced." -ForegroundColor Green
    exit 0
}

Write-Host "themes/blog differences found:" -ForegroundColor Yellow
$diffs | Sort-Object Status, RelativePath | Format-Table -AutoSize
exit 1
