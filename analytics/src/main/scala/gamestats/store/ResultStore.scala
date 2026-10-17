package gamestats.store

import gamestats.model.{*, given}
import java.nio.charset.StandardCharsets.UTF_8
import java.nio.file.{Files, Path}
import java.util.concurrent.atomic.AtomicReference
import scala.jdk.CollectionConverters.*
import scala.util.Try
import upickle.default.*

/** Guarda los resultados en memoria (mapa inmutable en un AtomicReference,
  * seguro con peticiones concurrentes) y opcionalmente en disco como JSON.
  */
final class ResultStore(dir: Option[Path]):
  private val ref = AtomicReference(load())

  def save(r: MatchResult): Unit =
    ref.updateAndGet(_.updated(r.matchId, r))
    dir.foreach { d =>
      Files.createDirectories(d)
      Files.writeString(d.resolve(s"${r.matchId}.json"), write(r, indent = 2), UTF_8)
    }

  def get(id: String): Option[MatchResult] = ref.get.get(id)

  def all: List[MatchResult] = ref.get.values.toList.sortBy(_.startedAt)

  private def load(): Map[String, MatchResult] =
    dir.filter(Files.isDirectory(_)).toList
      .flatMap(d => Files.list(d).iterator.asScala.filter(_.toString.endsWith(".json")).toList)
      .flatMap(f => Try(read[MatchResult](Files.readString(f, UTF_8))).toOption)
      .map(r => r.matchId -> r)
      .toMap
