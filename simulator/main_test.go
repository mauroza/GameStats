package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

type gameConfig struct {
	Actions         map[string][]string `json:"actions"`
	NoPlayerActions []string            `json:"noPlayerActions"`
}

func loadConfig(t *testing.T, game string) gameConfig {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "games", game+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var c gameConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func generate(game string, seed uint64) []Event {
	s := newScript(game, fmt.Sprintf("%s-test-%d", game, seed), time.Date(2026, 10, 16, 20, 0, 0, 0, time.UTC))
	generators[game](s, rand.New(rand.NewPCG(seed, 0)))
	return s.finish()
}

// Las partidas generadas deben cumplir la especificación de games/*.json.
func TestGeneratedEventsMatchSpec(t *testing.T) {
	for game := range generators {
		cfg := loadConfig(t, game)
		for seed := uint64(1); seed <= 30; seed++ {
			events := generate(game, seed)
			if events[0].Action != "MATCH_STARTED" || events[len(events)-1].Action != "MATCH_FINISHED" {
				t.Fatalf("%s/%d: debe iniciar con MATCH_STARTED y terminar con MATCH_FINISHED", game, seed)
			}
			ids := map[string]bool{}
			for i, e := range events {
				if ids[e.EventID] {
					t.Fatalf("%s/%d: eventId repetido %s", game, seed, e.EventID)
				}
				ids[e.EventID] = true
				if i > 0 && e.at.Before(events[i-1].at) {
					t.Fatalf("%s/%d: eventos desordenados", game, seed)
				}
				if e.Type == "SYSTEM" {
					continue
				}
				required, ok := cfg.Actions[e.Action]
				if !ok {
					t.Fatalf("%s/%d: acción fuera de la especificación: %s", game, seed, e.Action)
				}
				for _, f := range required {
					if _, ok := e.Data[f]; !ok {
						t.Fatalf("%s/%d: %s sin campo %s", game, seed, e.Action, f)
					}
				}
				if e.PlayerID == "" && !slices.Contains(cfg.NoPlayerActions, e.Action) {
					t.Fatalf("%s/%d: %s sin playerId", game, seed, e.Action)
				}
			}
		}
	}
}

// Los escenarios especiales deben aparecer con frecuencia razonable para que
// las reglas de Scala tengan qué detectar en la demo.
func TestSpecialScenariosAppear(t *testing.T) {
	const seeds = 40
	var comebacks, streaks, doubleYellows, footballComebacks int
	for seed := uint64(1); seed <= seeds; seed++ {
		if racingComeback(generate("racing", seed)) {
			comebacks++
		}
		if killStreak(generate("combat", seed)) {
			streaks++
		}
		fb := generate("football", seed)
		if doubleYellow(fb) {
			doubleYellows++
		}
		if footballComeback(fb) {
			footballComebacks++
		}
	}
	t.Logf("de %d partidas: remontadas carreras=%d, rachas=%d, doble amarilla=%d, remontadas fútbol=%d",
		seeds, comebacks, streaks, doubleYellows, footballComebacks)
	if comebacks == 0 || streaks == 0 || doubleYellows == 0 || footballComebacks == 0 {
		t.Fatal("algún escenario especial nunca aparece")
	}
}

func num(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// Un piloto estaba en las dos últimas posiciones al cerrar la vuelta 1 y
// terminó en el top 3.
func racingComeback(events []Event) bool {
	n := 0
	lap1 := map[string]int{}
	final := map[string]int{}
	for _, e := range events {
		switch e.Action {
		case "PLAYER_JOINED":
			n++
		case "LAP_COMPLETED":
			p := num(e.Data["position"])
			if num(e.Data["lap"]) == 1 {
				lap1[e.PlayerID] = p
			}
			final[e.PlayerID] = p
		}
	}
	for p, first := range lap1 {
		if first >= n-1 && final[p] <= 3 {
			return true
		}
	}
	return false
}

// 3 eliminaciones del mismo jugador en menos de 10 s sin morir.
func killStreak(events []Event) bool {
	kills := map[string][]time.Time{}
	for _, e := range events {
		if e.Action != "PLAYER_ELIMINATED" {
			continue
		}
		delete(kills, e.Data["victimId"].(string))
		k := append(kills[e.PlayerID], e.at)
		if len(k) >= 3 && k[len(k)-1].Sub(k[len(k)-3]) < 10*time.Second {
			return true
		}
		kills[e.PlayerID] = k
	}
	return false
}

func doubleYellow(events []Event) bool {
	y := map[string]int{}
	for _, e := range events {
		if e.Action == "FOUL" && e.Data["card"] == "yellow" {
			y[e.PlayerID]++
			if y[e.PlayerID] == 2 {
				return true
			}
		}
	}
	return false
}

// Un equipo perdía por 2 o más y terminó empatando o ganando.
func footballComeback(events []Event) bool {
	score := map[string]int{}
	trailed := map[string]bool{}
	for _, e := range events {
		if e.Action != "GOAL" {
			continue
		}
		score[e.Data["team"].(string)]++
		if score["A"]-score["B"] >= 2 {
			trailed["B"] = true
		}
		if score["B"]-score["A"] >= 2 {
			trailed["A"] = true
		}
	}
	return (trailed["A"] && score["A"] >= score["B"]) || (trailed["B"] && score["B"] >= score["A"])
}
