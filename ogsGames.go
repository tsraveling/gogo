package main

// @region ogs:games

// OGS wire structs and transformers into the light core game model.
// Source: /api/v1/ui/overview (see NEXT.md). Kept apart from the core models
// so those stay source-agnostic (OGS, gnugo, hotseat).

const overviewURL = ogsBaseURL + "/api/v1/ui/overview"

type ogsOverview struct {
	ActiveGames []ogsActiveGame `json:"active_games"`
}

type ogsActiveGame struct {
	ID     int64         `json:"id"`
	Name   string        `json:"name"`
	Width  int           `json:"width"`
	Height int           `json:"height"`
	Black  ogsGamePlayer `json:"black"`
	White  ogsGamePlayer `json:"white"`
	JSON   ogsGameJSON   `json:"json"`
}

type ogsGamePlayer struct {
	ID       int64   `json:"id"`
	Username string  `json:"username"`
	Ranking  float64 `json:"ranking"`
}

type ogsGameJSON struct {
	Rules string `json:"rules"`
	Phase string `json:"phase"`
	Clock struct {
		CurrentPlayer int64 `json:"current_player"`
	} `json:"clock"`
}

// Converts an OGS active game to the core model. myID selects which side the
// local user plays (empty when neither).
func (a ogsActiveGame) toGame(myID int64) game {
	toMove := black
	if a.JSON.Clock.CurrentPlayer == a.White.ID {
		toMove = white
	}
	you := empty
	switch myID {
	case a.Black.ID:
		you = black
	case a.White.ID:
		you = white
	}
	return game{
		id:      a.ID,
		name:    a.Name,
		ruleset: ruleset(a.JSON.Rules),
		width:   a.Width,
		height:  a.Height,
		you:     you,
		black:   player{name: a.Black.Username, rank: a.Black.Ranking},
		white:   player{name: a.White.Username, rank: a.White.Ranking},
		state:   boardState{playerToMove: toMove, phase: gamePhase(a.JSON.Phase)},
	}
}

// Returns the user's ongoing games from the OGS overview endpoint.
func fetchActiveGames(accessToken string, myID int64) ([]game, error) {
	var ov ogsOverview
	if err := authGet(overviewURL, accessToken, &ov); err != nil {
		return nil, err
	}
	games := make([]game, len(ov.ActiveGames))
	for i, a := range ov.ActiveGames {
		games[i] = a.toGame(myID)
	}
	return games, nil
}
