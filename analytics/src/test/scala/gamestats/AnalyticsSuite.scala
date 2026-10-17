package gamestats

import gamestats.analysis.*
import gamestats.model.*
import gamestats.store.Queries
import java.time.Duration
import upickle.default.*

class CodecSuite extends munit.FunSuite:
  test("parsea el lote que envía Go, con offsets y players null") {
    val json = """{
      "matchId":"r1","gameId":"racing","status":"FINISHED",
      "startedAt":"2026-10-16T14:00:00-06:00","endedAt":"2026-10-16T20:05:00Z","players":null,
      "events":[
        {"eventId":"e2","timestamp":"2026-10-16T20:01:00Z","gameId":"racing","matchId":"r1","playerId":"p1",
         "type":"GAMEPLAY","action":"LAP_COMPLETED","data":{"lap":1,"lapTimeMs":80000,"position":1}},
        {"eventId":"e1","timestamp":"2026-10-16T20:00:00Z","gameId":"racing","matchId":"r1",
         "type":"SYSTEM","action":"MATCH_STARTED"}
      ]}"""
    val b = Codec.parseBatch(json).fold(fail(_), identity)
    assertEquals(b.players, Nil)
    assertEquals(b.events.map(_.eventId), List("e1", "e2")) // ordenados por tiempo
    assertEquals(b.events(1).long("lapTimeMs"), Some(80000L))
    assertEquals(b.durationSec, 300L)
  }

  test("JSON inválido devuelve Left") {
    assert(Codec.parseBatch("{no es json").isLeft)
    assert(Codec.parseBatch("""{"matchId":"x"}""").isLeft)
  }

class PatternsSuite extends munit.FunSuite:
  private val kill = action("K")
  private def m = MatchBuilder("g")
    .at(0, "K", "a").at(3, "K", "a").at(4, "D", "a").at(5, "K", "a") // la muerte reinicia
    .at(20, "K", "b").at(22, "K", "b").at(25, "K", "b").at(28, "K", "b").build

  test("streaks detecta una vez por racha y respeta reinicios") {
    val found = Patterns.streaks(m.events, 3, Duration.ofSeconds(10))(
      key = e => Option.when(kill(e))(e.player),
      resets = e => Option.when(e.is("D"))(e.player))
    assertEquals(found.map(_._1), List("b"))
  }

  test("comebacks combina condición de desventaja y recuperación") {
    val series = Map("x" -> List(0, -2, -1, 0), "y" -> List(0, -3, -2), "z" -> List(1, 2))
    val found = Patterns.comebacks(series)(_.exists(_ <= -2), _ >= 0).map(_._1)
    assertEquals(found, List("x"))
  }

class GenericSuite extends munit.FunSuite:
  test("tiempo conectado descuenta desconexiones y detecta abandono") {
    val m = MatchBuilder("cualquiera")
      .at(0, Sys.MatchStarted)
      .at(0, Sys.PlayerJoined, "p1").at(0, Sys.PlayerJoined, "p2")
      .at(10, Sys.PlayerDisconnected, "p1").at(30, Sys.PlayerReconnected, "p1")
      .at(40, "X", "p2").at(50, Sys.PlayerDisconnected, "p2")
      .at(100, Sys.MatchFinished).build
    val r = Analytics.analyze(m)
    val p1 = read[GenericPlayer](r.genericPlayers("p1"))
    val p2 = read[GenericPlayer](r.genericPlayers("p2"))
    assertEquals(p1.connectedSec, 80L)
    assertEquals(p2.connectedSec, 50L)
    assert(!p1.abandoned && p2.abandoned)
    assertEquals(rules(r), List("ABANDONO"))
  }

  test("un juego sin analizador igual recibe estadísticas genéricas") {
    val r = Analytics.analyze(MatchBuilder("cards").at(0, Sys.MatchStarted).at(5, "CARD_PLAYED", "p1").build)
    assertEquals(r.generic("eventCount").num, 2.0)
    assertEquals(r.playerStats, Map.empty)
  }

