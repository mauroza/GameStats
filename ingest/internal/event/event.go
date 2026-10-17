// Package event define el sobre común de eventos de GameStats.
package event

import (
	"encoding/json"
	"time"
)

// Tipos generales de evento.
const (
	TypeSystem   = "SYSTEM"
	TypeGameplay = "GAMEPLAY"
)

// Acciones de sistema, comunes a todos los juegos.
const (
	MatchStarted       = "MATCH_STARTED"
	MatchFinished      = "MATCH_FINISHED"
	PlayerJoined       = "PLAYER_JOINED"
	PlayerLeft         = "PLAYER_LEFT"
	PlayerDisconnected = "PLAYER_DISCONNECTED"
	PlayerReconnected  = "PLAYER_RECONNECTED"
)

// SystemActions indica si la acción de sistema requiere playerId.
var SystemActions = map[string]bool{
	MatchStarted:       false,
	MatchFinished:      false,
	PlayerJoined:       true,
	PlayerLeft:         true,
	PlayerDisconnected: true,
	PlayerReconnected:  true,
}

// Event es el sobre común. Data se conserva sin interpretar: su contenido
// depende del juego y lo analiza el módulo Scala.
type Event struct {
	EventID     string          `json:"eventId"`
	Timestamp   time.Time       `json:"timestamp"`
	GameID      string          `json:"gameId"`
	GameVersion string          `json:"gameVersion,omitempty"`
	MatchID     string          `json:"matchId"`
	PlayerID    string          `json:"playerId,omitempty"`
	Type        string          `json:"type"`
	Action      string          `json:"action"`
	Data        json.RawMessage `json:"data,omitempty"`
}
