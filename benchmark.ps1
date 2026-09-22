param(
    [int]$Runs = 10,
    [int]$Warmup = 2
)

function Get-Percentile {
    param(
        [double[]]$Values,
        [double]$Ratio
    )

    $sorted = $Values | Sort-Object
    if ($sorted.Count -eq 0) {
        return 0
    }
    if ($sorted.Count -eq 1) {
        return [double]$sorted[0]
    }

    $index = ($sorted.Count - 1) * $Ratio
    $lowerIndex = [math]::Floor($index)
    $upperIndex = [math]::Ceiling($index)
    if ($lowerIndex -eq $upperIndex) {
        return [double]$sorted[$lowerIndex]
    }

    $weight = $index - $lowerIndex
    return [double]$sorted[$lowerIndex] + (([double]$sorted[$upperIndex] - [double]$sorted[$lowerIndex]) * $weight)
}

function Get-Stats {
    param([double[]]$Values)

    $sorted = $Values | Sort-Object
    $count = $sorted.Count
    $sum = 0.0
    foreach ($value in $sorted) { $sum += [double]$value }

    $mean = $sum / $count
    $variance = 0.0
    foreach ($value in $sorted) { $variance += ([double]$value - $mean) * ([double]$value - $mean) }
    $variance /= $count

    $median = Get-Percentile -Values $sorted -Ratio 0.5

    [ordered]@{
        Mean = $mean
        Median = $median
        Min = [double]$sorted[0]
        Max = [double]$sorted[$count - 1]
        P95 = Get-Percentile -Values $sorted -Ratio 0.95
        P99 = Get-Percentile -Values $sorted -Ratio 0.99
        StdDev = [math]::Sqrt($variance)
    }
}

$benchmarkName = 'BenchmarkTick'
$benchmarkCommand = "go test ./game -run '^$' -bench '^$benchmarkName`$' -benchmem -count 1"
$benchmarks = @()
$goos = $null
$goarch = $null
$cpu = $null

for ($warmupIndex = 1; $warmupIndex -le $Warmup; $warmupIndex++) {
    & go test ./game -run '^$' -bench "^$benchmarkName`$" -benchmem -count 1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Warmup benchmark failed"
    }
}

for ($runIndex = 1; $runIndex -le $Runs; $runIndex++) {
    $output = & go test ./game -run '^$' -bench "^$benchmarkName`$" -benchmem -count 1 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Benchmark run $runIndex failed"
    }

    $benchmarkLine = $output | Select-String "^$benchmarkName"
    if (-not $benchmarkLine) {
        throw "BenchmarkBoardStep line missing from run $runIndex"
    }

    $parts = ($benchmarkLine.Line -split '\s+' | Where-Object { $_ -ne '' })
    if ($parts.Count -lt 8) {
        throw "Unexpected benchmark line format: $($benchmarkLine.Line)"
    }

    $benchmarks += [pscustomobject]@{
        NsPerOp = [double]$parts[2]
        BytesPerOp = [double]$parts[4]
        AllocsPerOp = [double]$parts[6]
    }

    if (-not $goos) {
        $goosLine = $output | Select-String '^goos:'
        $goarchLine = $output | Select-String '^goarch:'
        $cpuLine = $output | Select-String '^cpu:'
        if ($goosLine) { $goos = ($goosLine.Line -split '\s+')[1] }
        if ($goarchLine) { $goarch = ($goarchLine.Line -split '\s+')[1] }
        if ($cpuLine) { $cpu = ($cpuLine.Line -replace '^cpu:\s*', '') }
    }
}

$nsValues = [double[]]($benchmarks | ForEach-Object { $_.NsPerOp })
$bytesValues = [double[]]($benchmarks | ForEach-Object { $_.BytesPerOp })
$allocValues = [double[]]($benchmarks | ForEach-Object { $_.AllocsPerOp })

$nsStats = Get-Stats -Values $nsValues
$bytesStats = Get-Stats -Values $bytesValues
$allocStats = Get-Stats -Values $allocValues
$throughput = 1000000000 / $nsStats.Mean

