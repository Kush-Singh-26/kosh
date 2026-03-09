param(
    [int[]]$VipsConcurrencyValues = @(4, 6, 8),
    [int[]]$ImageWorkerValues = @(8, 12, 16),
    [int]$RunsPerCombo = 3,
    [string]$ResultsFile = ".\docs\build-benchmark-results-warm.md",
    [string]$ConfigFile = "C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src\kosh.yaml",
    [string]$LogsDir = ".\perf-results\benchmark-logs-warm"
)

$ErrorActionPreference = "Stop"

function Set-YamlScalarLine {
    param(
        [string]$Content,
        [string]$Key,
        [string]$Value
    )

    $pattern = "(?m)^\s*" + [regex]::Escape($Key) + "\s*:\s*.*$"
    if ([regex]::IsMatch($Content, $pattern)) {
        return [regex]::Replace($Content, $pattern, "${Key}: $Value", 1)
    }

    $trimmed = $Content.TrimEnd("`r", "`n")
    return $trimmed + "`r`n" + "${Key}: $Value" + "`r`n"
}

function Parse-Metric {
    param(
        [string]$Text,
        [string]$Pattern
    )

    $match = [regex]::Match($Text, $Pattern)
    if ($match.Success) {
        return $match.Groups[1].Value.Trim()
    }
    return "N/A"
}

function Upsert-ResultRow {
    param(
        [string]$FilePath,
        [int]$VipsConcurrency,
        [int]$ImageWorkers,
        [int]$Run,
        [string]$Total,
        [string]$RootStatic,
        [string]$AssetBuilding,
        [string]$ParsePosts,
        [string]$CacheHits,
        [string]$Notes
    )

    $row = "| $VipsConcurrency | $ImageWorkers | $Run | $Total | $RootStatic | $AssetBuilding | $ParsePosts | $CacheHits | $Notes |"
    $content = Get-Content -Path $FilePath -Raw
    $pattern = "(?m)^\|\s*$VipsConcurrency\s*\|\s*$ImageWorkers\s*\|\s*$Run\s*\|.*$"
    if ([regex]::IsMatch($content, $pattern)) {
        $updated = [regex]::Replace($content, $pattern, [System.Text.RegularExpressions.MatchEvaluator]{ param($m) $row }, 1)
        Set-Content -Path $FilePath -Value $updated -NoNewline
    } else {
        Add-Content -Path $FilePath -Value $row
    }
}

if (-not (Test-Path $ConfigFile)) {
    throw "Config file not found: $ConfigFile"
}

if (-not (Test-Path $ResultsFile)) {
    throw "Results file not found: $ResultsFile"
}

New-Item -ItemType Directory -Path $LogsDir -Force | Out-Null

$originalConfig = Get-Content -Path $ConfigFile -Raw
$siteWorkingDir = Split-Path -Parent $ConfigFile

try {
    foreach ($vips in $VipsConcurrencyValues) {
        foreach ($workers in $ImageWorkerValues) {
            for ($run = 1; $run -le $RunsPerCombo; $run++) {
                Write-Host "Running warm benchmark: vipsConcurrency=$vips imageWorkers=$workers run=$run" -ForegroundColor Cyan

                $updatedConfig = $originalConfig
                $updatedConfig = Set-YamlScalarLine -Content $updatedConfig -Key "vipsConcurrency" -Value "$vips"
                $updatedConfig = Set-YamlScalarLine -Content $updatedConfig -Key "imageWorkers" -Value "$workers"
                Set-Content -Path $ConfigFile -Value $updatedConfig -NoNewline

                $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
                $logFile = Join-Path $LogsDir ("warm-v{0}-w{1}-run{2}-{3}.log" -f $vips, $workers, $run, $timestamp)
                $stdoutFile = Join-Path $LogsDir ("warm-v{0}-w{1}-run{2}-{3}.stdout.log" -f $vips, $workers, $run, $timestamp)
                $stderrFile = Join-Path $LogsDir ("warm-v{0}-w{1}-run{2}-{3}.stderr.log" -f $vips, $workers, $run, $timestamp)

                $proc = Start-Process -FilePath "kosh" -ArgumentList "clean" -WorkingDirectory $siteWorkingDir -NoNewWindow -PassThru -Wait -RedirectStandardOutput $stdoutFile -RedirectStandardError $stderrFile
                $stdout = if (Test-Path $stdoutFile) { Get-Content -Path $stdoutFile -Raw } else { "" }
                $stderr = if (Test-Path $stderrFile) { Get-Content -Path $stderrFile -Raw } else { "" }
                $output = ($stdout + [Environment]::NewLine + $stderr).Trim()
                Set-Content -Path $logFile -Value $output

                $total = Parse-Metric -Text $output -Pattern "Built \d+ posts in (.*?) \(cache:"
                $rootStatic = Parse-Metric -Text $output -Pattern 'Asset copy root/static" duration=(.*?)\r?\n'
                $assetBuilding = Parse-Metric -Text $output -Pattern 'Asset building" duration=(.*?)\r?\n'
                $parsePosts = Parse-Metric -Text $output -Pattern 'Parse \d+ posts" duration=(.*?)\r?\n'
                $cacheHits = Parse-Metric -Text $output -Pattern "cache: (.*?)\)"
                $notes = if ($proc.ExitCode -ne 0 -or $output -match "Build failed|rebuild failed") { "FAILED: see $(Split-Path $logFile -Leaf)" } else { "$(Split-Path $logFile -Leaf)" }

                Upsert-ResultRow -FilePath $ResultsFile -VipsConcurrency $vips -ImageWorkers $workers -Run $run -Total $total -RootStatic $rootStatic -AssetBuilding $assetBuilding -ParsePosts $parsePosts -CacheHits $cacheHits -Notes $notes
            }
        }
    }
}
finally {
    Set-Content -Path $ConfigFile -Value $originalConfig -NoNewline
    Write-Host "Restored original kosh.yaml" -ForegroundColor Green
}

Write-Host "Warm benchmarking complete. Results written to $ResultsFile" -ForegroundColor Green
