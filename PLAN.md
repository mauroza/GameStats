# GameStats — Plan de desarrollo

Equipo: Mauro (programación) · Melanny (documentación)

## 1. Arquitectura

```
 simulator (Go) x3  ──POST /events──►  ingest (Go :8080)  ──POST /matches──►  analytics (Scala :9000)
 racing/combat/football                 validar · estado · concurrencia         estadísticas · patrones
 time.Ticker                                                                     GET /results/... (JSON + HTML)
```

| Componente | Lenguaje | Responsabilidad |
|---|---|---|
| `simulator/` | Go | Fuente externa simulada. Un proceso por juego, eventos espaciados con `time.Ticker`, varias partidas en goroutines, ~5 % de eventos inválidos, `--seed` reproducible. |
| `ingest/` | Go | Recibir, validar, deduplicar, agrupar por partida, mantener estado, procesar concurrente, enviar partidas completas a Scala. |
| `analytics/` | Scala 3 | Recibir partidas, estadísticas genéricas + específicas por juego, detección de patrones, resultados JSON consultables. |

Comunicación: HTTP/REST + JSON en ambos tramos (documentar en `docs/`).

## 2. Formato de evento (sobre común)

```json
{ "eventId": "uuid", "timestamp": "RFC3339", "gameId": "racing", "gameVersion": "1.0",
  "matchId": "race-001", "playerId": "p07", "type": "SYSTEM|GAMEPLAY",
  "action": "LAP_COMPLETED", "data": { ... } }
```

- Eventos de sistema (todos los juegos): `MATCH_STARTED`, `PLAYER_JOINED`, `PLAYER_LEFT`, `PLAYER_DISCONNECTED`, `PLAYER_RECONNECTED`, `MATCH_FINISHED`.
- Específicos por juego: definidos en `games/<gameId>.json` (acción → campos requeridos de `data`).

### Juegos
| Juego | Acciones | Regla de secuencia |
|---|---|---|
| racing | LAP_COMPLETED, POSITION_CHANGED, PIT_STOP, PENALTY, CRASH | Remontada (posición ≥ N/2 → top 3) |
| combat | DAMAGE_DEALT, PLAYER_ELIMINATED, ITEM_PICKED, OBJECTIVE_CAPTURED, ROUND_END | Racha (3 bajas < 10 s sin morir), Clutch |
| football | PASS, SHOT, GOAL, FOUL, SUBSTITUTION | Remontada (−2 → empate/victoria), Expulsión (2 amarillas) |
| cards (demo de genericidad) | CARD_PLAYED, TURN_END | — solo config + analizador pequeño |

## 3. Módulo Go — ingest

- `POST /events` (uno o lote) → channel `incoming` → **worker pool** (N goroutines).
- **Validación**: sobre obligatorio (eventId, timestamp, gameId, matchId, type, action; playerId según acción) + reglas de `games/*.json`. Rechazos con motivo → `rejected.log` + contador.
- **Deduplicación** por `eventId`. Eventos tras `MATCH_FINISHED` → rechazados.
- **Estado**: `MatchManager` con una goroutine + channel por partida (`MatchState`: jugadores activos, eventos acumulados, estado STARTED/FINISHED).
- Al `MATCH_FINISHED` (o timeout de inactividad) → envío del lote ordenado por timestamp a Scala, con reintentos.
- `GET /health`, `GET /matches` (estado en vivo), `GET /stats` (recibidos/validos/rechazados).
- Shutdown limpio (`context`, `sync.WaitGroup`). Tests: `go test` de validador y manager.

## 4. Módulo Scala — analytics

- Stack: Scala 3 + sbt + cask (HTTP) + upickle (JSON) + munit (tests).
- `POST /matches` → `List[Event]` → modelos inmutables (`case class`).
- `trait GameAnalyzer` + registro `Map[gameId, GameAnalyzer]`; si no hay analizador, solo estadísticas genéricas.
- Genéricas: duración, eventos por jugador, desconexiones, tiempo activo.
- Mínimos del enunciado (por juego):
  - por jugador (ej. K/D, mejor vuelta, % pases)
  - por partida (ej. marcador, vuelta más rápida)
  - combinada (ej. tiempo perdido = penalizaciones + pits + choques; asistencias PASS→GOAL)
  - patrón de secuencia (remontada, racha, clutch, expulsión)
- Combinadores genéricos reutilizables: `streak(pred, n, window)`, `comeback(...)`.
- Uso justificado de `filter/map/flatMap/groupBy/foldLeft/reduce/collect` + composición.
- Consulta: `GET /results/matches`, `/results/matches/{id}`, `/results/players/{id}`, `/results/games/{gameId}`, `/results/patterns`, `GET /` dashboard HTML simple. Resultados también persistidos a `data/results/*.json`.

## 5. DevOps

- Repo GitHub, commits pequeños y frecuentes, ramas por feature.
- `Dockerfile` multi-stage por módulo (ingest, simulator, analytics).
- `docker-compose.yml`: analytics, ingest, simulator-racing, simulator-combat, simulator-football.
- GitHub Actions: build + test Go, build + test Scala, build de imágenes, push a GHCR (en `main`).
- Deploy: VM con Docker (ej. Azure for Students / Oracle Free) vía SSH + `docker compose pull && up -d`. **Decidir entorno.**

## 6. Fases y reparto

| Fase | Mauro (código) | Melanny (docs) |
|---|---|---|
| 0. Diseño | Revisar catálogo de eventos | Cerrar juegos/eventos/métricas → `docs/eventos.md` |
| 1. Base | Repo, estructura, `Event`, validador, `games/*.json` | Especificación formato de eventos (+ ejemplos válidos/inválidos) |
| 2. Go | ingest (workers, estado, envío) + simulator | Justificación juegos y eventos |
| 3. Scala | analytics, analizadores, patrones, endpoints | Descripción estadísticas y reglas |
| 4. Integración | Dockerfiles, compose, demo 3 simuladores | Arquitectura + diagrama; Go imperativo vs Scala funcional |
| 5. Entrega | CI/CD, deploy, cuarto juego `cards` | Manual instalación/ejecución/deploy, guion demo, revisión final |

## 7. Checklist de entregables

- [ ] Código Go y Scala
- [ ] Especificación del formato de eventos
- [ ] Juegos considerados y justificación de eventos
- [ ] Estadísticas y reglas implementadas
- [ ] Dockerfiles + compose
- [ ] Pipeline CI/CD
- [ ] Manual de instalación, ejecución y deployment
- [ ] Demo funcional (3 simuladores concurrentes)
