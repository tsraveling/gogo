package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// @region local:persist

// Local (hotseat/offline) games persisted to ~/.config/gogo/local_games.json.
// Unlike OGS games, there's no server to refetch from — the full position,
// the ko reference, and the move list all live here.
const localGamesFileName = "local_games.json"

// On-disk store: a monotonic id source plus every local game.
type localStore struct {
	NextID int64             `json:"next_id"` // next id to hand out; negative, decreasing
	Games  []localGameRecord `json:"games"`
}

// A single persisted local game: meta + full board + ko reference + move list.
// Grids serialize as [][]stoneColor (numeric); nil grids become null.
type localGameRecord struct {
	ID       int64          `json:"id"`
	Name     string         `json:"name"`
	Ruleset  string         `json:"ruleset"`
	Width    int            `json:"width"`
	Height   int            `json:"height"`
	Komi     float64        `json:"komi"`
	Handicap int            `json:"handicap"`
	You      stoneColor     `json:"you"`
	Black    string         `json:"black"`
	White    string         `json:"white"`
	State    wireState      `json:"state"`
	PrevGrid [][]stoneColor `json:"prev_grid"`
	Moves    []wireMove     `json:"moves"`
}

type wireState struct {
	Grid         [][]stoneColor `json:"grid"`
	MoveNumber   int            `json:"move_number"`
	PlayerToMove stoneColor     `json:"player_to_move"`
	LastMove     wireMove       `json:"last_move"`
	Phase        string         `json:"phase"`
}

type wireMove struct {
	X     int        `json:"x"`
	Y     int        `json:"y"`
	Color stoneColor `json:"color"`
}

func (m move) wire() wireMove { return wireMove{X: m.x, Y: m.y, Color: m.color} }
func (w wireMove) move() move { return move{x: w.X, y: w.Y, color: w.Color} }

func localGamesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gogo", localGamesFileName), nil
}

// Reads the store. A missing file yields an empty store seeded at id -1.
func loadLocalStore() (localStore, error) {
	s := localStore{NextID: -1}
	path, err := localGamesPath()
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	if s.NextID >= 0 {
		s.NextID = -1 // guard a corrupt/zero counter into the negative range
	}
	return s, nil
}

func saveLocalStore(s localStore) error {
	path, err := localGamesPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// @region local:model

// Builds a persistable record from meta plus live backend state.
func toRecord(meta game, st boardState, prevGrid [][]stoneColor, moves []move) localGameRecord {
	wm := make([]wireMove, len(moves))
	for i, m := range moves {
		wm[i] = m.wire()
	}
	return localGameRecord{
		ID:       meta.id,
		Name:     meta.name,
		Ruleset:  string(meta.ruleset),
		Width:    meta.width,
		Height:   meta.height,
		Komi:     meta.komi,
		Handicap: meta.handicap,
		You:      meta.you,
		Black:    meta.black.name,
		White:    meta.white.name,
		State: wireState{
			Grid:         st.grid,
			MoveNumber:   st.moveNumber,
			PlayerToMove: st.playerToMove,
			LastMove:     st.lastMove.wire(),
			Phase:        string(st.phase),
		},
		PrevGrid: prevGrid,
		Moves:    wm,
	}
}

// Reconstructs the core game (meta + state) from a record.
func (r localGameRecord) toGame() game {
	return game{
		id:       r.ID,
		name:     r.Name,
		ruleset:  ruleset(r.Ruleset),
		width:    r.Width,
		height:   r.Height,
		komi:     r.Komi,
		handicap: r.Handicap,
		you:      r.You,
		black:    player{name: r.Black},
		white:    player{name: r.White},
		state: boardState{
			grid:         r.State.Grid,
			moveNumber:   r.State.MoveNumber,
			playerToMove: r.State.PlayerToMove,
			lastMove:     r.State.LastMove.move(),
			phase:        gamePhase(r.State.Phase),
		},
	}
}

func (r localGameRecord) moveList() []move {
	ms := make([]move, len(r.Moves))
	for i, w := range r.Moves {
		ms[i] = w.move()
	}
	return ms
}

// @region local:store-ops

// Persists a local game, replacing any existing record with the same id.
// Called on creation and after every committed move.
func persistLocalGame(meta game, st boardState, prevGrid [][]stoneColor, moves []move) error {
	s, err := loadLocalStore()
	if err != nil {
		return err
	}
	rec := toRecord(meta, st, prevGrid, moves)
	replaced := false
	for i := range s.Games {
		if s.Games[i].ID == rec.ID {
			s.Games[i] = rec
			replaced = true
			break
		}
	}
	if !replaced {
		s.Games = append(s.Games, rec)
	}
	return saveLocalStore(s)
}

// Removes a local game from the store. No-op if the id isn't present.
func deleteLocalGame(id int64) error {
	s, err := loadLocalStore()
	if err != nil {
		return err
	}
	out := s.Games[:0]
	for _, r := range s.Games {
		if r.ID != id {
			out = append(out, r)
		}
	}
	s.Games = out
	return saveLocalStore(s)
}

// Creates, persists, and returns a fresh local game (black to move, empty board).
// Ids are negative and monotonically decreasing. Used by the setup modal.
func newLocalGame(size boardSize, rs ruleset, komi float64, blackName, whiteName string) (game, error) {
	s, err := loadLocalStore()
	if err != nil {
		return game{}, err
	}
	id := s.NextID
	s.NextID--

	w, h := size.dims()
	st := newBoardState(w, h)
	g := game{
		id:      id,
		name:    blackName + " vs " + whiteName,
		ruleset: rs,
		width:   w,
		height:  h,
		komi:    komi,
		you:     empty, // hotseat: either side is local
		black:   player{name: blackName},
		white:   player{name: whiteName},
		state:   st,
	}

	rec := toRecord(g, st, cloneGrid(st.grid), nil)
	s.Games = append(s.Games, rec)
	if err := saveLocalStore(s); err != nil {
		return game{}, err
	}
	return g, nil
}

// Returns every persisted local game as a core game (for the home list).
func loadLocalGames() ([]game, error) {
	s, err := loadLocalStore()
	if err != nil {
		return nil, err
	}
	games := make([]game, len(s.Games))
	for i, r := range s.Games {
		games[i] = r.toGame()
	}
	return games, nil
}

// Loads one local game and a hotseat backend restored to its saved position.
// Returns ok=false when no record matches the id.
func openLocalGame(id int64) (game, *hotseatBackend, bool) {
	s, err := loadLocalStore()
	if err != nil {
		return game{}, nil, false
	}
	for _, r := range s.Games {
		if r.ID == id {
			g := r.toGame()
			b := buildLocalBackend(g, r.PrevGrid, r.moveList())
			return g, b, true
		}
	}
	return game{}, nil, false
}

// Builds a hotseat backend wired to persist after every committed move.
func buildLocalBackend(g game, prevGrid [][]stoneColor, moves []move) *hotseatBackend {
	b := newHotseatBackend(g.state, prevGrid)
	b.moves = moves
	b.onCommit = func(hb *hotseatBackend) {
		_ = persistLocalGame(g, hb.state, hb.prevGrid, hb.moves)
	}
	return b
}
