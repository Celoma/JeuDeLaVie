package game

import (
	"math/rand"
	"os"
	"strconv"
	"testing"
)

func benchmarkEnvironmentInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func benchmarkEnvironmentFloat(name string, fallback float64) float64 {
	value, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err != nil || value < 0 || value > 1 {
		return fallback
	}
	return value
}

func BenchmarkTick(b *testing.B) {
	const benchmarkSeed = 42
	benchmarkWidth := benchmarkEnvironmentInt("BENCHMARK_WIDTH", 600)
	benchmarkHeight := benchmarkEnvironmentInt("BENCHMARK_HEIGHT", 600)
	benchmarkDensity := benchmarkEnvironmentFloat("BENCHMARK_DENSITY", 0.25)
	rules := ContaminationConfig{CloseRadius: 4, CloseChance: 0.5, FarRadius: 20, FarChance: 0.15}
	board := RandomBoard(benchmarkWidth, benchmarkHeight, benchmarkDensity, rand.New(rand.NewSource(benchmarkSeed)))
	source := rand.New(rand.NewSource(benchmarkSeed))
	board.ensureCandidateOffsets(rules.FarRadius)

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		board.Step(rules, source)
	}
}

func BenchmarkPopulationMapTick(b *testing.B) {
	rules := ContaminationConfig{CloseRadius: 2, CloseChance: 0.5, FarRadius: 15, FarChance: 0.15}
	populationMap := GeneratePopulationMap(GeneratedMapWidth, GeneratedMapHeight, GeneratedMapSeed)
	source := rand.New(rand.NewSource(GeneratedMapSeed))
	populationMap.ensureCandidateOffsets(rules.FarRadius)

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		populationMap.Step(rules, source)
	}
}