class RacingSuite extends munit.FunSuite:
  private def lap(b: MatchBuilder, t: Double, p: String, lap: Int, ms: Int, pos: Int) =
    b.at(t, "LAP_COMPLETED", p, "lap" -> lap, "lapTimeMs" -> ms, "position" -> pos)

  private val race =
    val b = MatchBuilder("racing").at(0, Sys.MatchStarted, "", "laps" -> 2, "track" -> "Monza")
    List("a", "b", "c", "d").zipWithIndex.foreach((p, i) => b.at(0, Sys.PlayerJoined, p, "grid" -> (i + 1)))
    b.at(20, "CRASH", "d", "lap" -> 1, "severity" -> "minor")
    lap(b, 80, "a", 1, 80000, 1); lap(b, 81, "b", 1, 81000, 2); lap(b, 82, "c", 1, 82000, 3); lap(b, 90, "d", 1, 90000, 4)
    b.at(100, "PENALTY", "a", "reason" -> "track_limits", "secondsAdded" -> 5)
    lap(b, 164, "d", 2, 74000, 1); lap(b, 165, "a", 2, 85000, 2); lap(b, 166, "b", 2, 85000, 3); lap(b, 167, "c", 2, 85000, 4)
    b.at(170, Sys.MatchFinished).build

  test("estadísticas por jugador y por partida") {
    val r = Analytics.analyze(race)
    val d = read[RacingPlayer](r.playerStats("d"))
    assertEquals((d.gridPosition, d.finalPosition, d.positionsGained, d.bestLapMs), (4, 1, 3, 74000L))
    val s = read[RacingMatch](r.stats)
    assertEquals(s.winner, Some("d"))
    assertEquals(s.fastestLap.map(_.playerId), Some("d"))
    assertEquals(s.podium, List("d", "a", "b"))
  }

  test("tiempo perdido combina penalizaciones y choques") {
    val r = Analytics.analyze(race)
    val lost = r.combined("timeLost").arr.map(read[TimeLost](_)).map(t => t.playerId -> t).toMap
    assertEquals(lost("a").penaltiesMs, 5000L)
    assertEquals(lost("d").crashesMs, 16000L) // 90000 - 74000 (vuelta limpia)
  }

  test("detecta remontada desde el fondo en la vuelta 1") {
    val r = Analytics.analyze(race)
    assertEquals(r.patterns.filter(_.rule == "REMONTADA").flatMap(_.playerId), List("d"))
  }

class CombatSuite extends munit.FunSuite:
  private def elim(b: MatchBuilder, t: Double, k: String, v: String) =
    b.at(t, "DAMAGE_DEALT", k, "targetId" -> v, "weapon" -> "rifle", "amount" -> 100, "headshot" -> true)
     .at(t + 0.5, "PLAYER_ELIMINATED", k, "killerId" -> k, "victimId" -> v, "weapon" -> "rifle", "distance" -> 10)

  private val game =
    val b = MatchBuilder("combat").at(0, Sys.MatchStarted, "", "map" -> "Dust")
    List("a1", "a2", "a3").foreach(p => b.at(0, Sys.PlayerJoined, p, "team" -> "A"))
    List("b1", "b2", "b3").foreach(p => b.at(0, Sys.PlayerJoined, p, "team" -> "B"))
    // Ronda 1: a1 hace racha y ace (3 bajas en 4 s).
    elim(b, 10, "a1", "b1"); elim(b, 12, "a1", "b2"); elim(b, 14, "a1", "b3")
    b.at(16, "ROUND_END", "", "round" -> 1, "winnerTeam" -> "A")
    // Ronda 2: B queda con b3 solo contra 2 y gana (clutch).
    b.at(19, "ITEM_PICKED", "b3", "item" -> "armor")
    elim(b, 20, "a2", "b1"); elim(b, 22, "a3", "b2")
    elim(b, 30, "b3", "a1"); elim(b, 33, "b3", "a2"); elim(b, 36, "b3", "a3")
    b.at(38, "ROUND_END", "", "round" -> 2, "winnerTeam" -> "B")
    b.at(40, Sys.MatchFinished, "", "winner" -> "A").build

  test("K/D, daño y marcador por rondas") {
    val r = Analytics.analyze(game)
    val a1 = read[CombatPlayer](r.playerStats("a1"))
    assertEquals((a1.kills, a1.deaths, a1.kd, a1.damage, a1.headshotPct), (3, 1, 3.0, 300, 100.0))
    assertEquals(read[CombatMatch](r.stats).score, Map("A" -> 1, "B" -> 1))
  }

  test("detecta racha, ace y clutch") {
    val r = Analytics.analyze(game)
    val byRule = r.patterns.groupMap(_.rule)(_.playerId.getOrElse(""))
    assertEquals(byRule("RACHA").sorted, List("a1", "b3"))
    assertEquals(byRule("ACE").sorted, List("a1", "b3"))
    assertEquals(byRule("CLUTCH"), List("b3"))
  }

  test("impacto de objetos en eliminaciones") {
    val r = Analytics.analyze(game)
    val b3 = r.combined("impact").arr.map(read[CombatImpact](_)).find(_.playerId == "b3").get
    assertEquals((b3.killsPerRoundWithItem, b3.killsPerRoundWithoutItem), (3.0, 0.0))
  }

