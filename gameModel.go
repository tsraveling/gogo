package main

import "github.com/charmbracelet/lipgloss"

// gameModel wraps a single active game: board column + meta column.
type gameModel struct {
	name  string
	board boardModel
	info  infoModel
	chat  chatModel
}

func newGameModel(name string, w, h int) gameModel {
	return gameModel{
		name:  name,
		board: newBoardModel(w, h),
		info:  newInfoModel(),
		chat:  newChatModel(),
	}
}

func (g gameModel) View(termW, termH int) string {
	boardW := g.board.renderWidth()

	// Board column: board on top, 2-row control below.
	control := renderPanel("UI", boardW, 2, controlBg)
	boardCol := lipgloss.JoinVertical(lipgloss.Left, g.board.View(), control)
	colHeight := lipgloss.Height(boardCol)

	// Meta column fills remaining width; info 6 rows, chat gets the rest.
	metaW := max(termW-boardW, 0)
	infoH := 6
	chatH := max(colHeight-infoH, 0)
	metaCol := lipgloss.JoinVertical(
		lipgloss.Left,
		g.info.View(metaW, infoH),
		g.chat.View(metaW, chatH),
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, boardCol, metaCol)
}
