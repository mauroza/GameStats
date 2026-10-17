package gamestats.analysis

import gamestats.model.*
import java.time.Instant

/** Resultado del análisis específico de un juego. */
final case class Analysis(
    stats: ujson.Value,
    playerStats: Map[String, ujson.Value],
    combined: ujson.Value,
    patterns: List[Detection]
)

object Analysis:
  val empty: Analysis = Analysis(ujson.Obj(), Map.empty, ujson.Obj(), Nil)

/** Contrato de cada juego. Agregar un juego = un objeto nuevo + una línea en el registro. */
trait GameAnalyzer:
  def gameId: String

  /** Estadísticas de una partida: por partida, por jugador, combinadas y patrones. */
  def analyze(m: MatchBatch): Analysis

  /** Agrega las estadísticas de un jugador a lo largo de varias partidas. */
  def playerTotals(perMatch: List[ujson.Value]): ujson.Value

  /** Puntaje para ordenar la tabla de posiciones del juego. */
  def rankScore(totals: ujson.Value): Double

object Analyzers:
  val registry: Map[String, GameAnalyzer] =
    List(RacingAnalyzer, CombatAnalyzer, FootballAnalyzer).map(a => a.gameId -> a).toMap

object Analytics:

  /** Análisis completo: estadísticas genéricas + las del juego (si existe analizador). */
  def analyze(m: MatchBatch): MatchResult =
    val generic  = GenericAnalyzer.analyze(m)
    val specific = Analyzers.registry.get(m.gameId).map(_.analyze(m)).getOrElse(Analysis.empty)
    MatchResult(
      matchId = m.matchId,
      gameId = m.gameId,
      status = m.status,
      startedAt = m.startedAt,
      endedAt = m.endedAt,
      durationSec = m.durationSec,
      players = m.players,
      eventCount = m.events.size,
      generic = generic.stats,
      genericPlayers = generic.playerStats,
      stats = specific.stats,
      playerStats = specific.playerStats,
      combined = specific.combined,
      patterns = (generic.patterns ++ specific.patterns).sortBy(_.at.getOrElse(Instant.EPOCH)),
      analyzedAt = Instant.now()
    )

  /** Composición: texto JSON -> lote -> resultado. */
  val process: String => Either[String, MatchResult] =
    ((s: String) => Codec.parseBatch(s)).andThen(_.map(analyze))
