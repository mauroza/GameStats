// Package sender entrega las partidas cerradas al módulo de análisis
// (Scala) por HTTP. Si el envío falla tras varios reintentos, la partida se
// guarda en un directorio "outbox" y se reintenta al arrancar el servicio.
package sender

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mauroza/gamestats/ingest/internal/matches"
	"github.com/mauroza/gamestats/ingest/internal/stats"
)

// Config del sender.
type Config struct {
	URL      string        // endpoint de Scala, ej. http://analytics:9000/matches
	Outbox   string        // directorio para lotes no entregados
	Retries  int           // intentos por lote
	Backoff  time.Duration // espera inicial entre intentos (se duplica)
	Workers  int           // goroutines enviando en paralelo
	Timeout  time.Duration // timeout por request
	QueueLen int
}

// Sender envía lotes de forma asíncrona.
type Sender struct {
	cfg    Config
	client *http.Client
	stats  *stats.Stats
	queue  chan matches.Batch
	wg     sync.WaitGroup
}

// New crea el sender y arranca sus workers.
func New(cfg Config, st *stats.Stats) *Sender {
	if cfg.Retries < 1 {
		cfg.Retries = 1
	}
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	s := &Sender{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
		stats:  st,
		queue:  make(chan matches.Batch, cfg.QueueLen),
	}
	for range cfg.Workers {
		s.wg.Add(1)
		go s.worker()
	}
	return s
}

// Enqueue agrega un lote a la cola de envío.
func (s *Sender) Enqueue(b matches.Batch) {
	s.queue <- b
}

// Close espera a que se envíe (o se guarde) todo lo encolado.
func (s *Sender) Close() {
	close(s.queue)
	s.wg.Wait()
}

// ResendOutbox vuelve a encolar los lotes pendientes guardados en disco.
func (s *Sender) ResendOutbox() {
	files, _ := filepath.Glob(filepath.Join(s.cfg.Outbox, "*.json"))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var b matches.Batch
		if err := json.Unmarshal(raw, &b); err != nil {
			log.Printf("outbox: %s inválido: %v", f, err)
			continue
		}
		os.Remove(f) // si vuelve a fallar se guarda de nuevo
		s.Enqueue(b)
	}
	if len(files) > 0 {
		log.Printf("outbox: %d partidas reencoladas", len(files))
	}
}

func (s *Sender) worker() {
	defer s.wg.Done()
	for b := range s.queue {
		if err := s.send(b); err != nil {
			s.stats.SendErrors.Add(1)
			log.Printf("envío fallido %s: %v (guardado en outbox)", b.MatchID, err)
			s.save(b)
			continue
		}
		s.stats.MatchesSent.Add(1)
		log.Printf("partida enviada a análisis: %s/%s", b.GameID, b.MatchID)
	}
}

func (s *Sender) send(b matches.Batch) error {
	if s.cfg.URL == "" {
		return fmt.Errorf("ANALYTICS_URL no configurado")
	}
	body, err := json.Marshal(b)
	if err != nil {
		return err
	}
	wait := s.cfg.Backoff
	for attempt := 1; ; attempt++ {
		err = s.post(body)
		if err == nil || attempt == s.cfg.Retries {
			return err
		}
		time.Sleep(wait)
		wait *= 2
	}
}

func (s *Sender) post(body []byte) error {
	resp, err := s.client.Post(s.cfg.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("respuesta %s", resp.Status)
	}
	return nil
}

func (s *Sender) save(b matches.Batch) {
	if err := os.MkdirAll(s.cfg.Outbox, 0o755); err != nil {
		log.Printf("outbox: %v", err)
		return
	}
	raw, _ := json.MarshalIndent(b, "", "  ")
	path := filepath.Join(s.cfg.Outbox, b.MatchID+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		log.Printf("outbox: %v", err)
	}
}
