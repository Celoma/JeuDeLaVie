package game

import (
	"encoding/json"
	"math/rand"
	"testing"
)

func TestBoardJSONUsesCellArray(t *testing.T) {
	board := &Board{Width: 2, Height: 2, Cells: []CellState{CellEmpty, CellHealthy, CellInfected, CellEmpty}}

	encoded, err := json.Marshal(board)
	if err != nil {
		t.Fatalf("failed to marshal board: %v", err)
	}
	var decoded struct {
		Cells []int `json:"cells"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("failed to decode board JSON: %v", err)
	}
	if len(decoded.Cells) != len(board.Cells) {
		t.Fatalf("expected %d cells, got %d", len(board.Cells), len(decoded.Cells))
	}
}

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

func TestStepInfectsWithCloseNeighbors(t *testing.T) {
	board := NewBoard(5, 3)
	board.Cells[1] = CellInfected
	board.Cells[3] = CellInfected
	board.Cells[2] = CellHealthy

	board.Step(ContaminationConfig{
		CloseRadius: 1,
		CloseChance: 1,
		FarRadius:   1,
		FarChance:   0,
	}, rand.New(rand.NewSource(1)))

	if board.Cells[2] != CellInfected {
		t.Fatal("expected the healthy cell to be infected by its close neighbors")
	}
}

func TestInfectedPersonCanDie(t *testing.T) {
	board := NewBoard(1, 1)
	board.Cells[0] = CellInfected
	board.Step(ContaminationConfig{DeathChance: 1}, rand.New(rand.NewSource(1)))

	if board.Cells[0] != CellDead {
		t.Fatalf("expected infected cell to die, got %d", board.Cells[0])
	}
}

func TestInfectedPersonCanRecoverWithImmunity(t *testing.T) {
	board := NewBoard(1, 1)
	board.Cells[0] = CellInfected
	board.Step(ContaminationConfig{RecoveryChance: 1, ImmunityChance: 1}, rand.New(rand.NewSource(1)))

	if board.Cells[0] != CellImmune {
		t.Fatalf("expected infected cell to become immune, got %d", board.Cells[0])
	}
}

func TestInfectedPersonCanRecoverWithoutImmunity(t *testing.T) {
	board := NewBoard(1, 1)
	board.Cells[0] = CellInfected
	board.Step(ContaminationConfig{RecoveryChance: 1}, rand.New(rand.NewSource(1)))

	if board.Cells[0] != CellHealthy {
		t.Fatalf("expected infected cell to recover as healthy, got %d", board.Cells[0])
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
