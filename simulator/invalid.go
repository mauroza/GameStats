package main

import (
	"maps"
	"math/rand/v2"
	"slices"
	"time"
)

// corrupt crea una copia inválida de e para probar la validación de ingest.
// El evento original se envía igual; la copia es un evento extra.
func corrupt(e Event, rng *rand.Rand) (Event, string) {
	bad := e
	bad.EventID = e.EventID + "-x"
	bad.Data = maps.Clone(e.Data)

	switch rng.IntN(5) {
	case 0:
		bad.MatchID = ""
		return bad, "sin matchId"
	case 1:
		bad.Timestamp = "ayer en la tarde"
		return bad, "timestamp inválido"
	case 2:
		bad.Type, bad.Action = "GAMEPLAY", "TELEPORT"
		if bad.PlayerID == "" {
			bad.PlayerID = "x01"
		}
		return bad, "acción desconocida"
	case 3:
		if len(bad.Data) > 0 && bad.Type == "GAMEPLAY" {
			keys := slices.Sorted(maps.Keys(bad.Data))
			delete(bad.Data, keys[0])
			return bad, "data incompleta"
		}
		bad.Type = "BONUS"
		return bad, "type inválido"
	default:
		bad.EventID = e.EventID
		return bad, "duplicado"
	}
}

// lateEvent crea un evento con timestamp posterior al fin de la partida.
func lateEvent(finished Event, end time.Time) Event {
	late := finished
	late.EventID = finished.EventID + "-late"
	late.Type, late.Action = "SYSTEM", "PLAYER_LEFT"
	late.PlayerID = "late01"
	late.Data = nil
	late.Timestamp = end.Add(5 * time.Second).UTC().Format(time.RFC3339Nano)
	return late
}
