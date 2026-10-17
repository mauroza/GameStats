package event

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mauroza/gamestats/ingest/internal/games"
)

var reg = games.Registry{
	"racing": {GameID: "racing", Actions: map[string][]string{
		"LAP_COMPLETED": {"lap", "lapTimeMs", "position"},
		"RACE_FLAG":     {"flag"},
	}, NoPlayerActions: []string{"RACE_FLAG"}},
}

func base() Event {
	return Event{
		EventID: "e1", Timestamp: time.Now(), GameID: "racing", MatchID: "m1",
		PlayerID: "p1", Type: TypeGameplay, Action: "LAP_COMPLETED",
		Data: json.RawMessage(`{"lap":1,"lapTimeMs":80000,"position":2}`),
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Event)
		wantErr bool
	}{
		{"válido", func(e *Event) {}, false},
		{"sin matchId", func(e *Event) { e.MatchID = "" }, true},
		{"sin timestamp", func(e *Event) { e.Timestamp = time.Time{} }, true},
		{"type inválido", func(e *Event) { e.Type = "X" }, true},
		{"acción desconocida", func(e *Event) { e.Action = "FLY" }, true},
		{"data incompleta", func(e *Event) { e.Data = json.RawMessage(`{"lap":1}`) }, true},
		{"gameplay sin jugador", func(e *Event) { e.PlayerID = "" }, true},
		{"acción sin jugador permitida", func(e *Event) {
			e.PlayerID, e.Action, e.Data = "", "RACE_FLAG", json.RawMessage(`{"flag":"yellow"}`)
		}, false},
		{"juego no registrado", func(e *Event) { e.GameID = "cards"; e.Action = "ANY" }, false},
		{"sistema MATCH_STARTED", func(e *Event) { e.Type = TypeSystem; e.Action = MatchStarted; e.PlayerID = "" }, false},
		{"sistema sin jugador", func(e *Event) { e.Type = TypeSystem; e.Action = PlayerJoined; e.PlayerID = "" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := base()
			tt.mutate(&e)
			if err := Validate(e, reg); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
