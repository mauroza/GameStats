package gamestats.analysis

import gamestats.model.*
import gamestats.analysis.Num.*
import upickle.default.*

final case class RacingPlayer(
    gridPosition: Int,
    finalPosition: Int,
    positionsGained: Int,
    laps: Int,
    bestLapMs: Long,
    avgLapMs: Long,
    consistencyMs: Long, // desviación estándar de las vueltas limpias
    pitStops: Int,
    penalties: Int,
    crashes: Int,
    totalTimeMs: Long
) derives ReadWriter

final case class FastestLap(playerId: String, lap: Int, lapTimeMs: Long) derives ReadWriter

final case class RacingMatch(
    track: String,
    laps: Int,
    winner: Option[String],
    podium: List[String],
    fastestLap: Option[FastestLap],
    overtakes: Int,
    pitStops: Int,
    penalties: Int,
    crashes: Int
) derives ReadWriter

/** Estadística combinada: cruza LAP_COMPLETED, PIT_STOP, PENALTY y CRASH. */
final case class TimeLost(
    playerId: String,
    penaltiesMs: Long,
    pitStopsMs: Long,
    crashesMs: Long,
    totalMs: Long,
    finalPosition: Int,
    positionWithoutIncidents: Int
) derives ReadWriter

final case class RacingTotals(
    races: Int,
    wins: Int,
    podiums: Int,
    bestLapMs: Option[Long],
    avgFinalPosition: Double,
    positionsGained: Int,
    pitStops: Int,
    penalties: Int
) derives ReadWriter

