package gamestats

/** Tablero HTML que consume los endpoints JSON de /results. Es genérico:
  * las columnas de cada tabla de posiciones salen de los datos, así que un
  * juego nuevo aparece sin tocar esta página.
  */
object Dashboard:
  val html: String = """<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GameStats</title>
<style>
  :root {
    --bg: #f6f7f9; --surface: #ffffff; --text: #1b1f24; --muted: #5f6b7a;
    --border: #e1e5ea; --accent: #3557d4; --accent-soft: #e8edfc; --good: #1f8a4c;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --bg: #12151a; --surface: #1b1f26; --text: #e6e9ee; --muted: #9aa5b4;
      --border: #2b313b; --accent: #8aa4ff; --accent-soft: #232c45; --good: #5fd08f;
    }
  }
  * { box-sizing: border-box; }
  body { margin: 0; background: var(--bg); color: var(--text);
         font: 14px/1.45 system-ui, -apple-system, "Segoe UI", sans-serif; }
  header { padding: 20px 16px 8px; max-width: 1200px; margin: 0 auto;
           display: flex; align-items: baseline; gap: 12px; flex-wrap: wrap; }
  h1 { margin: 0; font-size: 22px; }
  h2 { font-size: 15px; margin: 0 0 10px; }
  .muted { color: var(--muted); }
  main { max-width: 1200px; margin: 0 auto; padding: 8px 16px 40px; display: grid; gap: 16px; }
  .kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 12px; }
  .card { background: var(--surface); border: 1px solid var(--border); border-radius: 10px; padding: 14px; min-width: 0; }
  .kpi b { display: block; font-size: 26px; font-variant-numeric: tabular-nums; }
  .tabs { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 12px; }
  .tabs button { border: 1px solid var(--border); background: transparent; color: var(--text);
                 border-radius: 999px; padding: 5px 12px; cursor: pointer; font: inherit; }
  .tabs button.on { background: var(--accent); border-color: var(--accent); color: #fff; }
  .chips { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 10px; }
  .chip { background: var(--accent-soft); color: var(--accent); border-radius: 6px; padding: 2px 8px; font-size: 12px; font-weight: 600; }
  .scroll { overflow-x: auto; }
  table { border-collapse: collapse; width: 100%; font-variant-numeric: tabular-nums; }
  th, td { text-align: left; padding: 6px 8px; border-bottom: 1px solid var(--border); white-space: nowrap; }
  th { color: var(--muted); font-weight: 600; font-size: 12px; }
  td.num, th.num { text-align: right; }
  a { color: var(--accent); cursor: pointer; text-decoration: none; }
  .two { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  @media (max-width: 800px) { .two { grid-template-columns: 1fr; } }
  ul.patterns { list-style: none; margin: 0; padding: 0; }
  ul.patterns li { padding: 6px 0; border-bottom: 1px solid var(--border); }
  #profile pre { margin: 0; white-space: pre-wrap; }
  .grid-kv { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 8px; }
  .grid-kv div { background: var(--bg); border-radius: 6px; padding: 6px 8px; }
  .grid-kv span { display: block; font-size: 12px; color: var(--muted); }
</style>
</head>
<body>
<header>
  <h1>GameStats</h1>
  <span class="muted">Análisis de eventos de partidas · se actualiza cada 5 s</span>
</header>
<main>
  <section class="kpis" id="kpis"></section>

  <section class="card">
    <h2>Tablas de posiciones</h2>
    <div class="tabs" id="tabs"></div>
    <div class="chips" id="rules"></div>
    <div class="scroll" id="board"></div>
  </section>

  <section class="card" id="profile" hidden></section>

  <div class="two">
    <section class="card">
      <h2>Patrones detectados</h2>
      <ul class="patterns" id="patterns"></ul>
    </section>
    <section class="card">
      <h2>Partidas analizadas</h2>
      <div class="scroll" id="matches"></div>
    </section>
  </div>
</main>
<script>
const state = { game: null };
const esc = s => String(s ?? "").replace(/[&<>"]/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]));
const label = k => k.replace(/([A-Z])/g, " $1").replace(/^./, c => c.toUpperCase());
const fmt = v => v === null || v === undefined ? "—"
  : typeof v === "boolean" ? (v ? "sí" : "no")
  : typeof v === "number" ? v.toLocaleString("es-CR") : esc(v);
const get = url => fetch(url).then(r => r.json());

async function refresh() {
  const [games, patterns, matches] = await Promise.all([
    get("/results/games"), get("/results/patterns"), get("/results/matches")]);

  const players = new Set(matches.length ? (await get("/results/players")).map(p => p.playerId) : []);
  document.getElementById("kpis").innerHTML = [
    ["Partidas", matches.length],
    ["Eventos", matches.reduce((a, m) => a + m.eventCount, 0)],
    ["Jugadores", players.size],
    ["Patrones", patterns.length],
  ].map(([k, v]) => `<div class="card kpi"><span class="muted">${k}</span><b>${fmt(v)}</b></div>`).join("");

  if (!state.game && games.length) state.game = games[0].gameId;
  document.getElementById("tabs").innerHTML = games.map(g =>
    `<button class="${g.gameId === state.game ? "on" : ""}" onclick="state.game='${esc(g.gameId)}';refresh()">
      ${esc(g.gameId)} (${g.matches})</button>`).join("") || '<span class="muted">Sin partidas todavía</span>';

  const g = games.find(x => x.gameId === state.game);
  document.getElementById("rules").innerHTML = g ? Object.entries(g.patternsByRule)
    .map(([r, n]) => `<span class="chip">${esc(r)} · ${n}</span>`).join("") : "";
  document.getElementById("board").innerHTML = g && g.leaderboard.length ? table(g.leaderboard) : "";

  document.getElementById("patterns").innerHTML = patterns.slice(-15).reverse().map(p =>
    `<li><span class="chip">${esc(p.rule)}</span> ${esc(p.description)}
     <div class="muted">${esc(p.matchId)}</div></li>`).join("") || '<li class="muted">Ninguno aún</li>';

  document.getElementById("matches").innerHTML = matches.length ? `<table>
    <tr><th>Partida</th><th>Juego</th><th>Estado</th><th>Ganador</th><th class="num">Eventos</th><th class="num">Patrones</th></tr>
    ${matches.slice().reverse().map(m => `<tr><td>${esc(m.matchId)}</td><td>${esc(m.gameId)}</td>
      <td>${esc(m.status)}</td><td>${fmt(m.winner)}</td><td class="num">${fmt(m.eventCount)}</td>
      <td class="num">${m.patterns.length}</td></tr>`).join("")}</table>` : "";
}

function table(rows) {
  const cols = Object.keys(rows[0].totals);
  return `<table><tr><th>#</th><th>Jugador</th>${cols.map(c => `<th class="num">${label(c)}</th>`).join("")}</tr>
    ${rows.map((r, i) => `<tr><td>${i + 1}</td>
      <td><a onclick="profile('${esc(r.playerId)}')">${esc(r.playerId)}</a></td>
      ${cols.map(c => `<td class="num">${fmt(r.totals[c])}</td>`).join("")}</tr>`).join("")}</table>`;
}

async function profile(id) {
  const p = await get("/results/players/" + encodeURIComponent(id));
  const el = document.getElementById("profile");
  el.hidden = false;
  el.innerHTML = `<h2>Jugador ${esc(id)} · ${p.matches} partidas
      <a style="float:right" onclick="document.getElementById('profile').hidden=true">cerrar</a></h2>
    ${Object.entries(p.games).map(([game, s]) => `<p class="muted">${esc(game)} · ${s.matches} partidas ·
      ${fmt(Math.round(s.connectedSec / 60))} min conectado</p>
      <div class="grid-kv">${Object.entries(s.totals).map(([k, v]) =>
        `<div><span>${label(k)}</span>${fmt(v)}</div>`).join("")}</div>`).join("")}
    ${p.patterns.length ? `<p class="muted">Patrones</p><ul class="patterns">${p.patterns.map(d =>
      `<li><span class="chip">${esc(d.rule)}</span> ${esc(d.description)}</li>`).join("")}</ul>` : ""}`;
  el.scrollIntoView({ behavior: "smooth", block: "nearest" });
}

refresh();
setInterval(refresh, 5000);
</script>
</body>
</html>
"""
