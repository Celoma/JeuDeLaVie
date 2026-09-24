param(
    [int]$Runs = 3,
    [int]$Warmup = 1
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

function Get-SystemMemorySnapshot {
    try {
        $operatingSystem = Get-CimInstance Win32_OperatingSystem -ErrorAction Stop
        $totalBytes = [int64]$operatingSystem.TotalVisibleMemorySize * 1KB
        $availableBytes = [int64]$operatingSystem.FreePhysicalMemory * 1KB
        return [ordered]@{
            totalBytes = $totalBytes
            availableBytes = $availableBytes
            usedBytes = $totalBytes - $availableBytes
        }
    } catch {
        return $null
    }
}

function Invoke-WithMemorySampling {
    param(
        [string]$FilePath,
        [string[]]$ArgumentList
    )

    $outputPath = Join-Path $env:TEMP "benchmark-memory-$([guid]::NewGuid()).out"
    $errorPath = Join-Path $env:TEMP "benchmark-memory-$([guid]::NewGuid()).err"
    try {
        $process = Start-Process -FilePath $FilePath -ArgumentList ($ArgumentList -join ' ') `
            -RedirectStandardOutput $outputPath -RedirectStandardError $errorPath -PassThru
        $peakWorkingSet = [int64]0
        $peakPrivateBytes = [int64]0
        while (-not $process.HasExited) {
            $process.Refresh()
            if ($process.WorkingSet64 -gt $peakWorkingSet) { $peakWorkingSet = $process.WorkingSet64 }
            if ($process.PrivateMemorySize64 -gt $peakPrivateBytes) { $peakPrivateBytes = $process.PrivateMemorySize64 }
            Start-Sleep -Milliseconds 50
        }
        $process.Refresh()
        if ($process.WorkingSet64 -gt $peakWorkingSet) { $peakWorkingSet = $process.WorkingSet64 }
        if ($process.PrivateMemorySize64 -gt $peakPrivateBytes) { $peakPrivateBytes = $process.PrivateMemorySize64 }
        return [ordered]@{
            peakWorkingSetBytes = $peakWorkingSet
            peakPrivateBytes = $peakPrivateBytes
            exitCode = $process.ExitCode
        }
    } finally {
        Remove-Item $outputPath, $errorPath -Force -ErrorAction SilentlyContinue
    }
}

$benchmarkName = 'BenchmarkTick'
$benchmarkPattern = '^BenchmarkTick$'
$benchmarkCommand = "go test ./game -run '^$' -bench '$benchmarkPattern' -benchmem -count 1"
$hyperfineCommand = 'go test ./game -run=^$ -bench=^BenchmarkTick$ -benchmem -benchtime=1s'
$benchmarks = @()
$goos = $null
$goarch = $null
$cpu = $null
$benchmarkResults = @{}
$systemMemory = Get-SystemMemorySnapshot
$processMemory = $null

for ($warmupIndex = 1; $warmupIndex -le $Warmup; $warmupIndex++) {
    & go test ./game -run '^$' -bench "$benchmarkPattern" -benchmem -count 1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Warmup benchmark failed"
    }
}

for ($runIndex = 1; $runIndex -le $Runs; $runIndex++) {
    $output = & go test ./game -run '^$' -bench "$benchmarkPattern" -benchmem -count 1 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Benchmark run $runIndex failed"
    }

    $benchmarkLines = @($output | Select-String '^BenchmarkTick(?:-|\s)')
    if ($benchmarkLines.Count -eq 0) {
        throw "Tick benchmark lines missing from run $runIndex"
    }

    foreach ($benchmarkLine in $benchmarkLines) {
        $parts = ($benchmarkLine.Line -split '\s+' | Where-Object { $_ -ne '' })
        if ($parts.Count -lt 8) { throw "Unexpected benchmark line format: $($benchmarkLine.Line)" }
        $name = $parts[0]
        if (-not $benchmarkResults.ContainsKey($name)) { $benchmarkResults[$name] = @() }
        $benchmarkResults[$name] += [pscustomobject]@{
            NsPerOp = [double]$parts[2]
            BytesPerOp = [double]$parts[4]
            AllocsPerOp = [double]$parts[6]
        }
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

$resultReports = @()
foreach ($benchmarkEntry in $benchmarkResults.GetEnumerator()) {
    $samples = $benchmarkEntry.Value
    $nsStats = Get-Stats -Values ([double[]]($samples | ForEach-Object { $_.NsPerOp }))
    $bytesStats = Get-Stats -Values ([double[]]($samples | ForEach-Object { $_.BytesPerOp }))
    $allocStats = Get-Stats -Values ([double[]]($samples | ForEach-Object { $_.AllocsPerOp }))
    $resultReports += [ordered]@{
        name = $benchmarkEntry.Key
        meanSeconds = $nsStats.Mean / 1000000000
        medianSeconds = $nsStats.Median / 1000000000
        minSeconds = $nsStats.Min / 1000000000
        maxSeconds = $nsStats.Max / 1000000000
        p95Seconds = $nsStats.P95 / 1000000000
        p99Seconds = $nsStats.P99 / 1000000000
        stddevSeconds = $nsStats.StdDev / 1000000000
        throughputOpsPerSecond = 1000000000 / $nsStats.Mean
        nsPerOp = $nsStats.Mean
        bytesPerOp = $bytesStats.Mean
        allocsPerOp = $allocStats.Mean
        nsSamples = @($samples | ForEach-Object { $_.NsPerOp })
        bytesSamples = @($samples | ForEach-Object { $_.BytesPerOp })
        allocsSamples = @($samples | ForEach-Object { $_.AllocsPerOp })
    }
}

$report = [ordered]@{
    generatedAt = (Get-Date).ToUniversalTime().ToString('o')
    configuration = [ordered]@{
        runs = $Runs
        warmup = $Warmup
        sampleCount = $resultReports.Count * $Runs
        command = $benchmarkCommand
        hyperfine = $hyperfineCommand
        goos = $goos
        goarch = $goarch
        cpu = $cpu
        systemMemory = $systemMemory
    }
    results = $resultReports
}

$benchmarksDir = Join-Path $PSScriptRoot 'benchmarks'
New-Item -ItemType Directory -Force -Path $benchmarksDir | Out-Null

$jsonPath = Join-Path $benchmarksDir 'latest.json'
$mdPath = Join-Path $benchmarksDir 'latest.md'
$timestamp = (Get-Date).ToUniversalTime().ToString('yyyyMMdd-HHmmssfff')
$archiveMdPath = Join-Path $benchmarksDir "benchmark-$timestamp.md"
$profilePath = Join-Path $benchmarksDir "cpu-$timestamp.prof"
$memoryProfilePath = Join-Path $benchmarksDir "memory-$timestamp.prof"
$gcLogPath = Join-Path $benchmarksDir "gc-$timestamp.log"
$hyperfinePath = Join-Path $benchmarksDir "hyperfine-$timestamp.json"

try {
    $previousGodebug = $env:GODEBUG
    $env:GODEBUG = 'gctrace=1'
    $profileOutput = & go test ./game -run '^$' -bench "$benchmarkPattern" -benchmem -count 1 -cpuprofile $profilePath -memprofile $memoryProfilePath 2>&1 | Tee-Object -FilePath $gcLogPath
    if ($LASTEXITCODE -ne 0) {
        throw "CPU profile failed: $($profileOutput -join [Environment]::NewLine)"
    }
} finally {
    if ($null -eq $previousGodebug) {
        Remove-Item Env:GODEBUG -ErrorAction SilentlyContinue
    } else {
        $env:GODEBUG = $previousGodebug
    }
}

$hyperfine = Get-Command hyperfine -ErrorAction SilentlyContinue
if ($hyperfine) {
    & hyperfine --warmup $Warmup --runs $Runs --export-json $hyperfinePath $hyperfineCommand | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Hyperfine benchmark failed"
    }
    $report.configuration.hyperfineAvailable = $true
    $report.configuration.hyperfineReport = [System.IO.Path]::GetFileName($hyperfinePath)
    $hyperfineData = Get-Content $hyperfinePath -Raw | ConvertFrom-Json
    $hyperfineMemory = @($hyperfineData.results[0].memory_usage_byte)
    if ($hyperfineMemory.Count -eq 0 -or ($hyperfineMemory | Where-Object { $_ -gt 0 }).Count -eq 0) {
        $processMemory = Invoke-WithMemorySampling -FilePath 'go' -ArgumentList @('test', './game', '-run', '^$', '-bench', '^BenchmarkTick$', '-benchmem', '-benchtime=1s')
    }
} else {
    $report.configuration.hyperfineAvailable = $false
    $report.configuration.hyperfineReport = $null
    $processMemory = Invoke-WithMemorySampling -FilePath 'go' -ArgumentList @('test', './game', '-run', '^$', '-bench', '^BenchmarkTick$', '-benchmem', '-benchtime=1s')
    Write-Warning "Hyperfine n'est pas installé : rapport Go généré sans comparaison Hyperfine."
}
$report.configuration.processMemory = $processMemory

$report | ConvertTo-Json -Depth 8 | Set-Content -Path $jsonPath -Encoding utf8

$markdown = @"
# Benchmark le plus récent

Généré le : $($report.generatedAt) (UTC)

Profils : ``$([System.IO.Path]::GetFileName($profilePath))`` (CPU), ``$([System.IO.Path]::GetFileName($memoryProfilePath))`` (mémoire), ``$([System.IO.Path]::GetFileName($gcLogPath))`` (GC)

| Paramètre | Valeur |
| --- | ---: |
| Runs Go | $Runs |
| Warmup Go | $Warmup |
| Hyperfine | $(if ($report.configuration.hyperfineAvailable) { "oui ($([System.IO.Path]::GetFileName($hyperfinePath)))" } else { 'non installé' }) |
| Profil | Résultat |
| CPU | ``$([System.IO.Path]::GetFileName($profilePath))`` |
| Mémoire | ``$([System.IO.Path]::GetFileName($memoryProfilePath))`` |
| GC détaillé | ``$([System.IO.Path]::GetFileName($gcLogPath))`` |

## Résultat du tick sur la carte 600 × 600

| Benchmark | Moyenne | P95 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
$(($resultReports | ForEach-Object { "| $($_.name) | $([math]::Round($_.meanSeconds * 1000, 3)) ms | $([math]::Round($_.p95Seconds * 1000, 3)) ms | $([math]::Round($_.nsPerOp, 0)) | $([math]::Round($_.bytesPerOp, 0)) | $([math]::Round($_.allocsPerOp, 0)) |" }) -join "`n")

## Notes

- Mesure du calcul du tick sur une carte 600 × 600.
- La carte et le générateur aléatoire utilisent toujours la seed 42.
- La latence API est mesurée à part dans le frontend, sans rendu canvas.
- La trace GC détaillée est enregistrée dans le fichier ``gc-*.log`` et résumée dans le PDF.
- Les métriques I/O, réseau et base de données ne sont pas instrumentées dans ce benchmark.
"@

$markdown | Set-Content -Path $mdPath -Encoding utf8
$markdown | Set-Content -Path $archiveMdPath -Encoding utf8

Write-Host "Benchmarks écrits dans $jsonPath, $mdPath et $archiveMdPath"
Write-Host "Profils écrits dans $profilePath et $memoryProfilePath"
Write-Host "Trace GC écrite dans $gcLogPath"
if ($report.configuration.hyperfineAvailable) { Write-Host "Rapport Hyperfine écrit dans $hyperfinePath" }

$python = Get-Command python -ErrorAction SilentlyContinue
if (-not $python) {
    throw "Python est requis pour générer le rapport de benchmark."
}

& $python.Source (Join-Path $PSScriptRoot 'generate_benchmark.py') --dir $benchmarksDir
if ($LASTEXITCODE -ne 0) {
    throw "La génération du rapport de benchmark a échoué."
}
