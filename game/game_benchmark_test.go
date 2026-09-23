package game

import (
	"math/rand"
	"testing"
)

func BenchmarkTick(b *testing.B) {
	const (
		benchmarkWidth  = 600
		benchmarkHeight = 600
		benchmarkSeed   = 42
	)
	rules := ContaminationConfig{CloseRadius: 4, CloseChance: 0.5, FarRadius: 20, FarChance: 0.15}
	board := RandomBoard(benchmarkWidth, benchmarkHeight, 0.25, rand.New(rand.NewSource(benchmarkSeed)))
	source := rand.New(rand.NewSource(benchmarkSeed))

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		board.Step(rules, source)
	}
}
