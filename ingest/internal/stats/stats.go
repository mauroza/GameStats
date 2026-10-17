// Package stats lleva contadores del flujo de ingestión y el registro de
// eventos rechazados. Es seguro para uso concurrente.
package stats

import (
	"encoding/json"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

const recentLimit = 200

// Rejection describe un evento rechazado y el motivo.
type Rejection struct {
	At      time.Time       `json:"at"`
	EventID string          `json:"eventId,omitempty"`
	GameID  string          `json:"gameId,omitempty"`
	MatchID string          `json:"matchId,omitempty"`
	Reason  string          `json:"reason"`
	Raw     json.RawMessage `json:"raw,omitempty"`
}

// Stats agrupa los contadores del servicio.
type Stats struct {
	Received    atomic.Int64
	Valid       atomic.Int64
	Rejected    atomic.Int64
	Duplicates  atomic.Int64
	MatchesSent atomic.Int64
	SendErrors  atomic.Int64

	mu     sync.Mutex
	recent []Rejection
	out    io.Writer
}

// New crea Stats que escribe cada rechazo como una línea JSON en out (puede ser nil).
func New(out io.Writer) *Stats {
	return &Stats{out: out}
}

// Reject registra un rechazo.
func (s *Stats) Reject(r Rejection) {
	s.Rejected.Add(1)
	if r.At.IsZero() {
		r.At = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recent = append(s.recent, r)
	if len(s.recent) > recentLimit {
		s.recent = s.recent[len(s.recent)-recentLimit:]
	}
	if s.out != nil {
		line, _ := json.Marshal(r)
		if _, err := s.out.Write(append(line, '\n')); err != nil {
			log.Printf("no se pudo escribir rechazo: %v", err)
		}
	}
}

// Recent devuelve una copia de los últimos rechazos.
func (s *Stats) Recent() []Rejection {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Rejection(nil), s.recent...)
}

// Snapshot devuelve los contadores actuales.
func (s *Stats) Snapshot() map[string]int64 {
	return map[string]int64{
		"received":    s.Received.Load(),
		"valid":       s.Valid.Load(),
		"rejected":    s.Rejected.Load(),
		"duplicates":  s.Duplicates.Load(),
		"matchesSent": s.MatchesSent.Load(),
		"sendErrors":  s.SendErrors.Load(),
	}
}
