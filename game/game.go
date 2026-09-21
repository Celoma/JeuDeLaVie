package game

import "math/rand"

type Board struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Cells  []bool `json:"cells"`
}

func NewBoard(width, height int) *Board {
	return &Board{
		Width:  width,
		Height: height,
		Cells:  make([]bool, width*height),
	}
}

func RandomBoard(width, height int, density float64, source *rand.Rand) *Board {
	board := NewBoard(width, height)
	for index := range board.Cells {
		board.Cells[index] = source.Float64() < density
	}
	return board
}

func (board *Board) Step() {
	next := make([]bool, len(board.Cells))
	for row := 0; row < board.Height; row++ {
		for column := 0; column < board.Width; column++ {
			neighbors := board.liveNeighbors(column, row)
			alive := board.Cells[board.index(column, row)]
			next[board.index(column, row)] = neighbors == 3 || alive && neighbors == 2
		}
	}
	board.Cells = next
}

func (board *Board) liveNeighbors(column, row int) int {
	count := 0
	for rowOffset := -1; rowOffset <= 1; rowOffset++ {
		for columnOffset := -1; columnOffset <= 1; columnOffset++ {
			if columnOffset == 0 && rowOffset == 0 {
				continue
			}
			neighborColumn := column + columnOffset
			neighborRow := row + rowOffset
			if neighborColumn >= 0 && neighborColumn < board.Width && neighborRow >= 0 && neighborRow < board.Height && board.Cells[board.index(neighborColumn, neighborRow)] {
				count++
			}
		}
	}
	return count
}

func (board *Board) index(column, row int) int {
	return row*board.Width + column
}
