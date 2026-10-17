package gamestats.model

import java.time.{Duration, Instant, OffsetDateTime}
import upickle.default.*

/** Fechas en ISO-8601 (acepta "Z" u offsets como "-06:00"). */
given ReadWriter[Instant] =
  readwriter[String].bimap[Instant](_.toString, s => OffsetDateTime.parse(s).toInstant)

/** Acciones de sistema comunes a todos los juegos. */
object Sys:
  val MatchStarted       = "MATCH_STARTED"
  val MatchFinished      = "MATCH_FINISHED"
  val PlayerJoined       = "PLAYER_JOINED"
  val PlayerLeft         = "PLAYER_LEFT"
  val PlayerDisconnected = "PLAYER_DISCONNECTED"
  val PlayerReconnected  = "PLAYER_RECONNECTED"

/** Evento inmutable tal como lo entrega el módulo Go. `data` depende del juego. */
final case class Event(
    eventId: String,
    timestamp: Instant,
    gameId: String,
    gameVersion: Option[String],
    matchId: String,
    playerId: Option[String],
    eventType: String,
    action: String,
    data: ujson.Obj
):
  def player: String              = playerId.getOrElse("")
  def is(a: String): Boolean      = action == a
  def by(p: String): Boolean      = playerId.contains(p)
  def str(k: String): Option[String]   = field(k).flatMap(_.strOpt)
  def int(k: String): Option[Int]      = field(k).flatMap(_.numOpt).map(_.toInt)
  def long(k: String): Option[Long]    = field(k).flatMap(_.numOpt).map(_.toLong)
  def bool(k: String): Option[Boolean] = field(k).flatMap(_.boolOpt)
  def obj(k: String): Option[ujson.Obj] = field(k).collect { case o: ujson.Obj => o }
  def secondsSince(t: Instant): Double  = Duration.between(t, timestamp).toMillis / 1000.0

  private def field(k: String): Option[ujson.Value] = data.value.get(k)

/** Partida completa (lote) enviada por Go al cerrar la partida. */
final case class MatchBatch(
    matchId: String,
    gameId: String,
    gameVersion: Option[String],
    status: String,
    startedAt: Instant,
    endedAt: Instant,
    players: List[String],
    events: List[Event]
):
  def durationSec: Long = Duration.between(startedAt, endedAt).getSeconds.max(0)
  def finishedAt: Instant = events.find(_.is(Sys.MatchFinished)).map(_.timestamp).getOrElse(endedAt)

/** Situación detectada por una regla que considera varios eventos. */
final case class Detection(
    rule: String,
    gameId: String,
    matchId: String,
    playerId: Option[String] = None,
    team: Option[String] = None,
    at: Option[Instant] = None,
    description: String
) derives ReadWriter

/** Resultado del análisis de una partida: lo que se guarda y se consulta. */
final case class MatchResult(
    matchId: String,
    gameId: String,
    status: String,
    startedAt: Instant,
    endedAt: Instant,
    durationSec: Long,
    players: List[String],
    eventCount: Int,
    generic: ujson.Value,
    genericPlayers: Map[String, ujson.Value],
    stats: ujson.Value,
    playerStats: Map[String, ujson.Value],
    combined: ujson.Value,
    patterns: List[Detection],
    analyzedAt: Instant
) derives ReadWriter
