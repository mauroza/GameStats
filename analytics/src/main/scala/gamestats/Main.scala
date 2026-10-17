package gamestats

object Main extends cask.MainRoutes:
  override def host: String = "0.0.0.0"
  override def port: Int = sys.env.get("PORT").map(_.toInt).getOrElse(9000)

  @cask.get("/health")
  def health() = "ok"

  initialize()
