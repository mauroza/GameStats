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

## Ejecutar en local (temporal)

```bash
cd ingest && go run .          # :8080
cd analytics && sbt run        # :9000
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

## Equipo

- Mauro — programación
- Melanny — documentación
