package gamestats

import gamestats.analysis.Analytics
import gamestats.model.given
import gamestats.store.{Queries, ResultStore}
import java.nio.file.Paths
import upickle.default.*

/** Servicio de análisis de GameStats. Recibe partidas completas desde Go
  * (POST /matches) y expone los resultados en JSON y en un tablero HTML.
  */
object Main extends cask.MainRoutes:
  override def host: String = "0.0.0.0"
  override def port: Int    = sys.env.get("PORT").map(_.toInt).getOrElse(9000)

  private val store = ResultStore(Some(Paths.get(sys.env.getOrElse("DATA_DIR", "data"), "results")))

  private def json(v: ujson.Value, status: Int = 200) =
    cask.Response(ujson.write(v, indent = 2), statusCode = status,
      headers = Seq("Content-Type" -> "application/json; charset=utf-8"))

  private def notFound(what: String) = json(ujson.Obj("error" -> s"$what no encontrado"), 404)

  @cask.get("/health")
  def health() = "ok"

  @cask.post("/matches")
  def receive(request: cask.Request) =
    Analytics.process(request.text()) match
      case Right(r) =>
        store.save(r)
        println(s"partida analizada: ${r.gameId}/${r.matchId} (${r.eventCount} eventos, " +
          s"patrones: ${r.patterns.map(_.rule).mkString(", ")})")
        json(ujson.Obj("matchId" -> r.matchId, "patterns" -> r.patterns.map(_.rule)), 201)
      case Left(err) =>
        println(s"lote rechazado: $err")
        json(ujson.Obj("error" -> err), 400)

  @cask.get("/results/matches")
  def matches() = json(store.all.map(Queries.matchSummary))

  @cask.get("/results/matches/:id")
  def matchById(id: String) = store.get(id).fold(notFound("partida"))(r => json(writeJs(r)))

  @cask.get("/results/players")
  def players() = json(Queries.players(store.all))

  @cask.get("/results/players/:id")
  def player(id: String) = Queries.playerProfile(store.all, id).fold(notFound("jugador"))(json(_))

  @cask.get("/results/games")
  def games() = json(Queries.games(store.all))

  @cask.get("/results/games/:gameId")
  def game(gameId: String) = json(Queries.gameSummary(store.all, gameId))

  @cask.get("/results/patterns")
  def patterns(game: String = "", rule: String = "") =
    json(writeJs(Queries.patterns(store.all, game, rule)))

  @cask.get("/")
  def dashboard() =
    cask.Response(Dashboard.html, headers = Seq("Content-Type" -> "text/html; charset=utf-8"))

  println(s"analytics escuchando en :$port (${store.all.size} resultados cargados)")
  initialize()
