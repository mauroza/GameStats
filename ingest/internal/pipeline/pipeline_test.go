package pipeline

import (
	"sync"
	"testing"
	"time"

	"github.com/mauroza/gamestats/ingest/internal/event"
	"github.com/mauroza/gamestats/ingest/internal/games"
	"github.com/mauroza/gamestats/ingest/internal/stats"
)

type recorder struct {
	mu  sync.Mutex
	ids []string
}

func (r *recorder) Dispatch(e event.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, e.EventID)
}

func TestPipelineValidatesAndDeduplicates(t *testing.T) {
	st := stats.New(nil)
	rec := &recorder{}
	p := New(4, games.Registry{}, rec, st, false)

	ok := event.Event{
		EventID: "e1", Timestamp: time.Now(), GameID: "g", MatchID: "m",
		Type: event.TypeSystem, Action: event.MatchStarted,
	}
	p.Submit(ok)
	p.Submit(ok) // duplicado
	bad := ok
	bad.EventID, bad.MatchID = "e2", ""
	p.Submit(bad) // inválido
	p.Close()

	if len(rec.ids) != 1 {
		t.Fatalf("despachados = %v, esperado 1", rec.ids)
	}
	if st.Valid.Load() != 1 || st.Rejected.Load() != 2 || st.Duplicates.Load() != 1 {
		t.Fatalf("stats = %v", st.Snapshot())
	}
}
