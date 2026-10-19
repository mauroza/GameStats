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

```
 simulator (Go) x3  ──POST /events──►  ingest (Go :8080)  ──POST /matches──►  analytics (Scala :9000)
 racing/combat/football                 validar · estado · concurrencia         estadísticas · patrones
 time.Ticker                                                                     GET /results/... + tablero
```

Ver [PLAN.md](PLAN.md) para el plan completo y [docs/demo/GUION.md](docs/demo/GUION.md) para el guion de la demo.

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

## CI/CD (GitHub Actions)

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) corre en cada push y pull request:

| Job | Qué hace |
|---|---|
| Go (ingest, simulator) | `gofmt`, `go vet`, tests con `-race`, build; publica el binario como artefacto |
| Scala (analytics) | `sbt test assembly`; publica el jar |
| Prueba end-to-end | [`scripts/e2e.sh`](scripts/e2e.sh): `docker compose up --build` con los 3 simuladores y verifica que las 9 partidas llegan a analytics sin errores; publica los resultados JSON y los logs |
| Imágenes Docker | Solo en `main`: construye y publica `gamestats-ingest`, `gamestats-simulator` y `gamestats-analytics` en `ghcr.io/mauroza/` con tags `latest` y el SHA corto |
| Deploy a VM (opcional) | Solo si el repo tiene configurado `DEPLOY_HOST`: despliega en una VM propia por SSH (ver más abajo). Hoy no se usa: producción está en Render |

**Deploy continuo:** Render está conectado al repositorio y vuelve a construir y desplegar `analytics` e `ingest` en cada push a `main`.

La prueba e2e también corre en local:

```bash
bash scripts/e2e.sh
```

## Deployment (Render)

Producción corre en [Render](https://render.com) (plan gratis), definido como Blueprint en [`render.yaml`](render.yaml): dos web services Docker que Render construye desde los Dockerfiles del repo.

| Servicio | URL |
|---|---|
| Tablero y API de resultados (Scala) | https://gamestats-analytics-nz8a.onrender.com |
| Ingestión (Go) | https://gamestats-ingest.onrender.com — `/stats`, `/rejected`, `/matches` |

- Los servicios gratis de Render no se comunican por red privada: `ingest` envía las partidas a la URL pública de `analytics` (`ANALYTICS_URL`).
- En el plan gratis los servicios se duermen tras 15 minutos sin tráfico y tardan ~1 minuto en despertar. Antes de una demo, abrir ambas URLs.
- El disco es efímero: los resultados se reinician cuando Render reinicia el servicio.

**Crear el despliegue desde cero:** en Render, New → Blueprint, conectar GitHub y elegir `mauroza/GameStats`; Render lee `render.yaml` y crea ambos servicios. Si Render asigna a `analytics` una URL distinta, actualizar `ANALYTICS_URL` en `render.yaml`.

**Demo con simuladores externos** (desde cualquier computadora):

```bash
cd simulator && go run . -game racing -matches 3 -target https://gamestats-ingest.onrender.com/events
```

### Alternativa: VM propia con Docker

[`deploy/`](deploy/) permite desplegar en cualquier VM Ubuntu 24.04 con [`deploy/docker-compose.prod.yml`](deploy/docker-compose.prod.yml), que usa las imágenes publicadas en GHCR:

1. Instalar Docker y swap: `ssh usuario@IP 'bash -s' < deploy/setup-vm.sh`
2. Configurar en GitHub la variable `DEPLOY_HOST` (IP), la variable `DEPLOY_USER` (usuario SSH) y el secret `DEPLOY_SSH_KEY` (llave privada). El job de deploy del pipeline queda activo.

Se intentó primero en Azure for Students, pero la suscripción solo permite 5 regiones y en ellas no hay cuota ni capacidad para VMs pequeñas.

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
