package gamestats.model

import java.time.{Instant, OffsetDateTime}
import scala.util.Try

/** Conversión del JSON recibido desde Go a las estructuras inmutables. */
object Codec:

  def parseTime(s: String): Instant = OffsetDateTime.parse(s).toInstant

  def parseEvent(v: ujson.Value): Event =
    val o = v.obj
    Event(
      eventId = o("eventId").str,
      timestamp = parseTime(o("timestamp").str),
      gameId = o("gameId").str,
      gameVersion = o.get("gameVersion").flatMap(_.strOpt),
      matchId = o("matchId").str,
      playerId = o.get("playerId").flatMap(_.strOpt).filter(_.nonEmpty),
      eventType = o("type").str,
      action = o("action").str,
      data = o.get("data").collect { case d: ujson.Obj => d }.getOrElse(ujson.Obj())
    )

  def parseBatch(v: ujson.Value): Either[String, MatchBatch] =
    Try {
      val o = v.obj
      MatchBatch(
        matchId = o("matchId").str,
        gameId = o("gameId").str,
        gameVersion = o.get("gameVersion").flatMap(_.strOpt),
        status = o.get("status").flatMap(_.strOpt).getOrElse("FINISHED"),
        startedAt = parseTime(o("startedAt").str),
        endedAt = parseTime(o("endedAt").str),
        players = o.get("players").flatMap(_.arrOpt).map(_.map(_.str).toList).getOrElse(Nil),
        events = o("events").arr.map(parseEvent).toList.sortBy(_.timestamp)
      )
    }.toEither.left.map(e => s"lote inválido: ${e.getMessage}")

  def parseBatch(text: String): Either[String, MatchBatch] =
    Try(ujson.read(text)).toEither.left
      .map(e => s"JSON inválido: ${e.getMessage}")
      .flatMap(parseBatch)
