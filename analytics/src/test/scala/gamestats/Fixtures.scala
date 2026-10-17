package gamestats

import gamestats.model.*
import java.time.Instant

/** Constructor compacto de partidas para los tests. */
final class MatchBuilder(game: String, matchId: String = "m1"):
  private val t0 = Instant.parse("2026-10-16T20:00:00Z")
  private var events = Vector.empty[Event]

  def at(sec: Double, action: String, player: String = "", data: (String, ujson.Value)*): MatchBuilder =
    val typ = if action.startsWith("MATCH_") || action.startsWith("PLAYER_") then "SYSTEM" else "GAMEPLAY"
    events :+= Event(
      eventId = s"$matchId-${events.size + 1}",
      timestamp = t0.plusMillis((sec * 1000).toLong),
      gameId = game,
      gameVersion = Some("1.0"),
      matchId = matchId,
      playerId = Option(player).filter(_.nonEmpty),
      eventType = typ,
      action = action,
      data = ujson.Obj.from(data)
    )
    this

  def build: MatchBatch =
    val sorted = events.toList.sortBy(_.timestamp)
    MatchBatch(matchId, game, Some("1.0"), "FINISHED", sorted.head.timestamp, sorted.last.timestamp,
      sorted.flatMap(_.playerId).distinct.sorted, sorted)

def rules(r: MatchResult): List[String] = r.patterns.map(_.rule)
