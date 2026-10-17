package main

import (
	"math/rand/v2"
	"slices"
)

var (
	arenas  = []string{"Dust", "Harbor", "Volcano", "Citadel"}
	weapons = []string{"rifle", "smg", "sniper", "pistol", "shotgun"}
	items   = []string{"armor", "medkit", "grenade", "scope"}
)

const roundsToWin = 4

// genCombat simula un shooter 5 vs 5 por rondas. Cada enfrentamiento produce
// impactos (DAMAGE_DEALT) y una eliminación. Hay un jugador "estrella" con
// más habilidad (genera rachas) y rondas marcadas como "clutch", donde el
// último sobreviviente de un equipo gana todos sus duelos.
func genCombat(s *script, rng *rand.Rand) {
	players := pick(rng, pool("c", 14), 10)
	team := map[string]string{}
	skill := map[string]float64{}
	for i, p := range players {
		team[p] = "A"
		if i >= 5 {
			team[p] = "B"
		}
		skill[p] = between(rng, 0.6, 1.4)
	}
	skill[choose(rng, players)] = 2.5 // estrella

	s.add(0, "MATCH_STARTED", "", map[string]any{
		"map": choose(rng, arenas), "mode": "team_elimination", "roundsToWin": roundsToWin,
	})
	for i, p := range players {
		s.add(sec(0.2*float64(i+1)), "PLAYER_JOINED", p, map[string]any{"team": team[p]})
	}

	t := 5.0
	score := map[string]int{"A": 0, "B": 0}
	for round := 1; score["A"] < roundsToWin && score["B"] < roundsToWin; round++ {
		alive := map[string][]string{"A": nil, "B": nil}
		for _, p := range players {
			alive[team[p]] = append(alive[team[p]], p)
		}
		for _, p := range pick(rng, players, 3) {
			s.add(sec(t+between(rng, 0.5, 3)), "ITEM_PICKED", p, map[string]any{"item": choose(rng, items)})
		}
		clutch := rng.Float64() < 0.3
		t += 4

		winner := ""
		for winner == "" {
			t += between(rng, 1, 6)

			if rng.Float64() < 0.04 {
				capTeam := "A"
				if len(alive["B"]) > len(alive["A"]) {
					capTeam = "B"
				}
				p := choose(rng, alive[capTeam])
				s.add(sec(t), "OBJECTIVE_CAPTURED", p, map[string]any{"objectiveId": "bomb_site", "team": capTeam})
				winner = capTeam
				break
			}

			atkTeam := duelWinner(rng, alive, skill, clutch)
			defTeam := other(atkTeam)
			attacker := weighted(rng, alive[atkTeam], skill)
			victim := choose(rng, alive[defTeam])
			weapon := choose(rng, weapons)

			remaining := 100.0
			for hit := 0; remaining > 0; hit++ {
				amount := between(rng, 20, 60)
				if amount >= remaining || hit == 3 {
					amount = remaining
				}
				remaining -= amount
				s.add(sec(t+0.3*float64(hit)), "DAMAGE_DEALT", attacker, map[string]any{
					"targetId": victim, "weapon": weapon, "amount": int(amount),
					"headshot": rng.Float64() < 0.2*skill[attacker],
				})
			}
			t += 1.5
			s.add(sec(t), "PLAYER_ELIMINATED", attacker, map[string]any{
				"killerId": attacker, "victimId": victim, "weapon": weapon,
				"distance": int(between(rng, 3, 80)),
			})
			alive[defTeam] = slices.DeleteFunc(alive[defTeam], func(p string) bool { return p == victim })
			if len(alive[defTeam]) == 0 {
				winner = atkTeam
			}
		}

		score[winner]++
		t += 1
		s.add(sec(t), "ROUND_END", "", map[string]any{
			"round": round, "winnerTeam": winner,
			"survivors": map[string]int{"A": len(alive["A"]), "B": len(alive["B"])},
		})
		t += 8
	}

	addDisconnect(s, rng, players, 20, t-20)
	win := "A"
	if score["B"] > score["A"] {
		win = "B"
	}
	s.add(sec(t+1), "MATCH_FINISHED", "", map[string]any{"score": score, "winner": win})
}

// duelWinner decide qué equipo gana el próximo duelo según la habilidad de
// los vivos. En rondas clutch, el último sobreviviente gana siempre.
func duelWinner(rng *rand.Rand, alive map[string][]string, skill map[string]float64, clutch bool) string {
	if clutch {
		if len(alive["A"]) == 1 && len(alive["B"]) > 1 {
			return "A"
		}
		if len(alive["B"]) == 1 && len(alive["A"]) > 1 {
			return "B"
		}
	}
	a, b := 0.0, 0.0
	for _, p := range alive["A"] {
		a += skill[p]
	}
	for _, p := range alive["B"] {
		b += skill[p]
	}
	if rng.Float64()*(a+b) < a {
		return "A"
	}
	return "B"
}

func weighted(rng *rand.Rand, players []string, w map[string]float64) string {
	total := 0.0
	for _, p := range players {
		total += w[p]
	}
	r := rng.Float64() * total
	for _, p := range players {
		r -= w[p]
		if r <= 0 {
			return p
		}
	}
	return players[len(players)-1]
}

func other(team string) string {
	if team == "A" {
		return "B"
	}
	return "A"
}
