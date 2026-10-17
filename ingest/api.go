package main

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/mauroza/gamestats/ingest/internal/event"
	"github.com/mauroza/gamestats/ingest/internal/matches"
	"github.com/mauroza/gamestats/ingest/internal/pipeline"
	"github.com/mauroza/gamestats/ingest/internal/stats"
)

const maxBody = 10 << 20 // 10 MB

func newAPI(pipe *pipeline.Pipeline, mgr *matches.Manager, st *stats.Stats) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// POST /events acepta un evento o un arreglo de eventos.
	// La validación ocurre de forma asíncrona en el worker pool.
	mux.HandleFunc("POST /events", func(w http.ResponseWriter, r *http.Request) {
		body := http.MaxBytesReader(w, r.Body, maxBody)
		var raw json.RawMessage
		if err := json.NewDecoder(body).Decode(&raw); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido: " + err.Error()})
			return
		}
		var items []json.RawMessage
		if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '[' {
			if err := json.Unmarshal(trimmed, &items); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		} else {
			items = []json.RawMessage{raw}
		}

		queued, malformed := 0, 0
		for _, item := range items {
			st.Received.Add(1)
			var e event.Event
			if err := json.Unmarshal(item, &e); err != nil {
				malformed++
				st.Reject(stats.Rejection{Reason: "evento mal formado: " + err.Error(), Raw: item})
				continue
			}
			pipe.Submit(e)
			queued++
		}
		writeJSON(w, http.StatusAccepted, map[string]int{
			"received": len(items), "queued": queued, "malformed": malformed,
		})
	})

	mux.HandleFunc("GET /matches", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, mgr.Snapshots())
	})

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		s := st.Snapshot()
		s["activeMatches"] = int64(len(mgr.Snapshots()))
		writeJSON(w, http.StatusOK, s)
	})

	mux.HandleFunc("GET /rejected", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, st.Recent())
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
