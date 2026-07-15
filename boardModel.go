package main

// boardModel renders a single game's board state. Stub for now.
type boardModel struct {
	width  int // board size in points (e.g. 9 for 9x9)
	height int
}

func newBoardModel(w, h int) boardModel {
	return boardModel{width: w, height: h}
}

// renderWidth is the board rect width: w*2 points + 2 for the letter margin.
func (b boardModel) renderWidth() int {
	return b.width*2 + 2
}

// renderHeight is the board rect height: h rows + 1 for the number margin.
func (b boardModel) renderHeight() int {
	return b.height + 1
}

func (b boardModel) View() string {
	return renderPanel("BOARD", b.renderWidth(), b.renderHeight(), boardBg)
}