$report = [ordered]@{
    generatedAt = (Get-Date).ToUniversalTime().ToString('o')
    configuration = [ordered]@{
        runs = $Runs
        warmup = $Warmup
        sampleCount = $benchmarks.Count
        command = $benchmarkCommand
        goos = $goos
        goarch = $goarch
        cpu = $cpu
    }
    results = @(
        [ordered]@{
            name = 'BenchmarkBoardStep'
            meanSeconds = $nsStats.Mean / 1000000000
            medianSeconds = $nsStats.Median / 1000000000
            minSeconds = $nsStats.Min / 1000000000
            maxSeconds = $nsStats.Max / 1000000000
            p95Seconds = $nsStats.P95 / 1000000000
            p99Seconds = $nsStats.P99 / 1000000000
            stddevSeconds = $nsStats.StdDev / 1000000000
            throughputOpsPerSecond = $throughput
            nsPerOp = $nsStats.Mean
            bytesPerOp = $bytesStats.Mean
            allocsPerOp = $allocStats.Mean
            notes = @(
                'Mesure du moteur Go sur plusieurs exécutions indépendantes.',
                'La latence API est mesurée à part dans le frontend, sans rendu canvas.',
                'Les métriques CPU bas niveau, GC détaillé, I/O, réseau et base de données ne sont pas instrumentées dans ce benchmark.'
            )
        }
    )
}

$benchmarksDir = Join-Path $PSScriptRoot 'benchmarks'
New-Item -ItemType Directory -Force -Path $benchmarksDir | Out-Null

$jsonPath = Join-Path $benchmarksDir 'latest.json'
$mdPath = Join-Path $benchmarksDir 'latest.md'
$timestamp = (Get-Date).ToUniversalTime().ToString('yyyyMMdd-HHmmssfff')
$archiveMdPath = Join-Path $benchmarksDir "benchmark-$timestamp.md"
$profilePath = Join-Path $benchmarksDir "cpu-$timestamp.prof"

$profileOutput = & go test ./game -run '^$' -bench "^$benchmarkName`$" -benchmem -count 1 -cpuprofile $profilePath 2>&1
if ($LASTEXITCODE -ne 0) {
    throw "CPU profile failed: $($profileOutput -join [Environment]::NewLine)"
}

$report | ConvertTo-Json -Depth 8 | Set-Content -Path $jsonPath -Encoding utf8

$markdown = @"
# Benchmark le plus récent

Généré le : $($report.generatedAt) (UTC)

Profil CPU : ``$([System.IO.Path]::GetFileName($profilePath))``

| Mesure | Valeur |
| --- | ---: |
| Runs | $Runs |
| Warmup | $Warmup |
| Latence moyenne | $([math]::Round($nsStats.Mean / 1000000, 3)) ms |
| Médiane | $([math]::Round($nsStats.Median / 1000000, 3)) ms |
| P95 | $([math]::Round($nsStats.P95 / 1000000, 3)) ms |
| P99 | $([math]::Round($nsStats.P99 / 1000000, 3)) ms |
| Maximum | $([math]::Round($nsStats.Max / 1000000, 3)) ms |
| ns/op | $([math]::Round($nsStats.Mean, 0)) |
| B/op | $([math]::Round($bytesStats.Mean, 0)) |
| allocs/op | $([math]::Round($allocStats.Mean, 0)) |
| Débit | $([math]::Round($throughput, 0)) op/s |

## Notes

- Mesure du moteur Go sur plusieurs exécutions indépendantes.
- La latence API est mesurée à part dans le frontend, sans rendu canvas.
- Les métriques CPU bas niveau, GC détaillé, I/O, réseau et base de données ne sont pas instrumentées dans ce benchmark.
"@

$markdown | Set-Content -Path $mdPath -Encoding utf8
$markdown | Set-Content -Path $archiveMdPath -Encoding utf8

Write-Host "Benchmarks écrits dans $jsonPath, $mdPath, $archiveMdPath et $profilePath"
