import { api } from "../app.js";
import { renderChart } from "../components/chart.js";
import { esc, fmtDate, fmtInt, fmtPct, fmtTimes, paramsShort, valueWithRating, variantDims, variantName } from "../texts.js";

const colors = ["var(--series-1)", "var(--series-2)", "var(--series-3)", "var(--series-4)"];

const paramNames = {
  profile: "профиль",
  mix: "смесь нагрузки",
  mem_size_mb: "размер буфера памяти",
  duration_ms: "длительность замера",
  bg_threads: "нагрузка на соседние ядра (--bg-threads)",
  order: "порядок загрузки",
  tick_rate: "тикрейт в тесте стабильности",
  tick_load_pct: "нагрузка сервера в тесте стабильности (--tick-load)",
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
  const label = (r) => r.run.label || r.run.hostname;
  // runs of one node are told apart by the settings that differ between them
  const dims = variantDims.filter((d) => new Set(runs.map((r) => d.value(r.run.params))).size > 1);
  const sameLabel = new Set(runs.map(label)).size < runs.length;
  const name = (r) => (sameLabel && dims.length ? `${label(r)} · ${variantName(r.run.params, dims)}` : label(r));
  const base0 = runs[0].summary.baseline_tps;
  const diff = differences(runs);
  // tick rate scenarios present in any of the runs, in test order
  const stabRows = [];
  for (const r of runs) {
    for (const x of r.summary.stab?.rows || []) {
      if (!stabRows.some(([n]) => n === x.name)) stabRows.push([x.name, x.title]);
    }
  }
  const stabRow = (r, name) => (r.summary.stab?.rows || []).find((x) => x.name === name);

  const row = (title, cell) => `<tr><th>${title}</th>${runs.map((r, i) => `<td class="num">${cell(r, i)}</td>`).join("")}</tr>`;
  const table = `<div class="panel scroll"><table>
    <thead><tr><th></th>${runs.map((r, i) => `<th class="num"><span class="legend"><span><span class="sw" style="border-color:${colors[i]}"></span><a href="#/run/${r.run.run_id}">${esc(name(r))}</a></span></span></th>`).join("")}</tr></thead>
    <tbody>
      ${row("Процессор", (r) => esc(r.run.cpu.model))}
      ${row("Профиль", (r) => esc(r.run.params.profile))}
      ${row("Параметры", (r) => `<span class="small wrap">${esc(paramsShort(r.run.params))}</span>`)}
      ${row("Дата", (r) => fmtDate(r.run.started_at))}
      ${row("Скорость одного потока, тиков/с", (r) => (r.summary.baseline_tps > 0 ? fmtInt(r.summary.baseline_tps) : "—"))}
      ${row(`Скорость ядра относительно «${esc(name(runs[0]))}»`, (r) => fmtPct(base0 > 0 ? (100 * r.summary.baseline_tps) / base0 : -1))}
      ${row("Оба потока ядра нагружены: 1-й и 2-й поток", (r) => (r.summary.has_pair ? valueWithRating(`${fmtPct(r.summary.ht_a_pct)} и ${fmtPct(r.summary.ht_b_pct)}`, r.summary.ratings.ht) : "HT нет"))}
      ${row("Поток один, остальные ядра нагружены", (r) => valueWithRating(fmtPct(r.summary.full_alone_pct), r.summary.ratings.full_single))}
      ${row("Оба потока нагружены, остальные ядра нагружены: каждый", (r) => (r.summary.has_pair ? valueWithRating(fmtPct(r.summary.full_pair_pct), r.summary.ratings.full_pair) : "HT нет"))}
      ${row("Лаги при полной загрузке", (r) => valueWithRating(fmtTimes(r.summary.lag_ratio), r.summary.ratings.lag))}
      ${stabRows.map(([name, title]) => row(`Тикрейт, худшая секунда / тик 1 из 100: ${esc(title)}`, (r) => {
        const x = stabRow(r, name);
        return x ? valueWithRating(`${fmtPct(x.worst_sec_pct)} / ${fmtPct(x.work_p99_pct)}`, x.rating) : "—";
      })).join("")}
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
