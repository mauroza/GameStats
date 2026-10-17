package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mauroza/gamestats/ingest/internal/games"
)

// Validate revisa el sobre común y, si el juego está registrado, que la
// acción exista y que data traiga los campos requeridos.
func Validate(e Event, reg games.Registry) error {
	var missing []string
	if e.EventID == "" {
		missing = append(missing, "eventId")
	}
	if e.Timestamp.IsZero() {
		missing = append(missing, "timestamp")
	}
	if e.GameID == "" {
		missing = append(missing, "gameId")
	}
	if e.MatchID == "" {
		missing = append(missing, "matchId")
	}
	if e.Type == "" {
		missing = append(missing, "type")
	}
	if e.Action == "" {
		missing = append(missing, "action")
	}
	if len(missing) > 0 {
		return fmt.Errorf("campos faltantes: %s", strings.Join(missing, ", "))
	}

	switch e.Type {
	case TypeSystem:
		needsPlayer, ok := SystemActions[e.Action]
		if !ok {
			return fmt.Errorf("acción de sistema desconocida: %s", e.Action)
		}
		if needsPlayer && e.PlayerID == "" {
			return errors.New("playerId requerido para " + e.Action)
		}
		return nil
	case TypeGameplay:
		return validateGameplay(e, reg)
	default:
		return fmt.Errorf("type inválido: %s", e.Type)
	}
}

func validateGameplay(e Event, reg games.Registry) error {
	cfg, ok := reg[e.GameID]
	if !ok {
		// Juego sin configuración: se acepta solo con el sobre común.
		if e.PlayerID == "" {
			return errors.New("playerId requerido en eventos GAMEPLAY")
		}
		return nil
	}
	required, ok := cfg.Actions[e.Action]
	if !ok {
		return fmt.Errorf("acción desconocida para %s: %s", e.GameID, e.Action)
	}
	if e.PlayerID == "" && !slices.Contains(cfg.NoPlayerActions, e.Action) {
		return errors.New("playerId requerido para " + e.Action)
	}
	var data map[string]json.RawMessage
	if len(e.Data) > 0 {
		if err := json.Unmarshal(e.Data, &data); err != nil {
			return errors.New("data no es un objeto JSON")
		}
	}
	var missing []string
	for _, f := range required {
		if _, ok := data[f]; !ok {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("data incompleta para %s: falta %s", e.Action, strings.Join(missing, ", "))
	}
	return nil
}
