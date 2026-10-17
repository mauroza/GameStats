// Package pipeline implementa el worker pool que valida los eventos
// recibidos y los entrega al manager de partidas.
package pipeline

import (
	"log"
	"sync"

	"github.com/mauroza/gamestats/ingest/internal/event"
	"github.com/mauroza/gamestats/ingest/internal/games"
	"github.com/mauroza/gamestats/ingest/internal/stats"
)

// Dispatcher recibe los eventos válidos (lo implementa matches.Manager).
type Dispatcher interface {
	Dispatch(event.Event)
}

// Pipeline reparte los eventos entrantes entre N workers.
type Pipeline struct {
	in      chan event.Event
	reg     games.Registry
	out     Dispatcher
	stats   *stats.Stats
	verbose bool
	wg      sync.WaitGroup

	seenMu sync.Mutex
	seen   map[string]bool
}

// New crea el pipeline y arranca los workers.
func New(workers int, reg games.Registry, out Dispatcher, st *stats.Stats, verbose bool) *Pipeline {
	p := &Pipeline{
		in:      make(chan event.Event, 4096),
		reg:     reg,
		out:     out,
		stats:   st,
		verbose: verbose,
		seen:    map[string]bool{},
	}
	for id := 1; id <= workers; id++ {
		p.wg.Add(1)
		go p.worker(id)
	}
	return p
}

// Submit encola un evento. Bloquea si la cola está llena (backpressure).
func (p *Pipeline) Submit(e event.Event) {
	p.in <- e
}

// Close deja de aceptar eventos y espera a que los workers terminen la cola.
func (p *Pipeline) Close() {
	close(p.in)
	p.wg.Wait()
}

func (p *Pipeline) worker(id int) {
	defer p.wg.Done()
	for e := range p.in {
		if err := event.Validate(e, p.reg); err != nil {
			p.Reject(e, err.Error())
			continue
		}
		if p.isDuplicate(e.EventID) {
			p.stats.Duplicates.Add(1)
			p.Reject(e, "eventId duplicado")
			continue
		}
		p.stats.Valid.Add(1)
		if p.verbose {
			log.Printf("[worker %d] %s/%s %s %s", id, e.GameID, e.MatchID, e.Action, e.PlayerID)
		}
		p.out.Dispatch(e)
	}
}

// isDuplicate marca el eventId como visto y dice si ya lo estaba.
func (p *Pipeline) isDuplicate(id string) bool {
	p.seenMu.Lock()
	defer p.seenMu.Unlock()
	if p.seen[id] {
		return true
	}
	p.seen[id] = true
	return false
}

// Reject registra un evento rechazado.
func (p *Pipeline) Reject(e event.Event, reason string) {
	if p.verbose {
		log.Printf("rechazado %s (%s/%s): %s", e.EventID, e.GameID, e.MatchID, reason)
	}
	p.stats.Reject(stats.Rejection{
		EventID: e.EventID, GameID: e.GameID, MatchID: e.MatchID, Reason: reason,
	})
}
