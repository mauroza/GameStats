package gamestats.analysis

import gamestats.model.*
import gamestats.analysis.Num.*
import upickle.default.*

final case class FootballPlayer(
    team: String,
    won: Boolean,
    passes: Int,
    passesCompleted: Int,
    passAccuracy: Double,
    shots: Int,
    shotsOnTarget: Int,
    goals: Int,
    assists: Int,   // derivadas de PASS -> GOAL
    keyPasses: Int, // pases que terminaron en tiro
    fouls: Int,
    yellowCards: Int,
    sentOff: Boolean
) derives ReadWriter

final case class GoalInfo(minute: Int, playerId: String, team: String, assistBy: Option[String]) derives ReadWriter

final case class FootballMatch(
    stadium: String,
    score: Map[String, Int],
    winner: String,
    possessionPct: Map[String, Double],
    shots: Map[String, Int],
    yellowCards: Int,
    redCards: Int,
    goals: List[GoalInfo]
) derives ReadWriter

/** Estadística combinada: eficiencia de cada equipo cruzando PASS, SHOT y GOAL. */
final case class TeamEfficiency(
    team: String,
    shots: Int,
    shotsOnTarget: Int,
    goals: Int,
    onTargetPct: Double,
    conversionPct: Double,
    goalsFromAssistPct: Double
) derives ReadWriter

final case class FootballTotals(
    matches: Int,
    wins: Int,
    goals: Int,
    assists: Int,
    keyPasses: Int,
    passAccuracy: Double,
    shots: Int,
    yellowCards: Int,
    sentOff: Int
) derives ReadWriter

