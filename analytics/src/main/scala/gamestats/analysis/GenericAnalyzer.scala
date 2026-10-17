package gamestats.analysis

import gamestats.model.*
import java.time.{Duration, Instant}
import upickle.default.*

final case class GenericMatch(
    eventCount: Int,
    eventsByType: Map[String, Int],
    eventsByAction: Map[String, Int],
    playerCount: Int,
    durationSec: Long,
    disconnections: Int,
    reconnections: Int
) derives ReadWriter

final case class GenericPlayer(
    events: Int,
    disconnections: Int,
    connectedSec: Long,
    abandoned: Boolean
) derives ReadWriter

/** Estadísticas que solo usan el sobre común: sirven para cualquier juego. */
object GenericAnalyzer:

  private val connection: Pred =
    action(Sys.PlayerDisconnected) or action(Sys.PlayerReconnected) or action(Sys.PlayerLeft)
  private val leaves: Pred = action(Sys.PlayerDisconnected) or action(Sys.PlayerLeft)

  def analyze(m: MatchBatch): Analysis =
    val ev = m.events
    val players = (m.players ++ ev.flatMap(_.playerId)).distinct.sorted

    val stats = GenericMatch(
      eventCount = ev.size,
      eventsByType = ev.groupMapReduce(_.eventType)(_ => 1)(_ + _),
      eventsByAction = ev.groupMapReduce(_.action)(_ => 1)(_ + _),
      playerCount = players.size,
      durationSec = m.durationSec,
      disconnections = ev.count(action(Sys.PlayerDisconnected)),
      reconnections = ev.count(action(Sys.PlayerReconnected))
    )

    val perPlayer = players.map(p => p -> playerStats(m, p)).toMap

    val abandons = perPlayer.toList.collect { case (p, s) if s.abandoned =>
      val at = ev.filter(byPlayer(p) and leaves).lastOption.map(_.timestamp)
      Detection("ABANDONO", m.gameId, m.matchId, Some(p), at = at,
        description = s"$p salió de la partida y no volvió a tener actividad")
    }

    Analysis(writeJs(stats), perPlayer.view.mapValues(writeJs(_)).toMap, ujson.Obj(), abandons)

  /** Tiempo conectado: desde que entra hasta el final, menos los intervalos desconectado. */
  def playerStats(m: MatchBatch, p: String): GenericPlayer =
    val mine  = m.events.filter(byPlayer(p))
    val start = mine.find(_.is(Sys.PlayerJoined)).orElse(mine.headOption).map(_.timestamp).getOrElse(m.startedAt)
    val conn  = mine.filter(connection)

    val (since, total) = conn.foldLeft((Option(start), 0L)) {
      case ((Some(s), acc), e) if leaves(e) => (None, acc + secs(s, e.timestamp))
      case ((None, acc), e) if e.is(Sys.PlayerReconnected) => (Some(e.timestamp), acc)
      case (state, _) => state
    }
    val connected = total + since.map(secs(_, m.endedAt)).getOrElse(0L)

    GenericPlayer(
      events = mine.size,
      disconnections = conn.count(leaves),
      connectedSec = connected,
      // Abandono: su último evento de conexión es una salida y no tuvo actividad después.
      abandoned = conn.lastOption.exists(last => leaves(last) && mine.lastOption.contains(last))
    )

  private def secs(a: Instant, b: Instant): Long = Duration.between(a, b).getSeconds.max(0)
