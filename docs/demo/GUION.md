# Guion de la demo (~10 minutos)

Sistema desplegado en Render:

- Tablero y API de resultados: https://gamestats-analytics-nz8a.onrender.com
- Ingestión: https://gamestats-ingest.onrender.com

Si Render falla, ver **Plan B** al final: todo funciona igual en local.

Los comandos son para **PowerShell** en Windows (por eso `curl.exe` y no `curl`).

## Antes de la presentación (15 min antes)

1. **Despertar y limpiar los servicios.** En el plan gratis se duermen tras 15 minutos sin uso y tardan ~1 minuto en despertar. En https://dashboard.render.com, entrar a cada servicio (`gamestats-analytics` y `gamestats-ingest`) y usar **Manual Deploy → Restart service**. Al reiniciar, el tablero queda vacío (el disco de Render es efímero).
2. Comprobar que responden (debe decir `ok`):
   - https://gamestats-analytics-nz8a.onrender.com/health
   - https://gamestats-ingest.onrender.com/health
3. Compilar el simulador una vez para que la demo no espere la compilación:
   ```bash
   cd simulator
   go build -o bin/simulator.exe .
   ```
4. Abrir en el navegador:
   - Pestaña 1: https://gamestats-analytics-nz8a.onrender.com (tablero; debe mostrar 0 partidas)
   - Pestaña 2: https://gamestats-ingest.onrender.com/stats
   - Pestaña 3: Render → `gamestats-ingest` → **Logs** (logs en vivo de la ingestión)
   - Pestaña 4: https://github.com/mauroza/GameStats/actions
5. Abrir **3 terminales** en la carpeta `GameStats/simulator`, una por simulador.

## 1. Arquitectura (1 min)

Mostrar el diagrama del README y decirlo en una frase:

> Los simuladores representan el sistema externo de juegos. **Go** recibe los eventos, los valida, mantiene el estado de cada partida y los procesa en paralelo. Cuando una partida termina, se la envía completa a **Scala**, que calcula las estadísticas con programación funcional.

## 2. Concurrencia: tres simuladores a la vez (3 min)

Lanzar los tres **casi al mismo tiempo**, uno en cada terminal:

Terminal 1:
```bash
./bin/simulator.exe -game racing -matches 3 -seed 2 -interval 300ms -target https://gamestats-ingest.onrender.com/events
```
Terminal 2:
```bash
./bin/simulator.exe -game combat -matches 3 -seed 1 -interval 200ms -target https://gamestats-ingest.onrender.com/events
```
Terminal 3:
```bash
./bin/simulator.exe -game football -matches 3 -seed 2 -interval 120ms -target https://gamestats-ingest.onrender.com/events
```

Mientras corren, explicar:
- Cada simulador es un **proceso aparte** (como pidió el profe). Cada partida corre en su propia **goroutine** y envía un evento por tick de `time.Ticker`.
- En la pestaña de **Logs** de Render se ven las 9 partidas **abriéndose y cerrándose intercaladas**: el servidor HTTP atiende cada request en una goroutine, un **worker pool** valida, y un router reparte cada evento a la goroutine de su partida por **channels**.
- Refrescar https://gamestats-ingest.onrender.com/matches: las partidas en curso con sus jugadores activos (el estado que mantiene Go).
- Código para mostrar si preguntan: `ingest/internal/matches/manager.go` (router + una goroutine por partida) y `ingest/internal/pipeline/pipeline.go` (worker pool).

Los simuladores tardan 1 a 2 minutos en terminar.

## 3. Validación: eventos inválidos (1 min)

- https://gamestats-ingest.onrender.com/stats: recibidos, válidos, rechazados y duplicados.
- https://gamestats-ingest.onrender.com/rejected: cada rechazo con su motivo (sin `matchId`, timestamp roto, acción desconocida, `data` incompleta, duplicado, evento posterior al fin de la partida).
- Explicar que el simulador inyecta ~5 % de eventos inválidos **a propósito**, y que el número de rechazados coincide con lo que reporta cada simulador al terminar ("N inválidos inyectados").

## 4. Análisis en Scala: tablero (3 min)

