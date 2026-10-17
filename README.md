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

## Equipo

- Mauro — programación
- Melanny — documentación