object FootballAnalyzer extends GameAnalyzer:
  val gameId = "football"

  private val pass = action("PASS")
  private val shot = action("SHOT")
  private val goal = action("GOAL")
  private val foul = action("FOUL")
  private val successfulPass: Pred = pass and (_.bool("success").contains(true))
  private val assistWindowSec = 10.0

  /** Último pase exitoso hacia `e.player` dentro de la ventana previa a `e`. */
  private def feeder(events: List[Event], e: Event): Option[Event] =
    events
      .takeWhile(_.timestamp.isBefore(e.timestamp))
      .filter(successfulPass and (_.str("toPlayerId").contains(e.player)))
      .lastOption
      .filter(p => e.secondsSince(p.timestamp) <= assistWindowSec)

  def analyze(m: MatchBatch): Analysis =
    val ev = m.events
    val team: Map[String, String] =
      ev.filter(action(Sys.PlayerJoined) or action("SUBSTITUTION"))
        .flatMap(e => e.str("team").map(e.player -> _))
        .toMap
    val teams = team.values.toList.distinct.sorted

    val goals = ev.filter(goal)
    val score = teams.map(t => t -> goals.count(g => team.get(g.player).contains(t))).toMap
    val winner = score.toList.sortBy(-_._2) match
      case (a, ga) :: (_, gb) :: _ if ga > gb => a
      case (a, _) :: Nil                      => a
      case _                                   => "DRAW"

    val assists   = goals.flatMap(g => feeder(ev, g).map(_.player)).groupMapReduce(identity)(_ => 1)(_ + _)
    val keyPasses = ev.filter(shot).flatMap(s => feeder(ev, s).map(_.player)).groupMapReduce(identity)(_ => 1)(_ + _)
    val yellows   = ev.filter(foul and (_.str("card").contains("yellow"))).groupMapReduce(_.player)(_ => 1)(_ + _)
    val reds      = ev.filter(foul and (_.str("card").contains("red"))).map(_.player).toSet

    val players: Map[String, FootballPlayer] = team.keys.toList.sorted.map { p =>
      val mine      = ev.filter(byPlayer(p))
      val passes    = mine.count(pass)
      val completed = mine.count(successfulPass)
      val shots     = mine.filter(shot)
      p -> FootballPlayer(
        team = team(p),
        won = winner == team(p),
        passes = passes,
        passesCompleted = completed,
        passAccuracy = pct(completed, passes),
        shots = shots.size,
        shotsOnTarget = shots.count(_.bool("onTarget").contains(true)),
        goals = mine.count(goal),
        assists = assists.getOrElse(p, 0),
        keyPasses = keyPasses.getOrElse(p, 0),
        fouls = mine.count(foul),
        yellowCards = yellows.getOrElse(p, 0),
        sentOff = reds.contains(p) || yellows.getOrElse(p, 0) >= 2
      )
    }.toMap

    def byTeam(xs: List[Event]): Map[String, Int] =
      teams.map(t => t -> xs.count(e => team.get(e.player).contains(t))).toMap

    val completedByTeam = byTeam(ev.filter(successfulPass))
    val totalCompleted  = completedByTeam.values.sum
    val shotsByTeam     = byTeam(ev.filter(shot))

    val matchStats = FootballMatch(
      stadium = ev.find(_.is(Sys.MatchStarted)).flatMap(_.str("stadium")).getOrElse("desconocido"),
      score = score,
      winner = winner,
      possessionPct = completedByTeam.view.mapValues(pct(_, totalCompleted)).toMap,
      shots = shotsByTeam,
      yellowCards = yellows.values.sum,
      redCards = reds.size,
      goals = goals.map(g => GoalInfo(g.int("minute").getOrElse(0), g.player, team.getOrElse(g.player, "?"),
        feeder(ev, g).map(_.player)))
    )

    val efficiency = teams.map { t =>
      val ts        = ev.filter(shot).filter(e => team.get(e.player).contains(t))
      val tg        = goals.filter(g => team.get(g.player).contains(t))
      val onTarget  = ts.count(_.bool("onTarget").contains(true))
      TeamEfficiency(t, ts.size, onTarget, tg.size,
        onTargetPct = pct(onTarget, ts.size),
        conversionPct = pct(tg.size, ts.size),
        goalsFromAssistPct = pct(tg.count(g => feeder(ev, g).isDefined), tg.size))
    }

    Analysis(
      stats = writeJs(matchStats),
      playerStats = players.view.mapValues(writeJs(_)).toMap,
      combined = ujson.Obj("teamEfficiency" -> writeJs(efficiency)),
      patterns = comebacks(m, goals, team, teams) ++ doubleYellows(m) ++ hatTricks(m, goals)
    )

  /** REMONTADA: un equipo perdía por 2 o más y terminó empatando o ganando. */
  private def comebacks(m: MatchBatch, goals: List[Event], team: Map[String, String], teams: List[String]): List[Detection] =
    val series: Map[String, List[Int]] = teams.map { t =>
      t -> goals.scanLeft(0)((diff, g) => if team.get(g.player).contains(t) then diff + 1 else diff - 1)
    }.toMap
    Patterns
      .comebacks(series)(behind = _.exists(_ <= -2), recovered = _ >= 0)
      .map { case (t, xs) =>
        val result = if xs.last > 0 then "ganó" else "empató"
        Detection("REMONTADA", gameId, m.matchId, team = Some(t), at = goals.lastOption.map(_.timestamp),
          description = s"El equipo $t perdía por ${-xs.min} y $result")
      }

  /** EXPULSION: segunda amarilla del mismo jugador. */
  private def doubleYellows(m: MatchBatch): List[Detection] =
    m.events
      .filter(foul and (_.str("card").contains("yellow")))
      .groupBy(_.player)
      .collect { case (p, ys) if ys.size >= 2 =>
        val second = ys.sortBy(_.timestamp).apply(1)
        Detection("DOBLE_AMARILLA", gameId, m.matchId, Some(p), at = Some(second.timestamp),
          description = s"$p fue expulsado por doble amarilla")
      }
      .toList

  /** HAT_TRICK: 3 o más goles del mismo jugador. */
  private def hatTricks(m: MatchBatch, goals: List[Event]): List[Detection] =
    goals.groupBy(_.player).collect { case (p, gs) if gs.size >= 3 =>
      Detection("HAT_TRICK", gameId, m.matchId, Some(p), at = Some(gs.map(_.timestamp).max),
        description = s"$p anotó ${gs.size} goles")
    }.toList

  def playerTotals(perMatch: List[ujson.Value]): ujson.Value =
    val xs = perMatch.map(read[FootballPlayer](_))
    writeJs(FootballTotals(
      matches = xs.size,
      wins = xs.count(_.won),
      goals = xs.map(_.goals).sum,
      assists = xs.map(_.assists).sum,
      keyPasses = xs.map(_.keyPasses).sum,
      passAccuracy = pct(xs.map(_.passesCompleted).sum, xs.map(_.passes).sum),
      shots = xs.map(_.shots).sum,
      yellowCards = xs.map(_.yellowCards).sum,
      sentOff = xs.count(_.sentOff)
    ))

  def rankScore(totals: ujson.Value): Double =
    val t = read[FootballTotals](totals)
    t.goals * 3 + t.assists * 2 + t.keyPasses * 0.5 + t.wins
