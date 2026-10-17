package main

import (
	"math/rand/v2"
	"slices"
)

var stadiums = []string{"Saprissa", "Morera Soto", "Nacional", "Fello Meza"}

const matchSeconds = 90 * 60

// genFootball simula un partido 11 vs 11 como secuencia de posesiones:
// pases, tiros, goles, faltas y cambios. En la mitad de los partidos un
// equipo domina el primer tiempo y el otro el segundo (escenario de
// remontada), y a veces hay un jugador agresivo propenso a doble amarilla.
func genFootball(s *script, rng *rand.Rand) {
	all := pick(rng, pool("f", 30), 28)
	onField := map[string][]string{"A": slices.Clone(all[0:11]), "B": slices.Clone(all[14:25])}
	bench := map[string][]string{"A": all[11:14], "B": all[25:28]}
	team := map[string]string{}
	for _, p := range all[0:14] {
		team[p] = "A"
	}
	for _, p := range all[14:28] {
		team[p] = "B"
	}

	s.add(0, "MATCH_STARTED", "", map[string]any{"stadium": choose(rng, stadiums), "durationMin": 90})
	for i, p := range append(slices.Clone(onField["A"]), onField["B"]...) {
		s.add(sec(0.1*float64(i+1)), "PLAYER_JOINED", p, map[string]any{"team": team[p]})
	}

	// Multiplicadores de peligro por equipo y tiempo.
	mult := func(tm string, t float64) float64 { return 1 }
	if rng.Float64() < 0.5 {
		early := choose(rng, []string{"A", "B"})
		mult = func(tm string, t float64) float64 {
			first := t < matchSeconds/2
			if (tm == early) == first {
				return 4.5
			}
			return 0.3
		}
	}
	aggressive := ""
	if rng.Float64() < 0.6 {
		aggressive = choose(rng, all[0:11])
	}

	subTimes := []float64{between(rng, 55, 65) * 60, between(rng, 70, 80) * 60}
	nextSub := 0
	yellows := map[string]int{}
	score := map[string]int{"A": 0, "B": 0}

	pos := choose(rng, []string{"A", "B"})
	holder := choose(rng, onField[pos])
	lastPasser, lastPassT := "", -100.0

	for t := 2.0; t < matchSeconds; t += between(rng, 15, 35) {
		if nextSub < len(subTimes) && t > subTimes[nextSub] {
			for _, tm := range []string{"A", "B"} {
				out := choose(rng, onField[tm])
				in := bench[tm][nextSub]
				onField[tm] = replace(onField[tm], out, in)
				s.add(sec(t), "SUBSTITUTION", in, map[string]any{"outId": out, "inId": in, "team": tm})
				if holder == out {
					holder = in
				}
			}
			nextSub++
		}

		// Falta del equipo sin balón.
		def := other(pos)
		foulProb := 0.04
		if aggressive != "" && team[aggressive] == def && slices.Contains(onField[def], aggressive) {
			foulProb = 0.15
		}
		if rng.Float64() < foulProb {
			fouler := choose(rng, onField[def])
			yellowP := 0.2
			if fouler == aggressive {
				yellowP = 0.75
			}
			card := "none"
			switch r := rng.Float64(); {
			case r < 0.02:
				card = "red"
			case r < yellowP:
				card = "yellow"
				yellows[fouler]++
			}
			s.add(sec(t), "FOUL", fouler, map[string]any{"victimId": holder, "card": card})
			if card == "red" || yellows[fouler] == 2 {
				onField[def] = slices.DeleteFunc(onField[def], func(p string) bool { return p == fouler })
			}
			continue
		}

		// Pase.
		to := choose(rng, onField[pos])
		for to == holder {
			to = choose(rng, onField[pos])
		}
		// El equipo dominante conserva más el balón.
		passOK := 0.82
		if m := mult(pos, t); m > 1 {
			passOK = 0.9
		} else if m < 1 {
			passOK = 0.72
		}
		ok := rng.Float64() < passOK
		s.add(sec(t), "PASS", holder, map[string]any{
			"toPlayerId": to, "distance": int(between(rng, 5, 40)), "success": ok,
		})
		if !ok {
			pos = def
			holder = choose(rng, onField[pos])
			continue
		}
		lastPasser, lastPassT = holder, t
		holder = to

		// Tiro.
		if rng.Float64() < 0.07*mult(pos, t) {
			t += 2
			onTarget := rng.Float64() < 0.45
			s.add(sec(t), "SHOT", holder, map[string]any{"onTarget": onTarget, "distance": int(between(rng, 6, 30))})
			if onTarget && rng.Float64() < 0.4*min(mult(pos, t), 1.4) {
				t += 1
				data := map[string]any{"minute": int(t/60) + 1, "team": pos}
				if lastPasser != "" && t-lastPassT < 10 {
					data["assistBy"] = lastPasser
				}
				s.add(sec(t), "GOAL", holder, data)
				score[pos]++
			}
			pos = def
			holder = choose(rng, onField[pos])
			lastPasser = ""
		}
	}

	addDisconnect(s, rng, onField["A"], 300, matchSeconds-300)
	winner := "DRAW"
	if score["A"] > score["B"] {
		winner = "A"
	} else if score["B"] > score["A"] {
		winner = "B"
	}
	s.add(sec(matchSeconds+60), "MATCH_FINISHED", "", map[string]any{"score": score, "winner": winner})
}

func replace(list []string, out, in string) []string {
	res := slices.Clone(list)
	if i := slices.Index(res, out); i >= 0 {
		res[i] = in
	}
	return res
}
