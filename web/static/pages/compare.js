import { api } from "../app.js";
import { renderChart } from "../components/chart.js";
import { esc, fmtDate, fmtInt, fmtPct, fmtTimes, valueWithRating } from "../texts.js";

const colors = ["var(--series-1)", "var(--series-2)", "var(--series-3)", "var(--series-4)"];

const paramNames = {
  profile: "профиль",
  mix: "смесь нагрузки",
  mem_size_mb: "размер буфера памяти",
  duration_ms: "длительность замера",
  bg_threads: "потоки фоновых ядер",
  order: "порядок загрузки",
  kernels_version: "версия нагрузки",
};

function paramValue(run, key) {
  if (key === "kernels_version") return String(run.kernels_version);
  const v = run.params[key];
  if (key === "mix") return Object.entries(v || {}).sort().map(([k, n]) => `${k}=${n}`).join(",");
  return String(v);
}

function differences(runs) {
  return Object.keys(paramNames).filter((k) => new Set(runs.map((r) => paramValue(r.run, k))).size > 1);
}

// mergePoints puts node points of several runs on one axis of server counts; gaps are -1.
function mergePoints(lists) {
  const xs = [...new Set(lists.flatMap((l) => l.map((p) => p.servers)))].sort((a, b) => a - b);
  return {
    xs,
    pick: (l) => xs.map((x) => l.find((p) => p.servers === x)?.per_server_pct ?? -1),
  };
}

export async function renderCompare(app, ids) {
  const runs = await Promise.all(ids.slice(0, 4).map((id) => api(`/api/runs/${id}`)));
  const name = (r) => r.run.label || r.run.hostname;
  const base0 = runs[0].summary.baseline_tps;
  const diff = differences(runs);
  const fit = (r, field) => {
    const d = (r.summary.node.density || []).find((x) => x.min_pct === 90);
    if (!d) return "—";
    if (d[field] < 0) return "—";
    return d[field] > 0 ? `до ${d[field]}` : "ни одного";
  };

  const row = (title, cell) => `<tr><th>${title}</th>${runs.map((r, i) => `<td class="num">${cell(r, i)}</td>`).join("")}</tr>`;
  const table = `<div class="panel scroll"><table>
    <thead><tr><th></th>${runs.map((r, i) => `<th class="num"><span class="legend"><span><span class="sw" style="border-color:${colors[i]}"></span><a href="#/run/${r.run.run_id}">${esc(name(r))}</a></span></span></th>`).join("")}</tr></thead>
    <tbody>
      ${row("Процессор", (r) => esc(r.run.cpu.model))}
      ${row("Профиль", (r) => esc(r.run.params.profile))}
      ${row("Дата", (r) => fmtDate(r.run.started_at))}
      ${row("Скорость одного потока, тиков/с", (r) => (r.summary.baseline_tps > 0 ? fmtInt(r.summary.baseline_tps) : "—"))}
      ${row(`Скорость ядра относительно «${esc(name(runs[0]))}»`, (r) => fmtPct(base0 > 0 ? (100 * r.summary.baseline_tps) / base0 : -1))}
      ${row("Оба потока ядра нагружены: 1-й и 2-й поток", (r) => (r.summary.has_pair ? valueWithRating(`${fmtPct(r.summary.ht_a_pct)} и ${fmtPct(r.summary.ht_b_pct)}`, r.summary.ratings.ht) : "HT нет"))}
      ${row("Поток один, остальные ядра нагружены", (r) => valueWithRating(fmtPct(r.summary.full_alone_pct), r.summary.ratings.full_single))}
      ${row("Оба потока нагружены, остальные ядра нагружены: каждый", (r) => (r.summary.has_pair ? valueWithRating(fmtPct(r.summary.full_pair_pct), r.summary.ratings.full_pair) : "HT нет"))}
      ${row("Помещается серверов по одному на ядро, каждый не медленнее 90%", (r) => fit(r, "isolated"))}
      ${row("Помещается серверов по два на ядро, каждый не медленнее 90%", (r) => (r.summary.has_pair ? fit(r, "packed") : "HT нет"))}
      ${row("Лаги при полной загрузке", (r) => valueWithRating(fmtTimes(r.summary.lag_ratio), r.summary.ratings.lag))}
    </tbody></table></div>`;

  app.innerHTML = `<h1>Сравнение</h1>
    <p class="sub">Проценты — от скорости потока своей же машины на пустой машине. Абсолютную скорость ядер сравнивает строка «Скорость ядра относительно…».</p>
    ${diff.length ? `<div class="notice">Прогоны сделаны с разными настройками — сравнивать напрямую нельзя: различаются ${diff.map((k) => paramNames[k]).join(", ")}.</div>` : ""}
    ${table}
    <h2>Сервер один на своём ядре: скорость по числу серверов на ноде</h2>
    <div class="panel"><div id="c1"></div></div>
    <h2>Сервер делит ядро со вторым: скорость по числу серверов на ноде</h2>
    <div class="panel"><div id="c2"></div></div>`;

  const draw = (el, field) => {
    const lists = runs.map((r) => r.summary.node[field]);
    const m = mergePoints(lists);
    renderChart(el, {
      xs: m.xs,
      series: runs.map((r, i) => ({ name: name(r), color: colors[i], values: m.pick(lists[i]) })),
      xTitle: "серверов на ноде",
      tooltipTitle: (x) => `серверов на ноде: ${x}`,
    });
  };
  draw(app.querySelector("#c1"), "isolated");
  if (runs.some((r) => r.summary.has_pair)) draw(app.querySelector("#c2"), "packed");
  else app.querySelector("#c2").innerHTML = '<p class="muted">HT нет ни на одной из машин.</p>';
}