class FootballSuite extends munit.FunSuite:
  private def goal(b: MatchBuilder, t: Double, passer: String, scorer: String, team: String) =
    b.at(t, "PASS", passer, "toPlayerId" -> scorer, "distance" -> 20, "success" -> true)
     .at(t + 2, "SHOT", scorer, "onTarget" -> true, "distance" -> 12)
     .at(t + 3, "GOAL", scorer, "minute" -> (t / 60).toInt, "team" -> team)

  private val game =
    val b = MatchBuilder("football").at(0, Sys.MatchStarted, "", "stadium" -> "Saprissa")
    List("a1", "a2").foreach(p => b.at(0, Sys.PlayerJoined, p, "team" -> "A"))
    List("b1", "b2").foreach(p => b.at(0, Sys.PlayerJoined, p, "team" -> "B"))
    goal(b, 100, "a1", "a2", "A"); goal(b, 600, "a1", "a2", "A")       // 2-0
    b.at(900, "FOUL", "a1", "victimId" -> "b1", "card" -> "yellow")
    b.at(1500, "FOUL", "a1", "victimId" -> "b2", "card" -> "yellow")  // doble amarilla
    goal(b, 3000, "b2", "b1", "B"); goal(b, 4000, "b2", "b1", "B"); goal(b, 5000, "b2", "b1", "B") // 2-3
    b.at(5400, "PASS", "a1", "toPlayerId" -> "a2", "distance" -> 10, "success" -> false)
    b.at(5460, Sys.MatchFinished).build

  test("marcador, asistencias derivadas y efectividad de pase") {
    val r = Analytics.analyze(game)
    val s = read[FootballMatch](r.stats)
    assertEquals((s.score, s.winner), (Map("A" -> 2, "B" -> 3), "B"))
    val a1 = read[FootballPlayer](r.playerStats("a1"))
    assertEquals((a1.assists, a1.passes, a1.passAccuracy, a1.sentOff), (2, 3, 66.67, true))
    assertEquals(read[FootballPlayer](r.playerStats("b2")).keyPasses, 3)
  }

  test("detecta remontada, doble amarilla y hat-trick") {
    val r = Analytics.analyze(game)
    assertEquals(rules(r).sorted, List("DOBLE_AMARILLA", "HAT_TRICK", "REMONTADA"))
    assertEquals(r.patterns.find(_.rule == "REMONTADA").flatMap(_.team), Some("B"))
  }

class QueriesSuite extends munit.FunSuite:
  private def race(id: String, winner: String, other: String) =
    MatchBuilder("racing", id)
      .at(0, Sys.MatchStarted)
      .at(0, Sys.PlayerJoined, winner, "grid" -> 2).at(0, Sys.PlayerJoined, other, "grid" -> 1)
      .at(80, "LAP_COMPLETED", winner, "lap" -> 1, "lapTimeMs" -> 80000, "position" -> 1)
      .at(81, "LAP_COMPLETED", other, "lap" -> 1, "lapTimeMs" -> 81000, "position" -> 2)
      .at(90, Sys.MatchFinished).build

  private val results = List(race("r1", "p1", "p2"), race("r2", "p1", "p2"), race("r3", "p2", "p1"))
    .map(Analytics.analyze)

  test("perfil de jugador agrega varias partidas") {
    val p = Queries.playerProfile(results, "p1").get
    val totals = read[RacingTotals](p("games")("racing")("totals"))
    assertEquals((totals.races, totals.wins, totals.podiums), (3, 2, 3))
  }

  test("tabla de posiciones ordena por puntaje del juego") {
    val board = Queries.gameSummary(results, "racing")("leaderboard").arr.map(_("playerId").str)
    assertEquals(board.toList, List("p1", "p2"))
  }

  test("los resultados se serializan y se leen de vuelta") {
    val r = results.head
    assertEquals(read[MatchResult](write(r)), r)
  }