object RacingAnalyzer extends GameAnalyzer:
  val gameId = "racing"

  private val lapCompleted = action("LAP_COMPLETED")
  private val pitStop      = action("PIT_STOP")
  private val penalty      = action("PENALTY")
  private val crash        = action("CRASH")

  private def lapOf(e: Event): Int = e.int("lap").getOrElse(0)

  def analyze(m: MatchBatch): Analysis =
    val ev     = m.events
    val laps   = ev.filter(lapCompleted)
    val lapsBy = laps.groupBy(_.player).view.mapValues(_.sortBy(lapOf)).toMap
    val grid   = ev.filter(action(Sys.PlayerJoined)).flatMap(e => e.int("grid").map(e.player -> _)).toMap
    val drivers = (grid.keySet ++ lapsBy.keySet).toList.sorted

    val pits      = countBy(ev)(pitStop)
    val penalties = countBy(ev)(penalty)
    val crashes   = countBy(ev)(crash)

    // Vueltas con incidentes (pits o choques) por piloto: se excluyen de la consistencia.
    val incidentLaps: Map[String, Set[Int]] =
      ev.filter(pitStop or crash).groupMap(_.player)(lapOf).view.mapValues(_.toSet).toMap

    def lapTimes(p: String): List[Long] = lapsBy.getOrElse(p, Nil).flatMap(_.long("lapTimeMs"))
    def cleanLapTimes(p: String): List[Long] =
      lapsBy.getOrElse(p, Nil)
        .filterNot(l => incidentLaps.getOrElse(p, Set.empty).contains(lapOf(l)))
        .flatMap(_.long("lapTimeMs"))

    val players: Map[String, RacingPlayer] = drivers.map { p =>
      val times = lapTimes(p)
      val fin   = lapsBy.getOrElse(p, Nil).lastOption.flatMap(_.int("position")).getOrElse(0)
      val g     = grid.getOrElse(p, 0)
      p -> RacingPlayer(
        gridPosition = g,
        finalPosition = fin,
        positionsGained = if g > 0 && fin > 0 then g - fin else 0,
        laps = times.size,
        bestLapMs = times.minOption.getOrElse(0L),
        avgLapMs = mean(times.map(_.toDouble)).toLong,
        consistencyMs = stddev(cleanLapTimes(p).map(_.toDouble)).toLong,
        pitStops = pits.getOrElse(p, 0),
        penalties = penalties.getOrElse(p, 0),
        crashes = crashes.getOrElse(p, 0),
        totalTimeMs = times.sum
      )
    }.toMap

    val ranking = players.toList.filter(_._2.finalPosition > 0).sortBy(_._2.finalPosition).map(_._1)
    val start   = ev.find(_.is(Sys.MatchStarted))
    val fastest = laps
      .flatMap(l => l.long("lapTimeMs").map(t => FastestLap(l.player, lapOf(l), t)))
      .minByOption(_.lapTimeMs)

    val matchStats = RacingMatch(
      track = start.flatMap(_.str("track")).getOrElse("desconocida"),
      laps = start.flatMap(_.int("laps")).getOrElse(laps.map(lapOf).maxOption.getOrElse(0)),
      winner = ranking.headOption,
      podium = ranking.take(3),
      fastestLap = fastest,
      overtakes = ev.count(e => e.is("POSITION_CHANGED") && e.int("to").zip(e.int("from")).exists(_ < _)),
      pitStops = pits.values.sum,
      penalties = penalties.values.sum,
      crashes = crashes.values.sum
    )

    // --- Combinada: tiempo perdido por incidentes y posición "sin incidentes".
    val lost: List[TimeLost] = drivers.map { p =>
      val mine        = ev.filter(byPlayer(p))
      val penaltyMs   = mine.filter(penalty).flatMap(_.long("secondsAdded")).sum * 1000
      val pitMs       = mine.filter(pitStop).flatMap(_.long("durationMs")).sum
      val pitLaps     = mine.filter(pitStop).map(lapOf).toSet
      val crashLaps   = mine.filter(crash).map(lapOf).toSet -- pitLaps
      val reference   = mean(cleanLapTimes(p).map(_.toDouble))
      val crashMs = lapsBy.getOrElse(p, Nil)
        .filter(l => crashLaps.contains(lapOf(l)))
        .flatMap(_.long("lapTimeMs"))
        .map(t => (t - reference).toLong.max(0))
        .sum
      TimeLost(p, penaltyMs, pitMs, crashMs, penaltyMs + pitMs + crashMs,
        players(p).finalPosition, positionWithoutIncidents = 0)
    }
    val withoutIncidents = lost
      .sortBy(t => players(t.playerId).totalTimeMs - t.totalMs)
      .zipWithIndex
      .map { case (t, i) => t.playerId -> (i + 1) }
      .toMap
    val combined = lost
      .map(t => t.copy(positionWithoutIncidents = withoutIncidents(t.playerId)))
      .sortBy(-_.totalMs)

    // --- Patrones.
    val positions: Map[String, List[Int]] = lapsBy.view.mapValues(_.flatMap(_.int("position"))).toMap
    val n  = drivers.size
    val at = Some(m.finishedAt)

    val comebacks = Patterns
      .comebacks(positions)(behind = _.headOption.exists(_ >= n - 1), recovered = _ <= 3)
      .map { case (p, xs) =>
        Detection("REMONTADA", gameId, m.matchId, Some(p), at = at,
          description = s"$p pasó de P${xs.head} en la vuelta 1 a terminar P${xs.last}")
      }
    val wireToWire = positions.toList.collect {
      case (p, xs) if xs.size > 1 && xs.forall(_ == 1) =>
        Detection("PUNTA_A_PUNTA", gameId, m.matchId, Some(p), at = at,
          description = s"$p lideró todas las vueltas")
    }

    Analysis(
      stats = writeJs(matchStats),
      playerStats = players.view.mapValues(writeJs(_)).toMap,
      combined = ujson.Obj("timeLost" -> writeJs(combined)),
      patterns = comebacks ++ wireToWire
    )

  def playerTotals(perMatch: List[ujson.Value]): ujson.Value =
    val xs = perMatch.map(read[RacingPlayer](_))
    writeJs(RacingTotals(
      races = xs.size,
      wins = xs.count(_.finalPosition == 1),
      podiums = xs.count(x => x.finalPosition >= 1 && x.finalPosition <= 3),
      bestLapMs = xs.map(_.bestLapMs).filter(_ > 0).minOption,
      avgFinalPosition = r2(mean(xs.map(_.finalPosition.toDouble))),
      positionsGained = xs.map(_.positionsGained).sum,
      pitStops = xs.map(_.pitStops).sum,
      penalties = xs.map(_.penalties).sum
    ))

  def rankScore(totals: ujson.Value): Double =
    val t = read[RacingTotals](totals)
    t.wins * 100 + t.podiums * 10 - t.avgFinalPosition
