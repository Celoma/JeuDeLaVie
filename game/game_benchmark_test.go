package game

import (
	"math/rand"
	"testing"
)

func BenchmarkBoardStep(b *testing.B) {
	rules := ContaminationConfig{CloseRadius: 4, CloseChance: 0.5, FarRadius: 20, FarChance: 0.15}
	board := RandomBoard(200, 200, 0.25, rand.New(rand.NewSource(42)))
	source := rand.New(rand.NewSource(42))

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		board.Step(rules, source)
	}
}
