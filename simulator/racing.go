package main

import (
	"math/rand/v2"
	"sort"
)

var tracks = []string{"Monza", "Interlagos", "Suzuka", "La Sabana", "Spa"}

// genRacing simula una carrera vuelta por vuelta. Las posiciones salen del
// tiempo acumulado de cada piloto. Con probabilidad 0.6 un piloto rápido sale
// del fondo de la parrilla y choca en la vuelta 1: escenario de remontada.
func genRacing(s *script, rng *rand.Rand) {
	players := pick(rng, pool("r", 12), 6+rng.IntN(3))
	laps := 5 + rng.IntN(3)

	s.add(0, "MATCH_STARTED", "", map[string]any{"laps": laps, "track": choose(rng, tracks)})

	base := map[string]float64{} // ritmo base por vuelta (s)
	cum := map[string]float64{}  // tiempo acumulado (s)
	pos := map[string]int{}
	pitLap := map[string]int{}
	for i, p := range players {
		s.add(sec(0.1*float64(i+1)), "PLAYER_JOINED", p, map[string]any{"grid": i + 1})
		base[p] = between(rng, 79, 81.5)
		cum[p] = 1.5 * float64(i) // salida escalonada según la parrilla
		pos[p] = i + 1
		if rng.Float64() < 0.5 {
			pitLap[p] = 2 + rng.IntN(laps-2)
		}
	}

	comeback := ""
	if rng.Float64() < 0.6 {
		comeback = players[len(players)-1-rng.IntN(2)]
		base[comeback] = 76.5
	}

	for lap := 1; lap <= laps; lap++ {
		lapTime := map[string]float64{}
		for _, p := range players {
			t := base[p] + rng.NormFloat64()*1.2
			lapStart := cum[p]
			switch {
			case p == comeback && lap == 1:
				t += 9
				s.add(sec(lapStart+20), "CRASH", p, map[string]any{"lap": lap, "severity": "minor"})
			case rng.Float64() < 0.03:
				t += 12
				s.add(sec(lapStart+between(rng, 10, 60)), "CRASH", p, map[string]any{"lap": lap, "severity": "major"})
			}
			if pitLap[p] == lap {
				d := between(rng, 18, 24)
				t += d
				s.add(sec(lapStart+t-d), "PIT_STOP", p, map[string]any{"lap": lap, "durationMs": ms(d)})
			}
			if rng.Float64() < 0.08 {
				t += 5
				s.add(sec(lapStart+between(rng, 10, 60)), "PENALTY", p, map[string]any{
					"reason":       choose(rng, []string{"track_limits", "unsafe_release", "collision"}),
					"secondsAdded": 5,
				})
			}
			cum[p] += t
			lapTime[p] = t
		}

		order := sortedBy(players, cum)
		for i, p := range order {
			newPos := i + 1
			s.add(sec(cum[p]), "LAP_COMPLETED", p, map[string]any{
				"lap": lap, "lapTimeMs": ms(lapTime[p]), "position": newPos,
			})
			if newPos != pos[p] {
				s.add(sec(cum[p]+0.001), "POSITION_CHANGED", p, map[string]any{
					"from": pos[p], "to": newPos, "lap": lap,
				})
				pos[p] = newPos
			}
		}
	}

	order := sortedBy(players, cum)
	last := cum[order[len(order)-1]]
	addDisconnect(s, rng, players, 30, last-30)

	totals := map[string]int64{}
	for _, p := range players {
		totals[p] = ms(cum[p])
	}
	s.add(sec(last+2), "MATCH_FINISHED", "", map[string]any{"ranking": order, "totalTimesMs": totals})
}

// sortedBy ordena jugadores por valor ascendente.
func sortedBy(players []string, v map[string]float64) []string {
	out := append([]string(nil), players...)
	sort.SliceStable(out, func(i, j int) bool { return v[out[i]] < v[out[j]] })
	return out
}
