// Package matches mantiene el estado de las partidas en curso.
//
// Diseño: un goroutine "router" es el único dueño del mapa de partidas y
// reparte cada evento al canal de su partida. Cada partida es un goroutine
// (actor) que acumula sus eventos y su estado. Cuando llega MATCH_FINISHED
// el actor espera un período de gracia (los workers procesan en paralelo,
// así que eventos anteriores pueden llegar un poco después), ordena los
// eventos por timestamp y entrega el lote completo al sink.
package matches

import (
	"log"
	"sort"
	"sync"
	"time"

	"github.com/mauroza/gamestats/ingest/internal/event"
)

// Status del ciclo de vida de una partida.
type Status string

const (
	StatusActive     Status = "ACTIVE"
	StatusFinishing  Status = "FINISHING"
	StatusFinished   Status = "FINISHED"
	StatusIncomplete Status = "INCOMPLETE"
)

// Batch es la partida completa que se entrega al módulo de análisis.
type Batch struct {
	MatchID     string        `json:"matchId"`
	GameID      string        `json:"gameId"`
	GameVersion string        `json:"gameVersion,omitempty"`
	Status      Status        `json:"status"`
	StartedAt   time.Time     `json:"startedAt"`
	EndedAt     time.Time     `json:"endedAt"`
	Players     []string      `json:"players"`
	Events      []event.Event `json:"events"`
}

// Snapshot es una vista del estado de una partida en curso.
type Snapshot struct {
	MatchID       string    `json:"matchId"`
	GameID        string    `json:"gameId"`
	Status        Status    `json:"status"`
	Players       []string  `json:"players"`
	ActivePlayers []string  `json:"activePlayers"`
	EventCount    int       `json:"eventCount"`
	StartedAt     time.Time `json:"startedAt"`
	LastEventAt   time.Time `json:"lastEventAt"`
}

// Config controla los tiempos del manager.
type Config struct {
	Grace       time.Duration // espera tras MATCH_FINISHED antes de cerrar
	IdleTimeout time.Duration // cierre de partidas sin actividad
}

// Sink recibe cada partida cerrada.
type Sink func(Batch)

// RejectFunc recibe los eventos que el manager no acepta.
type RejectFunc func(e event.Event, reason string)

// Manager coordina las partidas activas.
type Manager struct {
	cfg    Config
	sink   Sink
	reject RejectFunc

	events     chan event.Event
	retire     chan string
	routerDone chan struct{}
	wg         sync.WaitGroup

	// Solo el router accede a actors y closed.
	actors map[string]*actor
	closed map[string]bool

	// live permite consultar snapshots desde el API sin pasar por el router.
	liveMu sync.RWMutex
	live   map[string]*actor
}

// New crea y arranca el manager.
func New(cfg Config, sink Sink, reject RejectFunc) *Manager {
	m := &Manager{
		cfg:        cfg,
		sink:       sink,
		reject:     reject,
		events:     make(chan event.Event, 1024),
		retire:     make(chan string),
		routerDone: make(chan struct{}),
		actors:     map[string]*actor{},
		closed:     map[string]bool{},
		live:       map[string]*actor{},
	}
	go m.route()
	return m
}

// Dispatch entrega un evento ya validado a su partida.
func (m *Manager) Dispatch(e event.Event) {
	m.events <- e
}

// Close cierra todas las partidas abiertas y espera a que terminen.
// No se debe llamar Dispatch después de Close.
func (m *Manager) Close() {
	close(m.events)
	<-m.routerDone
	m.wg.Wait()
}

// Snapshots devuelve el estado de las partidas en curso.
func (m *Manager) Snapshots() []Snapshot {
	m.liveMu.RLock()
	actors := make([]*actor, 0, len(m.live))
	for _, a := range m.live {
		actors = append(actors, a)
	}
	m.liveMu.RUnlock()

	out := make([]Snapshot, 0, len(actors))
	for _, a := range actors {
		out = append(out, a.snapshot())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MatchID < out[j].MatchID })
	return out
}

func (m *Manager) route() {
	defer close(m.routerDone)
	for {
		select {
		case e, ok := <-m.events:
			if !ok {
				for _, a := range m.actors {
					close(a.in)
				}
				return
			}
			if m.closed[e.MatchID] {
				m.reject(e, "la partida ya fue cerrada")
				continue
			}
			a, exists := m.actors[e.MatchID]
			if !exists {
				a = newActor(e.MatchID)
				m.actors[e.MatchID] = a
				m.setLive(a, true)
				m.wg.Add(1)
				go m.runActor(a)
				log.Printf("partida abierta: %s/%s", e.GameID, e.MatchID)
			}
			a.in <- e
		case id := <-m.retire:
			a := m.actors[id]
			delete(m.actors, id)
			m.closed[id] = true
			m.setLive(a, false)
			close(a.in)
		}
	}
}

func (m *Manager) setLive(a *actor, on bool) {
	m.liveMu.Lock()
	defer m.liveMu.Unlock()
	if on {
		m.live[a.id] = a
	} else {
		delete(m.live, a.id)
	}
}

func (m *Manager) runActor(a *actor) {
	defer m.wg.Done()
	if m.active(a) {
		m.closing(a)
	}
}

// active procesa eventos hasta cerrar la partida. Devuelve false si el canal
// fue cerrado por apagado del servicio.
func (m *Manager) active(a *actor) bool {
	idle := time.NewTimer(m.cfg.IdleTimeout)
	defer idle.Stop()
	var grace <-chan time.Time

	for {
		select {
		case e, ok := <-a.in:
			if !ok {
				if a.status() == StatusFinishing {
					m.flush(a, StatusFinished)
				} else {
					m.flush(a, StatusIncomplete)
				}
				return false
			}
			if reason := a.apply(e); reason != "" {
				m.reject(e, reason)
				continue
			}
			if e.Action == event.MatchFinished {
				grace = time.After(m.cfg.Grace)
			}
			idle.Reset(m.cfg.IdleTimeout)
		case <-grace:
			m.flush(a, StatusFinished)
			return true
		case <-idle.C:
			m.flush(a, StatusIncomplete)
			return true
		}
	}
}

// closing pide al router que retire la partida y rechaza los eventos tardíos
// que lleguen mientras tanto.
func (m *Manager) closing(a *actor) {
	const late = "evento tardío: la partida ya fue cerrada"
	for {
		select {
		case m.retire <- a.id:
			for e := range a.in {
				m.reject(e, late)
			}
			return
		case e, ok := <-a.in:
			if !ok {
				return
			}
			m.reject(e, late)
		}
	}
}

func (m *Manager) flush(a *actor, st Status) {
	b := a.batch(st)
	log.Printf("partida cerrada: %s/%s %s (%d eventos, %d jugadores)",
		b.GameID, b.MatchID, b.Status, len(b.Events), len(b.Players))
	m.sink(b)
}
