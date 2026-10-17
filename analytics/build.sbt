ThisBuild / scalaVersion := "3.3.4"
ThisBuild / organization := "gamestats"

lazy val root = (project in file("."))
  .settings(
    name := "analytics",
    run / fork := true,
    run / outputStrategy := Some(StdoutOutput),
    libraryDependencies ++= Seq(
      "com.lihaoyi"   %% "cask"    % "0.10.2",
      "com.lihaoyi"   %% "upickle" % "4.1.0",
      "org.scalameta" %% "munit"   % "1.0.3" % Test
    )
  )
