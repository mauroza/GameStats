package gamestats.analysis

import gamestats.model.*
import gamestats.analysis.Num.*
import java.time.Duration
import upickle.default.*

final case class CombatPlayer(
    team: String,
    won: Boolean,
    kills: Int,
    deaths: Int,
    kd: Double,
    damage: Int,
    hits: Int,
    avgDamagePerHit: Double,
    headshotPct: Double,
    itemsPicked: Int,
    objectives: Int
) derives ReadWriter

final case class CombatMatch(
    map: String,
    score: Map[String, Int],
    winner: Option[String],
    rounds: Int,
    totalKills: Int,
    mvp: Option[String],
    mostUsedWeapon: Option[String],
    killsByWeapon: Map[String, Int]
) derives ReadWriter

/** Estadística combinada: cruza ITEM_PICKED, DAMAGE_DEALT, PLAYER_ELIMINATED y ROUND_END. */
final case class CombatImpact(
    playerId: String,
    damagePerKill: Double,
    killsPerRoundWithItem: Double,
    killsPerRoundWithoutItem: Double
) derives ReadWriter

final case class CombatTotals(
    matches: Int,
    wins: Int,
    kills: Int,
    deaths: Int,
    kd: Double,
    damage: Int,
    headshotPct: Double,
    objectives: Int
) derives ReadWriter

