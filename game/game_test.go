package game

import (
	"math/rand"
	"testing"
)

func TestStepSpreadsInfectionByRadius(t *testing.T) {
	board := NewBoard(40, 40)
	board.Cells[20*board.Width+20] = CellInfected
	board.Cells[20*board.Width+22] = CellHealthy
	board.Cells[39*board.Width+39] = CellHealthy

	board.Step(ContaminationConfig{
		CloseRadius: 2,
		CloseChance: 1,
		FarRadius:   15,
		FarChance:   1,
	}, rand.New(rand.NewSource(1)))

	if board.Cells[20*board.Width+22] != CellInfected {
		t.Fatalf("expected close target to be infected")
	}
	if board.Cells[39*board.Width+39] != CellHealthy {
		t.Fatalf("expected far target to stay healthy beyond the configured radius")
	}
}

func TestRandomBoardCreatesOneInfectedPerson(t *testing.T) {
	board := RandomBoard(6, 6, 0.5, rand.New(rand.NewSource(42)))
	infectedCount := 0
	for _, cell := range board.Cells {
		if cell == CellInfected {
			infectedCount++
		}
	}
	if infectedCount != 1 {
		t.Fatalf("expected exactly one infected person, got %d", infectedCount)
	}
}
