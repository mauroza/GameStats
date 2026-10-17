#!/usr/bin/env bash
# Prueba end-to-end: levanta el sistema con docker compose, espera a que los
# 3 simuladores terminen y verifica que todas las partidas llegaron a Scala.
# Uso: scripts/e2e.sh [directorio-de-resultados]
set -euo pipefail

OUT="${1:-e2e-results}"
EXPECTED=9 # 3 simuladores x 3 partidas
cd "$(dirname "$0")/.."
mkdir -p "$OUT"

cleanup() {
  docker compose logs --no-color > "$OUT/compose.log" 2>&1 || true
  docker compose down -v > /dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> Levantando el sistema (desde cero)"
docker compose down -v > /dev/null 2>&1 || true
docker compose up -d --build

echo "==> Esperando a los simuladores"
for _ in $(seq 120); do
  done_count=$(docker compose ps -a --format '{{.Service}} {{.State}}' | grep -c 'simulator-.* exited' || true)
  [ "$done_count" -eq 3 ] && break
  sleep 3
done
[ "$done_count" -eq 3 ] || { echo "Los simuladores no terminaron a tiempo"; exit 1; }

for s in simulator-racing simulator-combat simulator-football; do
  code=$(docker inspect -f '{{.State.ExitCode}}' "$(docker compose ps -aq "$s")")
  [ "$code" -eq 0 ] || { echo "$s terminó con código $code"; exit 1; }
done

echo "==> Esperando a que analytics reciba las $EXPECTED partidas"
for _ in $(seq 30); do
  analyzed=$(curl -fs localhost:9000/results/matches | grep -o '"matchId"' | wc -l || true)
  [ "$analyzed" -ge "$EXPECTED" ] && break
  sleep 2
done

curl -fs localhost:8080/stats > "$OUT/ingest-stats.json"
curl -fs localhost:9000/results/games > "$OUT/games.json"
curl -fs localhost:9000/results/patterns > "$OUT/patterns.json"
curl -fs localhost:9000/results/matches > "$OUT/matches.json"

echo "==> Ingest: $(cat "$OUT/ingest-stats.json")"
echo "==> Partidas analizadas: $analyzed / $EXPECTED"

send_errors=$(grep -o '"sendErrors": *[0-9]*' "$OUT/ingest-stats.json" | grep -o '[0-9]*$')
[ "$analyzed" -ge "$EXPECTED" ] || { echo "FALLO: faltan partidas en analytics"; exit 1; }
[ "$send_errors" -eq 0 ] || { echo "FALLO: ingest tuvo $send_errors errores de envío"; exit 1; }
echo "==> OK"