object CombatAnalyzer extends GameAnalyzer:
  val gameId = "combat"

  private val elimination = action("PLAYER_ELIMINATED")
  private val damageDealt = action("DAMAGE_DEALT")
  private val roundEnd    = action("ROUND_END")

  def analyze(m: MatchBatch): Analysis =
    val ev = m.events
    val team: Map[String, String] =
      ev.filter(action(Sys.PlayerJoined)).flatMap(e => e.str("team").map(e.player -> _)).toMap
    val roster: Map[String, Set[String]] = team.groupMap(_._2)(_._1).view.mapValues(_.toSet).toMap

    // Cada evento con su número de ronda (la ronda cambia después de cada ROUND_END).
    val withRound: List[(Event, Int)] =
      ev.zip(ev.scanLeft(1)((r, e) => if roundEnd(e) then r + 1 else r))
    val roundWinner: Map[Int, String] =
      withRound.collect { case (e, r) if roundEnd(e) => e.str("winnerTeam").map(r -> _) }.flatten.toMap
    val rounds = roundWinner.size

    val score = roundWinner.values.toList.groupMapReduce(identity)(_ => 1)(_ + _)
    val winner = ev.find(_.is(Sys.MatchFinished)).flatMap(_.str("winner"))
      .orElse(score.maxByOption(_._2).map(_._1))

    val elims  = ev.filter(elimination)
    val hits   = ev.filter(damageDealt)
    val kills  = countBy(ev)(elimination)
    val deaths = elims.flatMap(_.str("victimId")).groupMapReduce(identity)(_ => 1)(_ + _)

    val players: Map[String, CombatPlayer] = team.keys.toList.sorted.map { p =>
      val myHits = hits.filter(byPlayer(p))
      val dmg    = myHits.flatMap(_.int("amount")).sum
      val k      = kills.getOrElse(p, 0)
      val d      = deaths.getOrElse(p, 0)
      p -> CombatPlayer(
        team = team(p),
        won = winner.contains(team(p)),
        kills = k,
        deaths = d,
        kd = ratio(k, d),
        damage = dmg,
        hits = myHits.size,
        avgDamagePerHit = ratio(dmg, myHits.size),
        headshotPct = pct(myHits.count(_.bool("headshot").contains(true)), myHits.size),
        itemsPicked = ev.count(action("ITEM_PICKED") and byPlayer(p)),
        objectives = ev.count(action("OBJECTIVE_CAPTURED") and byPlayer(p))
      )
    }.toMap

    val weaponKills = elims.flatMap(_.str("weapon")).groupMapReduce(identity)(_ => 1)(_ + _)
    val matchStats = CombatMatch(
      map = ev.find(_.is(Sys.MatchStarted)).flatMap(_.str("map")).getOrElse("desconocido"),
      score = score,
      winner = winner,
      rounds = rounds,
      totalKills = elims.size,
      mvp = kills.maxByOption(_._2).map(_._1),
      mostUsedWeapon = hits.flatMap(_.str("weapon")).groupMapReduce(identity)(_ => 1)(_ + _).maxByOption(_._2).map(_._1),
      killsByWeapon = weaponKills
    )

    // --- Combinada: ¿recoger objetos se traduce en más eliminaciones?
    val allRounds = (1 to rounds).toSet
    val impact = players.keys.toList.sorted.map { p =>
      val itemRounds = withRound.collect { case (e, r) if e.is("ITEM_PICKED") && e.by(p) => r }.toSet
      val killsPerRound = withRound.collect { case (e, r) if elimination(e) && e.by(p) => r }
        .groupMapReduce(identity)(_ => 1)(_ + _)
      def avgKills(rs: Set[Int]) = r2(mean(rs.toList.map(r => killsPerRound.getOrElse(r, 0).toDouble)))
      CombatImpact(
        playerId = p,
        damagePerKill = ratio(players(p).damage, players(p).kills),
        killsPerRoundWithItem = avgKills(itemRounds),
        killsPerRoundWithoutItem = avgKills(allRounds -- itemRounds)
      )
    }

    Analysis(
      stats = writeJs(matchStats),
      playerStats = players.view.mapValues(writeJs(_)).toMap,
      combined = ujson.Obj("impact" -> writeJs(impact)),
      patterns = killStreaks(m) ++ clutches(m, withRound, roundWinner, roster) ++ aces(m, withRound, roster)
    )

  /** RACHA: 3 eliminaciones del mismo jugador en menos de 10 s sin morir. */
  private def killStreaks(m: MatchBatch): List[Detection] =
    Patterns
      .streaks(m.events, n = 3, window = Duration.ofSeconds(10))(
        key = e => Option.when(elimination(e))(e.player),
        resets = e => if elimination(e) then e.str("victimId") else None
      )
      .map { case (p, run) =>
        val secs = r2(run.last.secondsSince(run.head.timestamp))
        Detection("RACHA", gameId, m.matchId, Some(p), at = Some(run.last.timestamp),
          description = s"$p hizo ${run.size} eliminaciones en ${secs}s sin morir")
      }

  /** CLUTCH: el último vivo de su equipo, frente a 2 o más rivales, gana la ronda. */
  private def clutches(
      m: MatchBatch,
      withRound: List[(Event, Int)],
      roundWinner: Map[Int, String],
      roster: Map[String, Set[String]]
  ): List[Detection] =
    roundWinner.toList.sortBy(_._1).flatMap { case (round, w) =>
      val roundElims = withRound.collect { case (e, r) if r == round && elimination(e) => e }
      val states = roundElims.scanLeft(roster) { (alive, e) =>
        val victim = e.str("victimId").getOrElse("")
        alive.view.mapValues(_ - victim).toMap
      }
      val loser = roster.keySet.find(_ != w).getOrElse("")
      states
        .find(s => s.getOrElse(w, Set.empty).size == 1 && s.getOrElse(loser, Set.empty).size >= 2)
        .map { s =>
          val p = s(w).head
          Detection("CLUTCH", gameId, m.matchId, Some(p), Some(w),
            roundElims.lastOption.map(_.timestamp),
            s"$p ganó la ronda $round siendo el último de su equipo contra ${s(loser).size} rivales")
        }
    }

  /** ACE: un jugador elimina a todo el equipo rival en una ronda. */
  private def aces(m: MatchBatch, withRound: List[(Event, Int)], roster: Map[String, Set[String]]): List[Detection] =
    val teamSize = roster.values.map(_.size).maxOption.getOrElse(5)
    withRound
      .collect { case (e, r) if elimination(e) => (r, e) }
      .groupBy { case (r, e) => (r, e.player) }
      .collect { case ((r, p), ks) if ks.size >= teamSize =>
        Detection("ACE", gameId, m.matchId, Some(p), at = Some(ks.map(_._2.timestamp).max),
          description = s"$p eliminó a todo el equipo rival en la ronda $r")
      }
      .toList

  def playerTotals(perMatch: List[ujson.Value]): ujson.Value =
    val xs     = perMatch.map(read[CombatPlayer](_))
    val kills  = xs.map(_.kills).sum
    val deaths = xs.map(_.deaths).sum
    val hits   = xs.map(_.hits).sum
    val heads  = xs.map(x => x.headshotPct * x.hits / 100).sum
    writeJs(CombatTotals(
      matches = xs.size,
      wins = xs.count(_.won),
      kills = kills,
      deaths = deaths,
      kd = ratio(kills, deaths),
      damage = xs.map(_.damage).sum,
      headshotPct = if hits == 0 then 0 else r2(heads * 100 / hits),
      objectives = xs.map(_.objectives).sum
    ))

  def rankScore(totals: ujson.Value): Double =
    val t = read[CombatTotals](totals)
    t.kills * 2 + t.kd * 3 + t.wins * 5
