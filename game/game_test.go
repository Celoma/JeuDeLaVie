package game

import "testing"

func TestStepKeepsBlinkerOscillating(t *testing.T) {
	board := NewBoard(5, 5)
	board.Cells[1*board.Width+2] = true
	board.Cells[2*board.Width+2] = true
	board.Cells[3*board.Width+2] = true

	board.Step()

	expected := []int{2*board.Width + 1, 2*board.Width + 2, 2*board.Width + 3}
	for _, index := range expected {
		if !board.Cells[index] {
			t.Fatalf("expected cell %d to be alive", index)
		}
	}
}
