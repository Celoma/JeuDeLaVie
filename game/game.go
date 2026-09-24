package game

import (
	"encoding/json"
	"math/rand"
	"runtime"
	"sync"
)

type CellState uint8

const (
	CellEmpty CellState = iota
	CellHealthy
	CellInfected
	CellDead
	CellImmune
)

type ContaminationConfig struct {
	CloseRadius    int     `json:"closeRadius"`
	CloseChance    float64 `json:"closeChance"`
	FarRadius      int     `json:"farRadius"`
	FarChance      float64 `json:"farChance"`
	DeathChance    float64 `json:"deathChance"`
	RecoveryChance float64 `json:"recoveryChance"`
	ImmunityChance float64 `json:"immunityChance"`
}

type Board struct {
	Width  int         `json:"width"`
	Height int         `json:"height"`
	Cells  []CellState `json:"cells"`

	nextCells   []CellState
	bucketHeads []int
	closeCounts []int
	farCounts   []int
}

func (board Board) MarshalJSON() ([]byte, error) {
	cells := make([]int, len(board.Cells))
	for index, cell := range board.Cells {
		cells[index] = int(cell)
	}
	return json.Marshal(struct {
		Width  int   `json:"width"`
		Height int   `json:"height"`
		Cells  []int `json:"cells"`
	}{board.Width, board.Height, cells})
}

func NewBoard(width, height int) *Board {
	cellCount := width * height
	bucketHeads := make([]int, cellCount)
	for index := range bucketHeads {
		bucketHeads[index] = -1
	}
	return &Board{
		Width:       width,
		Height:      height,
		Cells:       make([]CellState, cellCount),
		nextCells:   make([]CellState, cellCount),
		bucketHeads: bucketHeads,
	}
}

func RandomBoard(width, height int, density float64, source *rand.Rand) *Board {
	board := NewBoard(width, height)
	healthyCells := make([]int, 0, len(board.Cells))
	for index := range board.Cells {
		if source.Float64() < density {
			board.Cells[index] = CellHealthy
			healthyCells = append(healthyCells, index)
		}
	}
	if len(healthyCells) == 0 {
		board.Cells[source.Intn(len(board.Cells))] = CellInfected
		return board
	}
	board.Cells[healthyCells[source.Intn(len(healthyCells))]] = CellInfected
	return board
}

func (board *Board) Step(config ContaminationConfig, source *rand.Rand) {
	board.ensureScratch()
	next := board.nextCells
	copy(next, board.Cells)

	for index, cell := range board.Cells {
		if cell != CellInfected {
			continue
		}
		if source.Float64() < config.DeathChance {
			next[index] = CellDead
			continue
		}
		if source.Float64() < config.RecoveryChance {
			if source.Float64() < config.ImmunityChance {
				next[index] = CellImmune
			} else {
				next[index] = CellHealthy
			}
		}
	}

	for index := range board.bucketHeads {
		board.bucketHeads[index] = -1
	}
	for index, cell := range next {
		if cell != CellInfected {
			continue
		}
		board.bucketHeads[index] = index
	}

	board.countInfectionCandidates(config)
	for index, cell := range board.Cells {
		if cell != CellHealthy {
			continue
		}
		if shouldInfectFromCounts(board.closeCounts[index], board.farCounts[index], config, source) {
			next[index] = CellInfected
		}
	}

	board.Cells, board.nextCells = next, board.Cells
}

func (board *Board) shouldInfect(column, row int, config ContaminationConfig, source *rand.Rand) bool {
	closeCandidates, farCandidates := board.infectionCandidateCounts(column, row, config)
	return shouldInfectFromCounts(closeCandidates, farCandidates, config, source)
}

func shouldInfectFromCounts(closeCandidates, farCandidates int, config ContaminationConfig, source *rand.Rand) bool {
	if closeCandidates > 0 {
		for range closeCandidates {
			if source.Float64() < config.CloseChance {
				return true
			}
		}
		return false
	}
	for range farCandidates {
		if source.Float64() < config.FarChance {
			return true
		}
	}
	return false
}

func (board *Board) infectionCandidateCounts(column, row int, config ContaminationConfig) (int, int) {
	closeRadiusSquared := config.CloseRadius * config.CloseRadius
	farRadiusSquared := config.FarRadius * config.FarRadius
	closeCandidates := 0
	farCandidates := 0

	minColumn := max(0, column-config.FarRadius)
	maxColumn := min(board.Width-1, column+config.FarRadius)
	minRow := max(0, row-config.FarRadius)
	maxRow := min(board.Height-1, row+config.FarRadius)
	for candidateRow := minRow; candidateRow <= maxRow; candidateRow++ {
		for candidateColumn := minColumn; candidateColumn <= maxColumn; candidateColumn++ {
			infectedIndex := board.bucketHeads[board.index(candidateColumn, candidateRow)]
			if infectedIndex != -1 {
				distance := squaredDistance(column, row, candidateColumn, candidateRow)
				if distance <= closeRadiusSquared {
					closeCandidates++
				} else if distance <= farRadiusSquared {
					farCandidates++
				}
			}
		}
	}

	return closeCandidates, farCandidates
}

func (board *Board) countInfectionCandidates(config ContaminationConfig) {
	workerCount := min(runtime.GOMAXPROCS(0), (len(board.Cells)+1023)/1024)
	if workerCount < 2 {
		for index, cell := range board.Cells {
			if cell != CellHealthy {
				continue
			}
			column, row := board.coordinates(index)
			board.closeCounts[index], board.farCounts[index] = board.infectionCandidateCounts(column, row, config)
		}
		return
	}

	var waitGroup sync.WaitGroup
	chunkSize := (len(board.Cells) + workerCount - 1) / workerCount
	waitGroup.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		start := worker * chunkSize
		end := min(len(board.Cells), start+chunkSize)
		go func() {
			defer waitGroup.Done()
			for index := start; index < end; index++ {
				if board.Cells[index] != CellHealthy {
					continue
				}
				column, row := board.coordinates(index)
				board.closeCounts[index], board.farCounts[index] = board.infectionCandidateCounts(column, row, config)
			}
		}()
	}
	waitGroup.Wait()
}

func (board *Board) ensureScratch() {
	cellCount := len(board.Cells)
	if len(board.nextCells) == cellCount && len(board.bucketHeads) == cellCount && len(board.closeCounts) == cellCount && len(board.farCounts) == cellCount {
		return
	}
	board.nextCells = make([]CellState, cellCount)
	board.bucketHeads = make([]int, cellCount)
	board.closeCounts = make([]int, cellCount)
	board.farCounts = make([]int, cellCount)
	for index := range board.bucketHeads {
		board.bucketHeads[index] = -1
	}
}

func squaredDistance(columnA, rowA, columnB, rowB int) int {
	columnDelta := columnA - columnB
	rowDelta := rowA - rowB
	return columnDelta*columnDelta + rowDelta*rowDelta
}

func (board *Board) coordinates(index int) (int, int) {
	row := index / board.Width
	column := index % board.Width
	return column, row
}

func (board *Board) index(column, row int) int {
	return row*board.Width + column
}
