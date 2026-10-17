package matches

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mauroza/gamestats/ingest/internal/event"
)

type collector struct {
	mu       sync.Mutex
	batches  []Batch
	rejected []string
	got      chan Batch
}

func newCollector() *collector { return &collector{got: make(chan Batch, 100)} }

func (c *collector) sink(b Batch) {
	c.mu.Lock()
	c.batches = append(c.batches, b)
	c.mu.Unlock()
	c.got <- b
}

func (c *collector) reject(e event.Event, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rejected = append(c.rejected, e.EventID+": "+reason)
}

func (c *collector) wait(t *testing.T) Batch {
	t.Helper()
	select {
	case b := <-c.got:
		return b
	case <-time.After(2 * time.Second):
		t.Fatal("timeout esperando lote")
		return Batch{}
	}
}

var t0 = time.Date(2026, 10, 16, 20, 0, 0, 0, time.UTC)

func ev(id, match, player, action string, sec int) event.Event {
	typ := event.TypeGameplay
	if _, ok := event.SystemActions[action]; ok {
		typ = event.TypeSystem
	}
	return event.Event{
		EventID: id, Timestamp: t0.Add(time.Duration(sec) * time.Second),
		GameID: "racing", MatchID: match, PlayerID: player, Type: typ, Action: action,
	}
}

func TestMatchFinishedSortsEvents(t *testing.T) {
	c := newCollector()
	m := New(Config{Grace: 20 * time.Millisecond, IdleTimeout: time.Minute}, c.sink, c.reject)
	defer m.Close()

	// Llegan desordenados, como pasaría con varios workers.
	m.Dispatch(ev("e1", "m1", "", event.MatchStarted, 0))
	m.Dispatch(ev("e3", "m1", "p1", "LAP_COMPLETED", 20))
	m.Dispatch(ev("e5", "m1", "", event.MatchFinished, 40))
	m.Dispatch(ev("e2", "m1", "p1", event.PlayerJoined, 1))
	m.Dispatch(ev("e4", "m1", "p2", "LAP_COMPLETED", 30))

	b := c.wait(t)
	if b.Status != StatusFinished || len(b.Events) != 5 {
		t.Fatalf("lote = %s con %d eventos, esperado FINISHED con 5", b.Status, len(b.Events))
	}
	for i, want := range []string{"e1", "e2", "e3", "e4", "e5"} {
		if b.Events[i].EventID != want {
			t.Fatalf("evento %d = %s, esperado %s", i, b.Events[i].EventID, want)
		}
	}
	if fmt.Sprint(b.Players) != "[p1 p2]" {
		t.Fatalf("jugadores = %v", b.Players)
	}
}

func TestLateEventsRejected(t *testing.T) {
	c := newCollector()
	m := New(Config{Grace: 20 * time.Millisecond, IdleTimeout: time.Minute}, c.sink, c.reject)

	m.Dispatch(ev("e1", "m1", "", event.MatchStarted, 0))
	m.Dispatch(ev("e2", "m1", "", event.MatchFinished, 10))
	m.Dispatch(ev("e3", "m1", "p1", "LAP_COMPLETED", 11)) // posterior al final
	c.wait(t)
	m.Dispatch(ev("e4", "m1", "p1", "LAP_COMPLETED", 5)) // partida ya cerrada
	m.Close()

	if len(c.rejected) != 2 {
		t.Fatalf("rechazados = %v, esperado 2", c.rejected)
	}
}

func TestIdleTimeoutMarksIncomplete(t *testing.T) {
	c := newCollector()
	m := New(Config{Grace: time.Second, IdleTimeout: 30 * time.Millisecond}, c.sink, c.reject)
	defer m.Close()

	m.Dispatch(ev("e1", "m1", "", event.MatchStarted, 0))
	if b := c.wait(t); b.Status != StatusIncomplete {
		t.Fatalf("status = %s, esperado INCOMPLETE", b.Status)
	}
}

func TestCloseFlushesOpenMatches(t *testing.T) {
	c := newCollector()
	m := New(Config{Grace: time.Minute, IdleTimeout: time.Minute}, c.sink, c.reject)
	m.Dispatch(ev("a1", "a", "", event.MatchStarted, 0))
	m.Dispatch(ev("b1", "b", "", event.MatchStarted, 0))
	m.Dispatch(ev("b2", "b", "", event.MatchFinished, 5))
	m.Close()

	status := map[string]Status{}
	for _, b := range c.batches {
		status[b.MatchID] = b.Status
	}
	if status["a"] != StatusIncomplete || status["b"] != StatusFinished {
		t.Fatalf("status = %v", status)
	}
}

func TestConcurrentMatches(t *testing.T) {
	const matches, perMatch = 20, 50
	c := newCollector()
	c.got = make(chan Batch, matches)
	m := New(Config{Grace: 20 * time.Millisecond, IdleTimeout: time.Minute}, c.sink, c.reject)

	var wg sync.WaitGroup
	for i := range matches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("m%d", i)
			m.Dispatch(ev(id+"-s", id, "", event.MatchStarted, 0))
			for j := range perMatch {
				m.Dispatch(ev(fmt.Sprintf("%s-%d", id, j), id, "p1", "LAP_COMPLETED", j+1))
			}
			m.Dispatch(ev(id+"-f", id, "", event.MatchFinished, perMatch+1))
		}()
	}
	wg.Wait()
	for range matches {
		if b := c.wait(t); len(b.Events) != perMatch+2 {
			t.Fatalf("%s tiene %d eventos", b.MatchID, len(b.Events))
		}
	}
	m.Close()
}
