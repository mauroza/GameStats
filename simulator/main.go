// Simulador de GameStats: representa el sistema externo de juegos.
//
// Cada proceso simula un juego. Cada partida corre en su propia goroutine y
// envía sus eventos uno por uno a ingest al ritmo de un time.Ticker, como si
// el juego estuviera ocurriendo en vivo. Para la demo de concurrencia se
// abren tres procesos a la vez (uno por juego).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var generators = map[string]func(*script, *rand.Rand){
	"racing":   genRacing,
	"combat":   genCombat,
	"football": genFootball,
}

type config struct {
	game     string
	matches  int
	interval time.Duration
	target   string
	seed     uint64
	invalid  float64
	run      string
	dryRun   bool
}

type counters struct {
	sent, failed, invalid atomic.Int64
}

func main() {
	var c config
	flag.StringVar(&c.game, "game", "", "juego a simular: racing | combat | football")
	flag.IntVar(&c.matches, "matches", 3, "partidas concurrentes")
	flag.DurationVar(&c.interval, "interval", 300*time.Millisecond, "tiempo entre eventos de una partida")
	flag.StringVar(&c.target, "target", envOr("TARGET", "http://localhost:8080/events"), "endpoint de ingest")
	flag.Uint64Var(&c.seed, "seed", 0, "semilla (0 = aleatoria)")
	flag.Float64Var(&c.invalid, "invalid", 0.05, "probabilidad de inyectar un evento inválido")
	flag.StringVar(&c.run, "run", "", "id de ejecución para los matchId (por defecto según la hora)")
	flag.BoolVar(&c.dryRun, "dry-run", false, "imprimir eventos en stdout (JSON lines) sin enviarlos")
	flag.Parse()

	if c.game == "" {
		c.game = os.Getenv("GAME")
	}
	gen, ok := generators[c.game]
	if !ok {
		log.Fatalf("juego desconocido %q (racing | combat | football)", c.game)
	}
	if c.seed == 0 {
		c.seed = uint64(time.Now().UnixNano())
	}
	if c.run == "" {
		c.run = strconv.FormatInt(time.Now().Unix()%1_000_000, 36)
	}
	if c.dryRun {
		log.SetOutput(os.Stderr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("simulador %s: %d partidas, intervalo %s, semilla %d -> %s",
		c.game, c.matches, c.interval, c.seed, c.target)

	out := newOutput(c)
	var cnt counters
	var wg sync.WaitGroup
	for i := range c.matches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runMatch(ctx, c, gen, i, out, &cnt)
		}()
	}
	wg.Wait()
	log.Printf("simulador %s terminado: %d enviados, %d fallidos, %d inválidos inyectados",
		c.game, cnt.sent.Load(), cnt.failed.Load(), cnt.invalid.Load())
}

// runMatch genera una partida y la reproduce con un ticker.
func runMatch(ctx context.Context, c config, gen func(*script, *rand.Rand), idx int, out *output, cnt *counters) {
	rng := rand.New(rand.NewPCG(c.seed, uint64(idx)))
	matchID := fmt.Sprintf("%s-%s-%02d", c.game, c.run, idx+1)

	s := newScript(c.game, matchID, time.Now())
	gen(s, rng)
	events := s.finish()

	// Arranques escalonados para que las partidas se entrelacen.
	if !sleep(ctx, time.Duration(rng.Int64N(int64(c.interval)*10+1))) {
		return
	}
	log.Printf("[%s] inicia: %d eventos", matchID, len(events))

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for i, e := range events {
		select {
		case <-ctx.Done():
			log.Printf("[%s] interrumpida en %d/%d", matchID, i, len(events))
			return
		case <-ticker.C:
		}
		out.send(ctx, e, cnt)
		if rng.Float64() < c.invalid {
			bad, why := corrupt(e, rng)
			cnt.invalid.Add(1)
			log.Printf("[%s] inyectando inválido (%s)", matchID, why)
			out.send(ctx, bad, cnt)
		}
		if (i+1)%50 == 0 {
			log.Printf("[%s] %d/%d eventos", matchID, i+1, len(events))
		}
	}

	if rng.Float64() < c.invalid*4 {
		last := events[len(events)-1]
		cnt.invalid.Add(1)
		log.Printf("[%s] inyectando evento tardío", matchID)
		out.send(ctx, lateEvent(last, last.at), cnt)
	}
	log.Printf("[%s] terminada", matchID)
}

// output envía eventos por HTTP o los imprime (dry-run).
type output struct {
	cfg    config
	client *http.Client
	mu     sync.Mutex
	enc    *json.Encoder
}

func newOutput(c config) *output {
	return &output{cfg: c, client: &http.Client{Timeout: 5 * time.Second}, enc: json.NewEncoder(os.Stdout)}
}

func (o *output) send(ctx context.Context, e Event, cnt *counters) {
	if o.cfg.dryRun {
		o.mu.Lock()
		o.enc.Encode(e)
		o.mu.Unlock()
		cnt.sent.Add(1)
		return
	}
	body, _ := json.Marshal(e)
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = o.post(ctx, body); err == nil {
			cnt.sent.Add(1)
			return
		}
		if !sleep(ctx, 500*time.Millisecond) {
			break
		}
	}
	cnt.failed.Add(1)
	log.Printf("no se pudo enviar %s: %v", e.EventID, err)
}

func (o *output) post(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("respuesta %s", resp.Status)
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
