# GameStats

Plataforma multilenguaje para procesamiento y análisis de eventos de videojuegos.
Proyecto 1 — Lenguajes de Programación, TEC.

| Módulo | Lenguaje | Rol |
|---|---|---|
| `simulator/` | Go | Fuente externa simulada de eventos |
| `ingest/` | Go | Ingestión, validación, estado de partidas, concurrencia |
| `analytics/` | Scala 3 | Estadísticas y detección de patrones (funcional) |
| `games/` | JSON | Configuración de acciones por juego |
| `docs/` | — | Documentación |

Ver [PLAN.md](PLAN.md) para el plan completo.

## Ejecutar con Docker (recomendado)

Requisitos: Docker Desktop (o Docker Engine + Compose v2).

```bash
docker compose up --build
```

Levanta `analytics` (:9000), `ingest` (:8080) y los 3 simuladores en paralelo (racing, combat, football), que envían sus partidas y terminan. Los builds ejecutan los tests de cada módulo.

- Tablero: http://localhost:9000
- Estado de ingestión: http://localhost:8080/stats

Volver a correr un simulador con otros parámetros:

```bash
docker compose run --rm simulator-racing -game racing -matches 5 -interval 300ms
```

Los resultados persisten en volúmenes (`analytics-data`, `ingest-data`). Para borrarlos:

```bash
docker compose down -v
```

## Ejecutar en local (desarrollo)

```bash
cd ingest && go run .          # :8080
cd analytics && sbt run        # :9000 (tablero en http://localhost:9000)
```

## Servicio ingest (Go)

Flujo: `POST /events` → cola → worker pool (validación + deduplicación) → router → una goroutine por partida → al `MATCH_FINISHED` (+ período de gracia) se envía la partida ordenada a Scala (`POST /matches`). Si Scala no responde, la partida queda en `data/outbox/` y se reintenta al arrancar.

| Endpoint | Descripción |
|---|---|
| `POST /events` | Un evento o un arreglo de eventos (JSON) |
| `GET /matches` | Partidas en curso y su estado |
| `GET /stats` | Contadores: recibidos, válidos, rechazados, duplicados, enviados |
| `GET /rejected` | Últimos eventos rechazados con motivo |
| `GET /health` | Salud del servicio |

Variables de entorno: `ADDR` (:8080), `GAMES_DIR` (../games), `DATA_DIR` (data), `ANALYTICS_URL` (http://localhost:9000/matches), `WORKERS` (4), `GRACE` (2s), `IDLE_TIMEOUT` (60s), `SEND_RETRIES` (4), `VERBOSE` (false).

## Simulador (Go)

Representa el sistema externo de juegos. Cada proceso simula un juego; cada partida corre en su goroutine y envía un evento por tick (`time.Ticker`). Inyecta eventos inválidos (sin `matchId`, timestamp roto, acción desconocida, `data` incompleta, duplicados, eventos tardíos) para probar la validación.

Demo de concurrencia (tres terminales, con ingest corriendo):

```bash
cd simulator && go run . -game racing -matches 3 -interval 300ms
```
```bash
cd simulator && go run . -game combat -matches 3 -interval 250ms
```
```bash
cd simulator && go run . -game football -matches 3 -interval 150ms
```

| Flag | Default | Descripción |
|---|---|---|
| `-game` | — | `racing`, `combat` o `football` (o env `GAME`) |
| `-matches` | 3 | partidas en paralelo |
| `-interval` | 300ms | tiempo entre eventos de una partida |
| `-target` | http://localhost:8080/events | endpoint de ingest (o env `TARGET`) |
| `-seed` | aleatoria | semilla para resultados reproducibles |
| `-invalid` | 0.05 | probabilidad de inyectar un evento inválido |
| `-dry-run` | false | imprime los eventos (JSON lines) sin enviarlos |

Escenarios generados para las reglas: remontada en carreras (piloto rápido que choca en la vuelta 1), rachas y clutch en combate, remontada y doble amarilla en fútbol.

## Servicio analytics (Scala)

Recibe cada partida completa desde Go y la analiza con colecciones inmutables y operaciones funcionales (`filter`, `map`, `groupBy`, `groupMapReduce`, `foldLeft`, `scanLeft`, `collect`, composición de predicados).

- `GameAnalyzer` (trait) + registro `Map[gameId, GameAnalyzer]`: un juego nuevo = un objeto nuevo + una línea en el registro. Juegos sin analizador reciben igual las estadísticas genéricas.
- Combinadores genéricos de patrones: `Patterns.streaks` (rachas en ventana de tiempo) y `Patterns.comebacks` (remontadas sobre series).
- Resultados en memoria + JSON en `data/results/`.

| Juego | Por jugador | Por partida | Combinada | Patrones |
|---|---|---|---|---|
| todos | eventos, tiempo conectado | eventos por tipo, duración, desconexiones | — | ABANDONO |
| racing | mejor vuelta, promedio, consistencia, posiciones ganadas | ganador, podio, vuelta rápida, adelantamientos | tiempo perdido (penalización + pits + choques) y posición sin incidentes | REMONTADA, PUNTA_A_PUNTA |
| combat | kills, deaths, K/D, daño, % headshot | marcador por rondas, MVP, arma más usada | daño por baja, bajas por ronda con/sin objeto | RACHA, CLUTCH, ACE |
| football | % pase, goles, asistencias derivadas, pases clave | marcador, posesión, tiros, tarjetas | conversión, % a puerta, % goles asistidos | REMONTADA, DOBLE_AMARILLA, HAT_TRICK |

| Endpoint | Descripción |
|---|---|
| `POST /matches` | Partida completa (lo usa ingest) |
| `GET /results/matches` · `/results/matches/{id}` | Resumen de partidas · detalle completo |
| `GET /results/players` · `/results/players/{id}` | Jugadores · perfil agregado por juego |
| `GET /results/games` · `/results/games/{gameId}` | Resumen por juego con tabla de posiciones |
| `GET /results/patterns?game=&rule=` | Patrones detectados |
| `GET /` | Tablero HTML |

Variables de entorno: `PORT` (9000), `DATA_DIR` (data).

```bash
cd analytics && sbt test
```

## Equipo

- Mauro — programación
- Melanny — documentación
