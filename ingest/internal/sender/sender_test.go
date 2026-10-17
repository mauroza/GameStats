package sender

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mauroza/gamestats/ingest/internal/matches"
	"github.com/mauroza/gamestats/ingest/internal/stats"
)

func cfg(url, outbox string) Config {
	return Config{URL: url, Outbox: outbox, Retries: 3, Backoff: time.Millisecond,
		Workers: 2, Timeout: time.Second, QueueLen: 10}
}

func TestSendsBatch(t *testing.T) {
	var got matches.Batch
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	st := stats.New(nil)
	s := New(cfg(srv.URL, t.TempDir()), st)
	s.Enqueue(matches.Batch{MatchID: "m1", GameID: "racing"})
	s.Close()

	if got.MatchID != "m1" || st.MatchesSent.Load() != 1 {
		t.Fatalf("recibido %+v, enviados %d", got, st.MatchesSent.Load())
	}
}

func TestRetriesThenOutbox(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	st := stats.New(nil)
	s := New(cfg(srv.URL, dir), st)
	s.Enqueue(matches.Batch{MatchID: "m1"})
	s.Close()

	if calls.Load() != 3 {
		t.Fatalf("intentos = %d, esperado 3", calls.Load())
	}
	if _, err := os.Stat(filepath.Join(dir, "m1.json")); err != nil {
		t.Fatalf("no se guardó en outbox: %v", err)
	}
	if st.SendErrors.Load() != 1 {
		t.Fatalf("sendErrors = %d", st.SendErrors.Load())
	}
}
