param(
    [int]$Runs = 3,
    [string]$WorkingDir = ".",
    [string]$OutputDir = "public",
    [string]$CacheDir = ".kosh-cache",
    [switch]$CleanCache,
    [string]$ResultsDir = "perf-results",
    [string]$BuildCommand = "kosh build --phase-timings"
)

$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path $scriptRoot -Parent

if ([System.IO.Path]::IsPathRooted($WorkingDir)) {
    $resolvedWorkingDir = $WorkingDir
} else {
    $resolvedWorkingDir = Join-Path (Get-Location) $WorkingDir
}

if ([System.IO.Path]::IsPathRooted($ResultsDir)) {
    $resolvedResultsDir = $ResultsDir
} else {
    $resolvedResultsDir = Join-Path $repoRoot $ResultsDir
}

New-Item -ItemType Directory -Force -Path $resolvedResultsDir | Out-Null
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$csvPath = Join-Path $resolvedResultsDir "cold-build-$timestamp.csv"

"run,total_ms,phase_file" | Out-File -FilePath $csvPath -Encoding ascii

for ($i = 1; $i -le $Runs; $i++) {
    $phaseFile = Join-Path $resolvedResultsDir "phases-$timestamp-run$i.json"
    $command = "$BuildCommand --phase-timings-file `"$phaseFile`""

    Push-Location $resolvedWorkingDir
    try {
        if (Test-Path $OutputDir) {
            Remove-Item -Recurse -Force $OutputDir
        }
        if ($CleanCache -and (Test-Path $CacheDir)) {
            Remove-Item -Recurse -Force $CacheDir
        }

        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        Invoke-Expression $command
        $exitCode = $LASTEXITCODE
        $sw.Stop()

        if ($exitCode -ne 0) {
            throw "Build command failed with exit code $exitCode on run $i"
        }
    }
    finally {
        Pop-Location
    }

    "$i,$($sw.ElapsedMilliseconds),$phaseFile" | Out-File -FilePath $csvPath -Append -Encoding ascii
}

Write-Host "Wrote results to $csvPath"
