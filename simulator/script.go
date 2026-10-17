package main

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"time"
)

// Event es el formato de salida del simulador. Es independiente del módulo
// ingest a propósito: el simulador representa un sistema externo que solo
// conoce la especificación JSON. Timestamp es string para poder generar
// fechas inválidas.
type Event struct {
	EventID     string         `json:"eventId,omitempty"`
	Timestamp   string         `json:"timestamp"`
	GameID      string         `json:"gameId"`
	GameVersion string         `json:"gameVersion"`
	MatchID     string         `json:"matchId,omitempty"`
	PlayerID    string         `json:"playerId,omitempty"`
	Type        string         `json:"type"`
	Action      string         `json:"action"`
	Data        map[string]any `json:"data,omitempty"`

	at time.Time
}

var systemActions = map[string]bool{
	"MATCH_STARTED": true, "MATCH_FINISHED": true,
	"PLAYER_JOINED": true, "PLAYER_LEFT": true,
	"PLAYER_DISCONNECTED": true, "PLAYER_RECONNECTED": true,
}

// script acumula los eventos de una partida con su tiempo de juego.
// Los eventos se generan primero y luego se "reproducen" con un ticker.
type script struct {
	game, match string
	start       time.Time
	events      []Event
}

func newScript(game, match string, start time.Time) *script {
	return &script{game: game, match: match, start: start}
}

// add registra un evento ocurrido off después del inicio de la partida.
func (s *script) add(off time.Duration, action, player string, data map[string]any) {
	typ := "GAMEPLAY"
	if systemActions[action] {
		typ = "SYSTEM"
	}
	at := s.start.Add(off)
	s.events = append(s.events, Event{
		Timestamp:   at.UTC().Format(time.RFC3339Nano),
		GameID:      s.game,
		GameVersion: "1.0",
		MatchID:     s.match,
		PlayerID:    player,
		Type:        typ,
		Action:      action,
		Data:        data,
		at:          at,
	})
}

// finish ordena los eventos por tiempo y asigna los eventId.
func (s *script) finish() []Event {
	sort.SliceStable(s.events, func(i, j int) bool { return s.events[i].at.Before(s.events[j].at) })
	for i := range s.events {
		s.events[i].EventID = fmt.Sprintf("%s-%04d", s.match, i+1)
	}
	return s.events
}

// Utilidades para los generadores.

func sec(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

func ms(f float64) int64 { return int64(f * 1000) }

func pool(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%02d", prefix, i+1)
	}
	return out
}

// pick elige n elementos distintos al azar.
func pick(rng *rand.Rand, from []string, n int) []string {
	out := make([]string, n)
	for i, j := range rng.Perm(len(from))[:n] {
		out[i] = from[j]
	}
	return out
}

func choose[T any](rng *rand.Rand, from []T) T { return from[rng.IntN(len(from))] }

func between(rng *rand.Rand, lo, hi float64) float64 { return lo + rng.Float64()*(hi-lo) }

// addDisconnect agrega, con cierta probabilidad, una desconexión de un
// jugador entre from y to segundos; a veces sin reconexión.
func addDisconnect(s *script, rng *rand.Rand, players []string, from, to float64) {
	if rng.Float64() > 0.35 {
		return
	}
	p := choose(rng, players)
	t := between(rng, from, to)
	s.add(sec(t), "PLAYER_DISCONNECTED", p, map[string]any{"reason": choose(rng, []string{"timeout", "network", "client_crash"})})
	if rng.Float64() < 0.75 {
		s.add(sec(t+between(rng, 5, 30)), "PLAYER_RECONNECTED", p, nil)
	}
}
