// Servicio de ingestión de GameStats: recibe eventos, los valida, mantiene
// el estado de las partidas y entrega cada partida cerrada al módulo Scala.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mauroza/gamestats/ingest/internal/event"
	"github.com/mauroza/gamestats/ingest/internal/games"
	"github.com/mauroza/gamestats/ingest/internal/matches"
	"github.com/mauroza/gamestats/ingest/internal/pipeline"
	"github.com/mauroza/gamestats/ingest/internal/sender"
	"github.com/mauroza/gamestats/ingest/internal/stats"
)

func main() {
	addr := getenv("ADDR", ":8080")
	gamesDir := getenv("GAMES_DIR", "../games")
	dataDir := getenv("DATA_DIR", "data")
	analyticsURL := getenv("ANALYTICS_URL", "http://localhost:9000/matches")
	workers := getint("WORKERS", 4)
	verbose := getenv("VERBOSE", "false") == "true"

	reg, err := games.Load(gamesDir)
	if err != nil {
		log.Fatalf("cargando juegos: %v", err)
	}
	log.Printf("juegos registrados: %d", len(reg))

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	rejectedLog, err := os.OpenFile(filepath.Join(dataDir, "rejected.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	defer rejectedLog.Close()

	st := stats.New(rejectedLog)
	snd := sender.New(sender.Config{
		URL:      analyticsURL,
		Outbox:   filepath.Join(dataDir, "outbox"),
		Retries:  getint("SEND_RETRIES", 4),
		Backoff:  500 * time.Millisecond,
		Workers:  2,
		Timeout:  10 * time.Second,
		QueueLen: 256,
	}, st)
	snd.ResendOutbox()

	mgr := matches.New(matches.Config{
		Grace:       getdur("GRACE", 2*time.Second),
		IdleTimeout: getdur("IDLE_TIMEOUT", 60*time.Second),
	}, snd.Enqueue, func(e event.Event, reason string) {
		st.Reject(stats.Rejection{EventID: e.EventID, GameID: e.GameID, MatchID: e.MatchID, Reason: reason})
	})
	pipe := pipeline.New(workers, reg, mgr, st, verbose)

	srv := &http.Server{Addr: addr, Handler: newAPI(pipe, mgr, st)}
	go func() {
		log.Printf("ingest escuchando en %s (%d workers) -> %s", addr, workers, analyticsURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	// Apagado en orden: dejar de recibir, vaciar workers, cerrar partidas, enviar.
	log.Println("apagando...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
	pipe.Close()
	mgr.Close()
	snd.Close()
	log.Printf("listo: %v", st.Snapshot())
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getint(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}

func getdur(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}
