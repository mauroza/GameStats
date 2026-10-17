package gamestats.analysis

import gamestats.model.Event
import java.time.Duration

/** Predicados componibles sobre eventos. */
type Pred = Event => Boolean

extension (p: Pred)
  infix def and(q: Pred): Pred = e => p(e) && q(e)
  infix def or(q: Pred): Pred  = e => p(e) || q(e)

def action(a: String): Pred   = _.action == a
def byPlayer(id: String): Pred = _.playerId.contains(id)

/** Combinadores genéricos de detección de patrones, independientes del juego. */
object Patterns:

  /** Rachas: `n` eventos de una misma clave dentro de `window`, sin que un
    * evento de reinicio para esa clave ocurra entre ellos.
    *
    * @param key    clave del evento que cuenta para la racha (None = no cuenta)
    * @param resets clave que este evento reinicia (None = no reinicia)
    * @return (clave, eventos de la racha), una vez por racha
    */
  def streaks(events: List[Event], n: Int, window: Duration)(
      key: Event => Option[String],
      resets: Event => Option[String]
  ): List[(String, List[Event])] =
    val (_, found) = events.foldLeft((Map.empty[String, Vector[Event]], List.empty[(String, List[Event])])) {
      case ((runs, found), e) =>
        val afterReset = resets(e).fold(runs)(runs - _)
        key(e) match
          case None => (afterReset, found)
          case Some(k) =>
            val run = (afterReset.getOrElse(k, Vector.empty) :+ e)
              .filter(x => Duration.between(x.timestamp, e.timestamp).compareTo(window) < 0)
            val next = if run.size == n then (k, run.toList) :: found else found
            (afterReset.updated(k, run), next)
    }
    found.reverse

  /** Remontadas: claves cuya serie de valores estuvo "en desventaja" y cuyo
    * último valor indica recuperación. Sirve para posiciones en carreras,
    * diferencia de goles en fútbol, etc.
    */
  def comebacks[K](series: Map[K, List[Int]])(
      behind: List[Int] => Boolean,
      recovered: Int => Boolean
  ): List[(K, List[Int])] =
    series.toList.collect { case (k, xs) if xs.nonEmpty && behind(xs) && recovered(xs.last) => (k, xs) }

/** Utilidades numéricas puras. */
object Num:
  def r2(d: Double): Double = math.round(d * 100) / 100.0
  def mean(xs: Seq[Double]): Double = if xs.isEmpty then 0 else xs.sum / xs.size
  def stddev(xs: Seq[Double]): Double =
    if xs.size < 2 then 0
    else
      val m = mean(xs)
      math.sqrt(xs.map(x => (x - m) * (x - m)).sum / (xs.size - 1))
  def pct(part: Int, total: Int): Double  = if total == 0 then 0 else r2(part * 100.0 / total)
  def ratio(a: Double, b: Double): Double = if b == 0 then a else r2(a / b)

  /** Cuenta eventos por jugador. */
  def countBy(events: List[Event])(p: Pred): Map[String, Int] =
    events.filter(p).groupMapReduce(_.player)(_ => 1)(_ + _)
