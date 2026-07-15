package main

import "errors"

// @region game:rules

// Pure Go rules: capture, suicide, and simple positional ko. No UI or network.
// Grids are [y][x] (matching boardState). applyMove is the single entry point;
// callers thread the previous grid for ko detection and read boardState.lastMove
// for two-pass game end.

// Move legality failures.
var (
	errOffBoard    = errors.New("off the board")
	errOccupied    = errors.New("point already occupied")
	errSuicide     = errors.New("suicide")
	errKo          = errors.New("ko")
	errNotYourTurn = errors.New("not your turn")
	errGameOver    = errors.New("game is finished")
)

// A fresh empty position: black to move, play phase.
func newBoardState(w, h int) boardState {
	grid := make([][]stoneColor, h)
	for y := range grid {
		grid[y] = make([]stoneColor, w)
	}
	return boardState{grid: grid, playerToMove: black, phase: phasePlay}
}

// The other color; empty maps to empty.
func opponent(c stoneColor) stoneColor {
	switch c {
	case black:
		return white
	case white:
		return black
	}
	return empty
}

// applyMove plays m on cur and returns the resulting state. prevGrid is the
// position one ply back, used for simple positional ko (the result may not
// recreate it). The mover is cur.playerToMove; m.color, if set, must match.
// A pass leaves the grid untouched; two passes in a row finish the game.
func applyMove(cur boardState, prevGrid [][]stoneColor, m move) (boardState, error) {
	if cur.finished() {
		return cur, errGameOver
	}
	mover := cur.playerToMove
	if m.color != empty && m.color != mover {
		return cur, errNotYourTurn
	}

	if m.isPass() {
		next := cur
		next.grid = cloneGrid(cur.grid)
		next.moveNumber = cur.moveNumber + 1
		next.lastMove = move{x: -1, y: -1, color: mover}
		if cur.lastMove.isPass() { // second pass ends the game
			next.phase = phaseFinished
			next.playerToMove = empty
		} else {
			next.playerToMove = opponent(mover)
		}
		return next, nil
	}

	if !inBounds(cur.grid, m.x, m.y) {
		return cur, errOffBoard
	}
	if cur.grid[m.y][m.x] != empty {
		return cur, errOccupied
	}

	grid := cloneGrid(cur.grid)
	grid[m.y][m.x] = mover

	// Remove any adjacent opponent group left with no liberties.
	opp := opponent(mover)
	for _, n := range orthNeighbors(grid, m.x, m.y) {
		if grid[n[1]][n[0]] != opp {
			continue
		}
		if cells, libs := collectGroup(grid, n[0], n[1]); libs == 0 {
			for _, c := range cells {
				grid[c[1]][c[0]] = empty
			}
		}
	}

	// Suicide: the placed group must have a liberty (captures above may grant one).
	if _, libs := collectGroup(grid, m.x, m.y); libs == 0 {
		return cur, errSuicide
	}

	// Simple positional ko: the move may not recreate the previous position.
	if equalGrid(grid, prevGrid) {
		return cur, errKo
	}

	next := cur
	next.grid = grid
	next.moveNumber = cur.moveNumber + 1
	next.playerToMove = opp
	next.lastMove = move{x: m.x, y: m.y, color: mover}
	next.phase = phasePlay
	return next, nil
}

// In-bounds orthogonal neighbors of (x,y), as {x,y} pairs.
func orthNeighbors(grid [][]stoneColor, x, y int) [][2]int {
	var out [][2]int
	for _, d := range [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
		nx, ny := x+d[0], y+d[1]
		if inBounds(grid, nx, ny) {
			out = append(out, [2]int{nx, ny})
		}
	}
	return out
}

func inBounds(grid [][]stoneColor, x, y int) bool {
	return y >= 0 && y < len(grid) && x >= 0 && x < len(grid[y])
}

// collectGroup returns the connected same-color group containing (x,y) and its
// liberty count (distinct adjacent empty points).
func collectGroup(grid [][]stoneColor, x, y int) (cells [][2]int, liberties int) {
	color := grid[y][x]
	seen := map[[2]int]bool{{x, y}: true}
	libs := map[[2]int]bool{}
	stack := [][2]int{{x, y}}
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		cells = append(cells, c)
		for _, n := range orthNeighbors(grid, c[0], c[1]) {
			switch grid[n[1]][n[0]] {
			case empty:
				libs[n] = true
			case color:
				if !seen[n] {
					seen[n] = true
					stack = append(stack, n)
				}
			}
		}
	}
	return cells, len(libs)
}

func cloneGrid(g [][]stoneColor) [][]stoneColor {
	out := make([][]stoneColor, len(g))
	for y := range g {
		out[y] = append([]stoneColor(nil), g[y]...)
	}
	return out
}

func equalGrid(a, b [][]stoneColor) bool {
	if len(a) != len(b) {
		return false
	}
	for y := range a {
		if len(a[y]) != len(b[y]) {
			return false
		}
		for x := range a[y] {
			if a[y][x] != b[y][x] {
				return false
			}
		}
	}
	return true
}
