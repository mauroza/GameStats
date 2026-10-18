# Guion de la demo (~10 minutos)

Reemplazar `IP` por la IP pública de la VM. Si la VM falla, ver **Plan B** al final: todo funciona igual en local con `localhost`.

Los comandos son para **PowerShell** en Windows (por eso `curl.exe` y no `curl`).

## Antes de la presentación (15 min antes)

1. Dejar el sistema en limpio para que el tablero arranque vacío:
   ```bash
   ssh -i gamestats.pem azureuser@IP "cd gamestats && docker compose -f docker-compose.prod.yml down -v && docker compose -f docker-compose.prod.yml up -d analytics ingest"
   ```
2. Compilar el simulador una vez para que la demo no espere la compilación:
   ```bash
   cd simulator
   go build -o bin/simulator.exe .
   ```
3. Abrir en el navegador:
   - Pestaña 1: `http://IP:9000` (tablero; debe mostrar 0 partidas)
   - Pestaña 2: `http://IP:8080/stats`
   - Pestaña 3: https://github.com/mauroza/GameStats/actions
4. Abrir **4 terminales** en la carpeta `GameStats/simulator`:
   - Terminales 1, 2 y 3: un simulador cada una
   - Terminal 4: logs de ingest en la VM

## 1. Arquitectura (1 min)

Mostrar el diagrama del README y decirlo en una frase:

> Los simuladores representan el sistema externo de juegos. **Go** recibe los eventos, los valida, mantiene el estado de cada partida y los procesa en paralelo. Cuando una partida termina, se la envía completa a **Scala**, que calcula las estadísticas con programación funcional.

## 2. Concurrencia: tres simuladores a la vez (3 min)

Terminal 4, dejar corriendo los logs de ingest:
```bash
ssh -i gamestats.pem azureuser@IP "cd gamestats && docker compose -f docker-compose.prod.yml logs -f --tail 0 ingest"
```

Lanzar los tres **casi al mismo tiempo**, uno en cada terminal:

Terminal 1:
```bash
./bin/simulator.exe -game racing -matches 3 -seed 2 -interval 300ms -target http://IP:8080/events
```
Terminal 2:
```bash
./bin/simulator.exe -game combat -matches 3 -seed 1 -interval 200ms -target http://IP:8080/events
```
Terminal 3:
```bash
./bin/simulator.exe -game football -matches 3 -seed 2 -interval 120ms -target http://IP:8080/events
```

Mientras corren, explicar:
- Cada simulador es un **proceso aparte** (como pidió el profe). Cada partida corre en su propia **goroutine** y envía un evento por tick de `time.Ticker`.
- En la terminal 4 se ven las 9 partidas **abriéndose y cerrándose intercaladas**: el servidor HTTP atiende cada request en una goroutine, un **worker pool** valida, y un router reparte cada evento a la goroutine de su partida por **channels**.
- Refrescar `http://IP:8080/matches`: las partidas en curso con sus jugadores activos (el estado que mantiene Go).
- Código para mostrar si preguntan: `ingest/internal/matches/manager.go` (router + una goroutine por partida) y `ingest/internal/pipeline/pipeline.go` (worker pool).

Los simuladores tardan ~1 minuto en terminar.

## 3. Validación: eventos inválidos (1 min)

- `http://IP:8080/stats`: recibidos, válidos, rechazados y duplicados.
- `http://IP:8080/rejected`: cada rechazo con su motivo (sin `matchId`, timestamp roto, acción desconocida, `data` incompleta, duplicado, evento posterior al fin de la partida).
- Explicar que el simulador inyecta ~5 % de eventos inválidos **a propósito**, y que el número de rechazados coincide con lo que reporta cada simulador al terminar ("N inválidos inyectados").

## 4. Análisis en Scala: tablero (3 min)

`http://IP:9000`:
- Los KPI: partidas, eventos, jugadores y patrones.
- Pestañas por juego: **tabla de posiciones** (estadística agregada por jugador entre varias partidas).
- Clic en un jugador: perfil con totales y patrones.
- **Patrones detectados** (reglas que miran secuencias de eventos):
  - Carreras: **REMONTADA** (era de los dos últimos en la vuelta 1 y terminó en el podio).
  - Combate: **RACHA** (3 bajas en menos de 10 s sin morir), **CLUTCH** (último vivo contra 2 o más y gana la ronda), **ACE**.
  - Fútbol: **REMONTADA** (perdía por 2 y empató o ganó), **DOBLE_AMARILLA**.
- Detalle de una partida (estadística por partida y combinada), por ejemplo `http://IP:9000/results/matches/<matchId>`, con el `matchId` copiado de la tabla. Mostrar `combined`:
  - Carreras: tiempo perdido = penalizaciones + pits + choques, y la posición que habría tenido sin incidentes.
  - Fútbol: conversión de tiros y goles asistidos (cruza `PASS`, `SHOT` y `GOAL`).
- Código para mostrar: `analytics/src/main/scala/gamestats/analysis/Patterns.scala` (`streaks` con `foldLeft`, `comebacks`) y `RacingAnalyzer.scala` (`groupBy`, `collect`, `scanLeft`, composición de predicados con `and`/`or`).

## 5. Genericidad: un juego que el sistema no conoce (1 min)

Desde la carpeta `GameStats`:
```bash
curl.exe -X POST --data-binary "@docs/demo/cards-match.json" http://IP:8080/events
```
A los ~3 s aparece en el tablero una pestaña **cards**: no hay analizador de cartas, pero recibe igual las **estadísticas genéricas** (eventos por tipo, duración, tiempo conectado) y la regla genérica **ABANDONO** (k02 se desconecta y no vuelve).

> Para agregar el juego completo basta con `games/cards.json` (acciones válidas) y un `object CardsAnalyzer extends GameAnalyzer` registrado en una línea. El núcleo no cambia.

Nota: el `matchId` `cards-demo-01` solo se puede usar una vez por arranque de ingest. Para repetirlo, cambiar el `matchId` en el JSON.

## 6. CI/CD y deploy (1 min)

https://github.com/mauroza/GameStats/actions → abrir la última ejecución:
- Go y Scala: formato, tests y build.
- **Prueba end-to-end**: levanta todo con docker compose y verifica que las 9 partidas lleguen a Scala.
- Imágenes publicadas en GHCR.
- **Deploy a la VM** por SSH y verificación de `/health`.
- Artefactos: binarios, jar y resultados JSON de la prueba e2e.

## Plan B: todo en local

Si la VM no responde, en la carpeta `GameStats`:
```bash
docker compose up -d --build analytics ingest
```
Y usar los mismos comandos con `localhost` en lugar de `IP` (los simuladores pueden omitir `-target`). Los logs de ingest:
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
