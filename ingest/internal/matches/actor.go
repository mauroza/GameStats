package matches

import (
	"sort"
	"sync"
	"time"

	"github.com/mauroza/gamestats/ingest/internal/event"
)

// actor guarda el estado de una partida. Solo su goroutine lo modifica; el
// mutex existe para que el API pueda leer snapshots.
type actor struct {
	id string
	in chan event.Event

	mu          sync.Mutex
	st          Status
	gameID      string
	gameVersion string
	events      []event.Event
	players     map[string]bool // jugador -> activo
	startedAt   time.Time
	endedAt     time.Time
	lastEventAt time.Time
}

func newActor(id string) *actor {
	return &actor{
		id:      id,
		in:      make(chan event.Event, 256),
		st:      StatusActive,
		players: map[string]bool{},
	}
}

// apply incorpora un evento. Devuelve un motivo de rechazo o "".
func (a *actor) apply(e event.Event) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.gameID == "" {
		a.gameID, a.gameVersion = e.GameID, e.GameVersion
	} else if e.GameID != a.gameID {
		return "gameId distinto al de la partida (" + a.gameID + ")"
	}
	if a.st == StatusFinishing {
		if e.Action == event.MatchFinished {
			return "MATCH_FINISHED duplicado"
		}
		if e.Timestamp.After(a.endedAt) {
			return "evento posterior a MATCH_FINISHED"
		}
	}

	switch e.Action {
	case event.MatchStarted:
		a.startedAt = e.Timestamp
	case event.MatchFinished:
		a.endedAt = e.Timestamp
		a.st = StatusFinishing
	case event.PlayerJoined, event.PlayerReconnected:
		a.players[e.PlayerID] = true
	case event.PlayerLeft, event.PlayerDisconnected:
		a.players[e.PlayerID] = false
	default:
		if _, seen := a.players[e.PlayerID]; !seen && e.PlayerID != "" {
			a.players[e.PlayerID] = true
		}
	}
	a.events = append(a.events, e)
	if e.Timestamp.After(a.lastEventAt) {
		a.lastEventAt = e.Timestamp
	}
	return ""
}

func (a *actor) status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.st
}

func (a *actor) batch(st Status) Batch {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.st = st

	events := append([]event.Event(nil), a.events...)
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	b := Batch{
		MatchID:     a.id,
		GameID:      a.gameID,
		GameVersion: a.gameVersion,
		Status:      st,
		StartedAt:   a.startedAt,
		EndedAt:     a.endedAt,
		Players:     sortedPlayers(a.players, false),
		Events:      events,
	}
	if len(events) > 0 {
		if b.StartedAt.IsZero() {
			b.StartedAt = events[0].Timestamp
		}
		if b.EndedAt.IsZero() {
			b.EndedAt = events[len(events)-1].Timestamp
		}
	}
	return b
}

func (a *actor) snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Snapshot{
		MatchID:       a.id,
		GameID:        a.gameID,
		Status:        a.st,
		Players:       sortedPlayers(a.players, false),
		ActivePlayers: sortedPlayers(a.players, true),
		EventCount:    len(a.events),
		StartedAt:     a.startedAt,
		LastEventAt:   a.lastEventAt,
	}
}

// sortedPlayers devuelve los jugadores ordenados; si onlyActive, solo los activos.
func sortedPlayers(m map[string]bool, onlyActive bool) []string {
	out := make([]string, 0, len(m))
	for p, active := range m {
		if !onlyActive || active {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
