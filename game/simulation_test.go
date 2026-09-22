package game

import "testing"

func TestSimulationResetReplaysTheSameInitialState(t *testing.T) {
	simulation := NewSimulation(12, 12, 0.25, 42, defaultRules())
	initial := append([]CellState(nil), simulation.Board.Cells...)

	simulation.Step()
	simulation.Reset()

	if simulation.Tick != 0 {
		t.Fatalf("expected reset tick to be zero, got %d", simulation.Tick)
	}
	for index, cell := range simulation.Board.Cells {
		if cell != initial[index] {
			t.Fatalf("expected reset to reproduce cell %d", index)
		}
	}
}

func TestSimulationResetWithSeedChangesTheInitialState(t *testing.T) {
	simulation := NewSimulation(12, 12, 0.25, 42, defaultRules())
	initial := append([]CellState(nil), simulation.Board.Cells...)

	simulation.ResetWithSeed(43)

	if simulation.Seed() != 43 {
		t.Fatalf("expected seed 43, got %d", simulation.Seed())
	}
	for index, cell := range simulation.Board.Cells {
		if cell != initial[index] {
			return
		}
	}
	t.Fatal("expected a different seed to produce a different initial state")
}

func TestContaminationConfigValidate(t *testing.T) {
	valid := defaultRules()
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid rules, got %v", err)
	}

	invalid := valid
	invalid.FarRadius = invalid.CloseRadius - 1
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected invalid radius configuration")
	}
}

func defaultRules() ContaminationConfig {
	return ContaminationConfig{CloseRadius: 2, CloseChance: 0.5, FarRadius: 15, FarChance: 0.15, DeathChance: 0.02, RecoveryChance: 0.05, ImmunityChance: 0.8}
}
