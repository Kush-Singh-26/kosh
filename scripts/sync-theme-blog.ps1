param(
    [string]$SourceTheme = "C:\Users\KIIT0001\blogs\themes\blog",
    [string]$SiteTheme = "C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src\themes\blog",
    [switch]$DeleteExtra
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $SourceTheme)) {
    throw "Source theme path not found: $SourceTheme"
}

New-Item -ItemType Directory -Path $SiteTheme -Force | Out-Null

Write-Host "Syncing theme from:`n  $SourceTheme`nto:`n  $SiteTheme" -ForegroundColor Cyan

Get-ChildItem -Path $SourceTheme -Recurse -File | ForEach-Object {
    $rel = $_.FullName.Substring($SourceTheme.Length).TrimStart('\')
    $dest = Join-Path $SiteTheme $rel
    $destDir = Split-Path -Parent $dest
    New-Item -ItemType Directory -Path $destDir -Force | Out-Null
    Copy-Item -Path $_.FullName -Destination $dest -Force
}

if ($DeleteExtra) {
    $sourcePaths = Get-ChildItem -Path $SourceTheme -Recurse -File | ForEach-Object { $_.FullName.Substring($SourceTheme.Length).TrimStart('\').Replace('\', '/') }
    Get-ChildItem -Path $SiteTheme -Recurse -File | ForEach-Object {
        $rel = $_.FullName.Substring($SiteTheme.Length).TrimStart('\').Replace('\', '/')
        if ($sourcePaths -notcontains $rel) {
            Remove-Item -Force $_.FullName
        }
    }
}

Write-Host "Theme sync complete." -ForegroundColor Green
