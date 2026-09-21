package game

import "math/rand"

type CellState uint8

const (
	CellEmpty CellState = iota
	CellHealthy
	CellInfected
)

type ContaminationConfig struct {
	CloseRadius int     `json:"closeRadius"`
	CloseChance float64 `json:"closeChance"`
	FarRadius   int     `json:"farRadius"`
	FarChance   float64 `json:"farChance"`
}

type Board struct {
	Width  int         `json:"width"`
	Height int         `json:"height"`
	Cells  []CellState `json:"cells"`
}

func NewBoard(width, height int) *Board {
	return &Board{
		Width:  width,
		Height: height,
		Cells:  make([]CellState, width*height),
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
	next := make([]CellState, len(board.Cells))
	copy(next, board.Cells)

	infectedPositions := make([]int, 0)
	for index, cell := range board.Cells {
		if cell == CellInfected {
			infectedPositions = append(infectedPositions, index)
		}
	}

	for index, cell := range board.Cells {
		if cell != CellHealthy {
			continue
		}
		column, row := board.coordinates(index)
		if board.shouldInfect(column, row, infectedPositions, config, source) {
			next[index] = CellInfected
		}
	}

	board.Cells = next
}

func (board *Board) shouldInfect(column, row int, infectedPositions []int, config ContaminationConfig, source *rand.Rand) bool {
	closeRadiusSquared := config.CloseRadius * config.CloseRadius
	farRadiusSquared := config.FarRadius * config.FarRadius
	farCandidate := false

	for _, infectedIndex := range infectedPositions {
		infectedColumn, infectedRow := board.coordinates(infectedIndex)
		distance := squaredDistance(column, row, infectedColumn, infectedRow)
		if distance == 0 {
			continue
		}
		if distance <= closeRadiusSquared {
			return source.Float64() < config.CloseChance
		}
		if distance <= farRadiusSquared {
			farCandidate = true
		}
	}

	if farCandidate {
		return source.Float64() < config.FarChance
	}
	return false
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
