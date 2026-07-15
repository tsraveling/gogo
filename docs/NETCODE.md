# Netcode

How gogo streams live board state from OGS into the Bubble Tea UI.

## The problem

OGS realtime is a Socket.IO connection. The library (`graarh/golang-socketio`)
is **callback-based**: you register `c.On("game/<id>/gamedata", fn)` and it
invokes `fn` from its own goroutine whenever a snapshot arrives.

Bubble Tea is the opposite — a single-threaded Elm loop where all state changes
happen inside `Update(msg)`. A socket callback can't touch the model directly.
The bridge connects the two.

## The listen pattern

1. The model owns one buffered channel, `events chan gameEvent`, created in
   `newModel`.
2. The socket's `gamedata` callback parses the snapshot and pushes a
   `gameEvent{gameID, state}` onto that channel (`connectGameCmd` in
   `bridge.go`). The callback never blocks the UI — it only sends on a channel.
3. `waitForGameEvent(ch)` is a `tea.Cmd` that **blocks** on `<-ch` and returns
   the event as a `tea.Msg`. It is started once in `Init` and lives for the
   whole app.
4. `Update` handles the `gameEvent`, then **re-issues** `waitForGameEvent` so
   the loop keeps draining. This re-issue is the heart of the pattern: one event
   in, one new listener out.

```
socket goroutine → events chan → waitForGameEvent (blocks) → tea.Msg → Update → re-issue
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

Read-only. We subscribe to `gamedata` and render the snapshot. No socket
`authenticate`, no move submission, no incremental `move`/`clock` handling yet.

## Files

- `ogsSocket.go` (`@region ogs:realtime`) — the Socket.IO client, `gamedata`
  parsing, `Disconnect`. Adapted from termsuji.
- `bridge.go` (`@region ogs:bridge`) — `gameEvent`, `waitForGameEvent`,
  `connectGameCmd`, `socketConnectedMsg`.
- `model.go` — owns the channel + socket, `syncFocus`, event routing.
