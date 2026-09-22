param(
    [int]$Runs = 10,
    [int]$Warmup = 2
)

$command = "go test ./game -run '^$' -bench '^BenchmarkBoardStep$' -benchtime=1s"
hyperfine --warmup $Warmup --runs $Runs $command
