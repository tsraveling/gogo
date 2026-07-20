# Netcode

How gogo streams live board state from OGS into bubbletea.

## Where the board comes from

- **The realtime socket** (`game/<id>/gamedata`) is only a **"state changed"
  trigger.** Despite the name, `gamedata` does *not* contain a rendered board —
  it carries `moves`, `initial_state`, and `initial_player`. Reconstructing the
  board from those requires replaying moves *with capture logic*, which we don't
  have yet.
- **The computed board** comes from a plain REST call:
  `GET /termination-api/game/<id>/state`, which returns a ready `board [][]int`
  (0/1/2 = empty/black/white, `[y][x]`) plus `move_number`, `player_to_move`,
  `phase`, `last_move`. This is `fetchBoardState` in `ogsState.go`.

So: the socket tells us *when* to refresh; the REST endpoint tells us *what* the
board looks like. (`gamedata` fires on connect and during scoring/finished, so
the initial board loads as soon as we subscribe.)

**Socket authentication is required.** OGS will not stream a game you're a party
to until you emit `authenticate` with the `chat_auth` token from
`/api/v1/ui/config`. Without it the socket connects but `gamedata` never fires.
We authenticate but stay read-only (no move submission).

**Emit only after the connection opens.** Emitting `authenticate`/`game/connect`
before the Socket.IO handshake completes silently loses the subscription — so
both emits happen inside the `OnConnection` handler, not right after `Dial`.

## The problem the bridge solves

`graarh/golang-socketio` is **callback-based**: `c.On(...)` invokes your
function from its own goroutine. Bubble Tea is the opposite — a single-threaded
Elm loop where all state changes happen inside `Update(msg)`. A socket callback
can't touch the model directly. The bridge connects the two.

## The listen pattern

1. The model owns one buffered channel, `events chan gameEvent`, created in
   `newModel`.
2. On each socket trigger, `connectGameCmd` (in `bridge.go`) fetches the board
   via `fetchBoardState` and pushes a `gameEvent{gameID, state}` onto the
   channel. The fetch runs in the socket goroutine, never blocking the UI.
3. `waitForGameEvent(ch)` is a `tea.Cmd` that **blocks** on `<-ch` and returns
   the event as a `tea.Msg`. It is started once in `Init` and lives for the
   whole app.
4. `Update` handles the `gameEvent`, then **re-issues** `waitForGameEvent` so
   the loop keeps draining. This re-issue is the heart of the pattern: one event
   in, one new listener out.

```
socket trigger → fetchBoardState → events chan → waitForGameEvent (blocks) → tea.Msg → Update → re-issue
```

## Connection lifecycle

Only the **focused** game holds a socket (one connection at a time).

- `syncFocus()` runs after any change to the active tab (tab nav, open, close).
  It compares the focused game's id to `socketGameID`; if they differ it
  disconnects the old socket and dials the new one via `connectGameCmd`.
- Dialing happens off the UI goroutine (inside the cmd). The resulting
  `*gameSocket` returns as a `socketConnectedMsg` so the model can store it and
  disconnect it later. If focus moved on mid-dial, the stale socket is dropped.
- Disconnect fires on tab switch, tab close, and app quit.

## Routing

Snapshots and connection results carry a `gameID`. The model routes them to the
matching `gameModel` via `gameByID` — the same by-id routing used for
`navErrorExpiredMsg`. Events for a closed tab match nothing and are ignored.

## Loading state

On focus, a game with no snapshot shows a centered spinner (`gameModel.connecting`)
until its first `gamedata`. A game that already has a cached snapshot renders it
instantly and refetches silently in the background.

## Scope (for now)

Read-only snapshot. We authenticate the socket, subscribe, and fetch/render the
board on each trigger. No move submission and no incremental `move`/`clock`
handling yet — so the board loads on focus but won't update stone-by-stone until
the moves spec lands (`gamedata` refreshes it on connect and scoring/finished).

## Files

- `ogsSocket.go` (`@region ogs:realtime`) — the Socket.IO client: authenticate,
  `game/connect`, `gamedata` trigger, `Disconnect`. Adapted from termsuji.
- `ogsState.go` (`@region ogs:state`) — `fetchBoardState` (the computed board
  from `termination-api`).
- `bridge.go` (`@region ogs:bridge`) — `gameEvent`, `waitForGameEvent`,
  `connectGameCmd` (trigger → fetch → push), `socketConnectedMsg`.
- `model.go` — owns the channel + socket, `syncFocus`, event routing.
