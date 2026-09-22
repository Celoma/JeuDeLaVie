package game

import "math/rand"

type Simulation struct {
	Board *Board
	Rules ContaminationConfig
	Tick  uint64

	width   int
	height  int
	density float64
	seed    int64
	rng     *rand.Rand
}

func NewSimulation(width, height int, density float64, seed int64, rules ContaminationConfig) *Simulation {
	simulation := &Simulation{
		width:   width,
		height:  height,
		density: density,
		seed:    seed,
		Rules:   rules,
	}
	simulation.rng = rand.New(rand.NewSource(seed))
	simulation.Board = RandomBoard(width, height, density, simulation.rng)
	return simulation
}

func (simulation *Simulation) Step() *Board {
	simulation.Board.Step(simulation.Rules, simulation.rng)
	simulation.Tick++
	return simulation.Board
}

func (simulation *Simulation) Reset() *Board {
	return simulation.ResetWithSeed(simulation.seed)
}

func (simulation *Simulation) ResetWithSeed(seed int64) *Board {
	simulation.seed = seed
	simulation.rng = rand.New(rand.NewSource(simulation.seed))
	simulation.Board = RandomBoard(simulation.width, simulation.height, simulation.density, simulation.rng)
	simulation.Tick = 0
	return simulation.Board
}

func (simulation *Simulation) Seed() int64 {
	return simulation.seed
}

func (config ContaminationConfig) Validate() error {
	if config.CloseRadius < 0 {
		return errInvalidRule("closeRadius must be greater than or equal to zero")
	}
	if config.FarRadius < config.CloseRadius {
		return errInvalidRule("farRadius must be greater than or equal to closeRadius")
	}
	if config.CloseChance < 0 || config.CloseChance > 1 {
		return errInvalidRule("closeChance must be between zero and one")
	}
	if config.FarChance < 0 || config.FarChance > 1 {
		return errInvalidRule("farChance must be between zero and one")
	}
	if config.DeathChance < 0 || config.DeathChance > 1 {
		return errInvalidRule("deathChance must be between zero and one")
	}
	if config.RecoveryChance < 0 || config.RecoveryChance > 1 {
		return errInvalidRule("recoveryChance must be between zero and one")
	}
	if config.ImmunityChance < 0 || config.ImmunityChance > 1 {
		return errInvalidRule("immunityChance must be between zero and one")
	}
	return nil
}

type invalidRuleError string

func (errorValue invalidRuleError) Error() string { return string(errorValue) }

func errInvalidRule(message string) error { return invalidRuleError(message) }
