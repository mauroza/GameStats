package gamestats.store

import gamestats.analysis.{Analyzers, GenericPlayer, Num}
import gamestats.model.{*, given}
import java.time.Instant
import upickle.default.*

/** Consultas puras sobre la lista de resultados. Las estadísticas agregadas
  * (por jugador o por juego) se calculan al consultar, a partir de las
  * estadísticas por partida guardadas.
  */
object Queries:

  def matchSummary(r: MatchResult): ujson.Obj =
    ujson.Obj(
      "matchId"     -> r.matchId,
      "gameId"      -> r.gameId,
      "status"      -> r.status,
      "startedAt"   -> r.startedAt.toString,
      "durationSec" -> r.durationSec.toDouble,
      "players"     -> r.players.size,
      "eventCount"  -> r.eventCount,
      "winner"      -> r.stats.objOpt.flatMap(_.get("winner")).getOrElse(ujson.Null),
      "patterns"    -> r.patterns.map(_.rule)
    )

  def players(rs: List[MatchResult]): List[ujson.Obj] =
    rs.flatMap(r => r.players.map(_ -> r))
      .groupMap(_._1)(_._2)
      .toList
      .sortBy(_._1)
      .map { case (p, ms) =>
        ujson.Obj("playerId" -> p, "matches" -> ms.size, "games" -> ms.map(_.gameId).distinct.sorted)
      }

  /** Perfil de un jugador agregado por juego a lo largo de todas sus partidas. */
  def playerProfile(rs: List[MatchResult], id: String): Option[ujson.Obj] =
    val mine = rs.filter(_.players.contains(id))
    Option.when(mine.nonEmpty) {
      val byGame = mine.groupBy(_.gameId).toList.sortBy(_._1).map { case (g, ms) =>
        val generic = ms.flatMap(_.genericPlayers.get(id)).map(read[GenericPlayer](_))
        val totals  = Analyzers.registry.get(g).map(_.playerTotals(ms.flatMap(_.playerStats.get(id))))
        g -> ujson.Obj(
          "matches"      -> ms.size,
          "matchIds"     -> ms.map(_.matchId),
          "connectedSec" -> generic.map(_.connectedSec).sum.toDouble,
          "abandoned"    -> generic.count(_.abandoned),
          "totals"       -> totals.getOrElse(ujson.Obj())
        )
      }
      ujson.Obj(
        "playerId" -> id,
        "matches"  -> mine.size,
        "games"    -> ujson.Obj.from(byGame),
        "patterns" -> writeJs(mine.flatMap(_.patterns).filter(_.playerId.contains(id)))
      )
    }

  /** Resumen de un juego con tabla de posiciones. */
  def gameSummary(rs: List[MatchResult], gameId: String): ujson.Obj =
    val ms = rs.filter(_.gameId == gameId)
    val leaderboard = Analyzers.registry.get(gameId).toList.flatMap { a =>
      ms.flatMap(_.playerStats.toList)
        .groupMap(_._1)(_._2)
        .toList
        .map { case (p, st) => (p, a.playerTotals(st)) }
        .sortBy { case (p, t) => (-a.rankScore(t), p) }
        .take(10)
        .map { case (p, t) => ujson.Obj("playerId" -> p, "totals" -> t) }
    }
    ujson.Obj(
      "gameId"         -> gameId,
      "matches"        -> ms.size,
      "byStatus"       -> writeJs(ms.groupMapReduce(_.status)(_ => 1)(_ + _)),
      "totalEvents"    -> ms.map(_.eventCount).sum,
      "avgDurationSec" -> Num.r2(Num.mean(ms.map(_.durationSec.toDouble))),
      "patternsByRule" -> writeJs(ms.flatMap(_.patterns).groupMapReduce(_.rule)(_ => 1)(_ + _)),
      "leaderboard"    -> leaderboard
    )

  def games(rs: List[MatchResult]): List[ujson.Obj] =
    rs.map(_.gameId).distinct.sorted.map(gameSummary(rs, _))

  def patterns(rs: List[MatchResult], game: String, rule: String): List[Detection] =
    rs.flatMap(_.patterns)
      .filter(d => game.isEmpty || d.gameId == game)
      .filter(d => rule.isEmpty || d.rule.equalsIgnoreCase(rule))
      .sortBy(_.at.getOrElse(Instant.EPOCH))
