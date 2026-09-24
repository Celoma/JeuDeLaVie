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

	nextCells          []CellState
	bucketHeads        []int
	closeCounts        []int
	farCounts          []int
	candidateWaitGroup sync.WaitGroup
	candidateOffsets   []candidateOffset
	candidateRadius    int
}

type candidateOffset struct {
	deltaColumn int
	deltaRow    int
	distanceSq  int
}

type candidateJob struct {
	board         *Board
	populationMap *PopulationMap
	config        ContaminationConfig
	start         int
	end           int
	waitGroup     *sync.WaitGroup
}

var (
	candidatePoolOnce    sync.Once
	candidateJobs        chan candidateJob
	candidateWorkerCount int
)

func startCandidatePool() {
	candidateWorkerCount = runtime.GOMAXPROCS(0)
	candidateJobs = make(chan candidateJob, candidateWorkerCount)
	for range candidateWorkerCount {
		go func() {
			for job := range candidateJobs {
				if job.board != nil {
					for index := job.start; index < job.end; index++ {
						if job.board.Cells[index] != CellHealthy {
							continue
						}
						column, row := job.board.coordinates(index)
						job.board.closeCounts[index], job.board.farCounts[index] = job.board.infectionCandidateCounts(column, row, job.config)
					}
				} else {
					for index := job.start; index < job.end; index++ {
						person := job.populationMap.People[index]
						if person.Infected || person.Dead || person.Immune {
							continue
						}
						job.populationMap.closeCounts[index], job.populationMap.farCounts[index] = job.populationMap.infectionCandidateCounts(index, job.config)
					}
				}
				job.waitGroup.Done()
			}
		}()
	}
}

func (board *Board) MarshalJSON() ([]byte, error) {
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
	candidatePoolOnce.Do(startCandidatePool)
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
		closeCounts: make([]int, cellCount),
		farCounts:   make([]int, cellCount),
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
	board.ensureCandidateOffsets(config.FarRadius)
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

	for _, offset := range board.candidateOffsets {
		candidateColumn := column + offset.deltaColumn
		candidateRow := row + offset.deltaRow
		if candidateColumn < 0 || candidateColumn >= board.Width || candidateRow < 0 || candidateRow >= board.Height {
			continue
		}
		infectedIndex := board.bucketHeads[candidateRow*board.Width+candidateColumn]
		if infectedIndex != -1 {
			if offset.distanceSq <= closeRadiusSquared {
				closeCandidates++
			} else if offset.distanceSq <= farRadiusSquared {
				farCandidates++
			}
		}
	}

	return closeCandidates, farCandidates
}

func (board *Board) ensureCandidateOffsets(radius int) {
	if board.candidateRadius == radius && board.candidateOffsets != nil {
		return
	}
	board.candidateOffsets = buildCandidateOffsets(radius)
	board.candidateRadius = radius
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

	candidatePoolOnce.Do(startCandidatePool)
	chunkSize := (len(board.Cells) + workerCount - 1) / workerCount
	board.candidateWaitGroup.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		start := worker * chunkSize
		end := min(len(board.Cells), start+chunkSize)
		candidateJobs <- candidateJob{board: board, config: config, start: start, end: end, waitGroup: &board.candidateWaitGroup}
	}
	board.candidateWaitGroup.Wait()
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

func buildCandidateOffsets(radius int) []candidateOffset {
	offsets := make([]candidateOffset, 0, (2*radius+1)*(2*radius+1))
	radiusSquared := radius * radius
	for deltaRow := -radius; deltaRow <= radius; deltaRow++ {
		for deltaColumn := -radius; deltaColumn <= radius; deltaColumn++ {
			distanceSquared := deltaColumn*deltaColumn + deltaRow*deltaRow
			if distanceSquared <= radiusSquared {
				offsets = append(offsets, candidateOffset{
					deltaColumn: deltaColumn,
					deltaRow:    deltaRow,
					distanceSq:  distanceSquared,
				})
			}
		}
	}
	return offsets
}

func (board *Board) coordinates(index int) (int, int) {
	row := index / board.Width
	column := index % board.Width
	return column, row
}

func (board *Board) index(column, row int) int {
	return row*board.Width + column
}