https://gamestats-analytics-nz8a.onrender.com:
- Los KPI: partidas, eventos, jugadores y patrones.
- Pestañas por juego: **tabla de posiciones** (estadística agregada por jugador entre varias partidas).
- Clic en un jugador: perfil con totales y patrones.
- **Patrones detectados** (reglas que miran secuencias de eventos):
  - Carreras: **REMONTADA** (era de los dos últimos en la vuelta 1 y terminó en el podio).
  - Combate: **RACHA** (3 bajas en menos de 10 s sin morir), **CLUTCH** (último vivo contra 2 o más y gana la ronda), **ACE**.
  - Fútbol: **REMONTADA** (perdía por 2 y empató o ganó), **DOBLE_AMARILLA**.
- Detalle de una partida (estadística por partida y combinada): `https://gamestats-analytics-nz8a.onrender.com/results/matches/<matchId>`, con el `matchId` copiado de la tabla. Mostrar `combined`:
  - Carreras: tiempo perdido = penalizaciones + pits + choques, y la posición que habría tenido sin incidentes.
  - Fútbol: conversión de tiros y goles asistidos (cruza `PASS`, `SHOT` y `GOAL`).
- Código para mostrar: `analytics/src/main/scala/gamestats/analysis/Patterns.scala` (`streaks` con `foldLeft`, `comebacks`) y `RacingAnalyzer.scala` (`groupBy`, `collect`, `scanLeft`, composición de predicados con `and`/`or`).

## 5. Genericidad: un juego que el sistema no conoce (1 min)

Desde la carpeta `GameStats`:
```bash
curl.exe -X POST --data-binary "@docs/demo/cards-match.json" https://gamestats-ingest.onrender.com/events
```
A los ~3 s aparece en el tablero una pestaña **cards**: no hay analizador de cartas, pero recibe igual las **estadísticas genéricas** (eventos por tipo, duración, tiempo conectado) y la regla genérica **ABANDONO** (k02 se desconecta y no vuelve).

> Para agregar el juego completo basta con `games/cards.json` (acciones válidas) y un `object CardsAnalyzer extends GameAnalyzer` registrado en una línea. El núcleo no cambia.

Nota: el `matchId` `cards-demo-01` solo se puede usar una vez por arranque de ingest. Para repetirlo, cambiar el `matchId` en el JSON o reiniciar el servicio.

## 6. CI/CD y deploy (1 min)

https://github.com/mauroza/GameStats/actions → abrir la última ejecución:
- Go y Scala: formato, tests y build.
- **Prueba end-to-end**: levanta todo con docker compose y verifica que las 9 partidas lleguen a Scala.
- Imágenes publicadas en GHCR.
- Artefactos: binarios, jar y resultados JSON de la prueba e2e.

Render (https://dashboard.render.com): el Blueprint `render.yaml` define los dos servicios y **cada push a `main` se redespliega automáticamente**.

## Plan B: todo en local

Si Render no responde, en la carpeta `GameStats`:
```bash
docker compose up -d --build analytics ingest
```
Y usar los mismos comandos con `http://localhost:8080/events` como `-target` (o sin `-target`) y el tablero en http://localhost:9000. Los logs de ingest:
```bash
docker compose logs -f --tail 0 ingest
```

## Preguntas probables

| Pregunta | Respuesta corta |
|---|---|
| ¿Por qué Go para la ingestión? | Concurrencia barata (goroutines, channels), control explícito del flujo y del estado mutable de cada partida. |
| ¿Por qué Scala para el análisis? | Las estadísticas son transformaciones de colecciones inmutables: `filter`, `groupBy`, `fold`, sin estado compartido. Es más fácil razonar sobre ellas y probarlas. |
| ¿Cómo se comunican? | HTTP/JSON: Go hace `POST /matches` con la partida completa y ordenada; si Scala no responde, reintenta y guarda en un outbox en disco. |
| ¿Qué pasa si los eventos llegan desordenados? | Al llegar `MATCH_FINISHED`, Go espera un período de gracia, ordena por timestamp y rechaza lo que llegue tarde. |
| ¿Qué pasa si una partida nunca termina? | Se cierra por inactividad y se envía como `INCOMPLETE`. |
| ¿Por qué Render y no una VM? | Azure for Students solo permite 5 regiones y no había cuota ni capacidad para VMs pequeñas. Render construye desde los mismos Dockerfiles; `deploy/` deja lista la alternativa con VM. |
