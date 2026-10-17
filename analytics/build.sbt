ThisBuild / scalaVersion := "3.3.4"
ThisBuild / organization := "gamestats"

lazy val root = (project in file("."))
  .settings(
    name := "analytics",
    run / fork := true,
    run / outputStrategy := Some(StdoutOutput),
    assembly / mainClass := Some("gamestats.Main"),
    assembly / assemblyJarName := "analytics.jar",
    assembly / assemblyMergeStrategy := {
      case PathList("META-INF", "services", _*)               => MergeStrategy.concat
      case PathList("META-INF", xs @ _*) if xs.lastOption.exists(f =>
            f.endsWith(".SF") || f.endsWith(".DSA") || f.endsWith(".RSA") || f == "MANIFEST.MF") =>
        MergeStrategy.discard
      case x if x.endsWith("module-info.class")              => MergeStrategy.discard
      case x =>
        val old = (assembly / assemblyMergeStrategy).value
        old(x)
    },
    libraryDependencies ++= Seq(
      "com.lihaoyi"   %% "cask"    % "0.10.2",
      "com.lihaoyi"   %% "upickle" % "4.1.0",
      "org.scalameta" %% "munit"   % "1.0.3" % Test
    )
  )
