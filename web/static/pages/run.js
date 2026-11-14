import { api } from "../app.js";
import { renderChart } from "../components/chart.js";
import { renderCoreMap } from "../components/coremap.js";
import { esc, fmtDate, fmtDec, fmtInt, fmtMHz, fmtPct, fmtTimes, groupKindWord, metricText, ratingHTML } from "../texts.js";

const C = ["var(--series-1)", "var(--series-2)", "var(--series-3)", "var(--series-4)", "var(--series-5)"];

function boundaryLabel(run) {
  const kind = groupKindWord(run.cpu);
  return run.params.order === "far" ? `дальше — свой ${kind}` : `дальше — другой ${kind}`;
}

function card(title, value, explain, rating) {
  return `<div class="card"><div class="title">${title}</div><div class="value">${value}</div>
    <div class="explain">${explain}</div>${ratingHTML(rating)}</div>`;
}

function cards(s, uniform) {
  const r = s.ratings || {};
  const out = [];
  const others = uniform ? "остальные ядра заняты так же" : "остальные ядра загружены";
  if (s.has_pair && s.ht_per_thread_pct >= 0) {
    out.push(card("Оба потока одного ядра нагружены, остальные ядра свободны",
      `${fmtPct(s.ht_a_pct)} и ${fmtPct(s.ht_b_pct)}`,
      `Скорость 1-го и 2-го потока. Вместе ядро даёт ${fmtPct(s.ht_total_pct)} от одного потока: второй поток добавляет ${Math.round(s.ht_total_pct - 100)}%.`, r.ht));
  } else {
    out.push(card("Оба потока одного ядра нагружены", "—", "HT выключен или недоступен, этот режим не тестировался.", null));
  }
  out.push(card(uniform ? "По одному серверу на ядро, нода забита" : "Сервер один на ядре, остальные ядра загружены", fmtPct(s.full_single_pct),
    `Скорость каждого сервера: на его ядре он один, второй поток свободен, ${others}.`, r.full_single));
  if (s.has_pair) {
    out.push(card(uniform ? "По два сервера на ядро, нода забита" : "Два сервера на ядре, остальные ядра загружены", fmtPct(s.full_pair_pct),
      `Скорость каждого сервера: на его ядре заняты оба потока, ${others}.`, r.full_pair));
  }
  if (s.lag_ratio > 0) {
    out.push(card("Лаги при полной загрузке", `в ${fmtTimes(s.lag_ratio).replace("×", " раза")}`,
      "Во столько раз самые долгие тики (1 из 100) длиннее, чем на пустой машине.", r.lag));
  }
  return out.join("");
}

// nodeSeries puts both placements on one axis of server counts.
function nodeSeries(node) {
  const xs = [...new Set([...node.isolated, ...node.packed].map((p) => p.servers))].sort((a, b) => a - b);
  const line = (points, name, color) => {
    const by = new Map(points.map((p) => [p.servers, p]));
    return {
      name, color,
      values: xs.map((x) => (by.has(x) ? by.get(x).per_server_pct : -1)),
      labels: xs.map((x) => {
        const p = by.get(x);
        if (!p) return "";
        const mhz = p.mhz > 0 ? ` · ${fmtMHz(p.mhz)}` : "";
        return `${fmtPct(p.per_server_pct)} · нода как ${fmtDec(p.total_eq)} сервера${mhz}`;
      }),
    };
  };
  const series = [line(node.isolated, node.uniform ? "по одному серверу на ядро" : "сервер один на своём ядре", C[0])];
  if (node.packed.length) series.push(line(node.packed, node.uniform ? "по два сервера на ядро" : "сервер делит ядро со вторым", C[1]));
  return { xs, series };
}

function densityTable(s) {
  const d = s.node.density || [];
  if (!d.length) return "";
  const n = (v) => (v > 0 ? `до ${v}` : "ни одного");
  return `<h3 class="sub-h">Сколько серверов помещается на ноду</h3>
    <div class="scroll"><table class="compact"><thead><tr><th>Чтобы каждый сервер работал</th>
      <th class="num">по одному на ядро</th>${s.has_pair ? '<th class="num">по два на ядро</th>' : ""}</tr></thead><tbody>
    ${d.map((x) => `<tr><td>не медленнее ${x.min_pct}%</td><td class="num">${n(x.isolated)}</td>${s.has_pair ? `<td class="num">${n(x.packed)}</td>` : ""}</tr>`).join("")}
    </tbody></table></div>`;
}

function nodeTable(node, hasFreq) {
  const xs = [...new Set([...node.isolated, ...node.packed].map((p) => p.servers))].sort((a, b) => a - b);
  const iso = new Map(node.isolated.map((p) => [p.servers, p]));
  const pk = new Map(node.packed.map((p) => [p.servers, p]));
  const cells = (p) => (p
    ? `<td class="num">${fmtPct(p.per_server_pct)}</td><td class="num">${fmtDec(p.total_eq)}</td>${hasFreq ? `<td class="num">${fmtMHz(p.mhz)}</td>` : ""}`
    : `<td class="num muted">—</td><td class="num muted">—</td>${hasFreq ? '<td class="num muted">—</td>' : ""}`);
  const span = hasFreq ? 3 : 2;
  const sub = `<th class="num">каждый сервер</th><th class="num">нода в сумме, серверов</th>${hasFreq ? '<th class="num">частота</th>' : ""}`;
  return `<div class="scroll"><table class="compact"><thead>
    <tr><th rowspan="2">Серверов на ноде</th><th colspan="${span}" class="num">по одному на ядро</th><th colspan="${span}" class="num">по два на ядро</th></tr>
    <tr>${sub}${sub}</tr></thead><tbody>
    ${xs.map((x) => `<tr><td>${x}</td>${cells(iso.get(x))}${cells(pk.get(x))}</tr>`).join("")}
    </tbody></table></div>`;
}

function threadNames(run, core) {
  const t = core == null ? null : run.cpu.cores.find((c) => c.id === core)?.threads;
  const a = t ? `1-й поток (CPU ${t[0]})` : "1-й поток";
  const b = t && t.length > 1 ? `2-й поток (CPU ${t[1]})` : "2-й поток";
  return { a, b };
}

function threadSeries(curve, names) {
  const all = [
    { name: `${names.a} один`, color: C[0], values: curve.single, noisy: curve.noisy },
    { name: `${names.b} один`, color: C[1], values: curve.solo_b },
    { name: `${names.a}, второй нагружен`, color: C[2], values: curve.pair_a, noisy: curve.noisy },
    { name: `${names.b}, первый нагружен`, color: C[3], values: curve.pair_b, noisy: curve.noisy },
    { name: "оба потока вместе", color: C[4], values: curve.pair_sum },
  ];
  return all.filter((s) => s.values.some((v) => v >= 0));
}

function threadTable(curve, names, hasFreq) {
  const cell = (v) => `<td class="num">${fmtPct(v)}</td>`;
  const rows = curve.loaded.map((k, i) => `<tr><td>${k}${curve.noisy[i] ? " ⚠" : ""}</td>
    ${cell(curve.single[i])}${cell(curve.solo_b[i])}${cell(curve.pair_a[i])}${cell(curve.pair_b[i])}${cell(curve.pair_sum[i])}
    ${hasFreq ? `<td class="num">${fmtMHz(curve.mhz[i])}</td><td class="num">${fmtMHz(curve.pair_mhz[i])}</td>` : ""}</tr>`).join("");
  return `<div class="scroll"><table class="compact"><thead><tr><th>Загружено других ядер</th>
    <th class="num">${esc(names.a)} один</th><th class="num">${esc(names.b)} один</th>
    <th class="num">${esc(names.a)}, второй нагружен</th><th class="num">${esc(names.b)}, первый нагружен</th>
    <th class="num">оба вместе</th>
    ${hasFreq ? '<th class="num">частота, один поток</th><th class="num">частота, оба потока</th>' : ""}
    </tr></thead><tbody>${rows}</tbody></table></div>`;
}

function detailsTable(run, s) {
  const f = s.has_freq;
  const threads = new Map(run.cpu.cores.map((c) => [c.id, c.threads.join("/")]));
  const rows = s.cores.map((c) => `<tr>
    <td>${c.core}</td><td>${threads.get(c.core) || ""}</td>
    <td class="num">${c.base_tps > 0 ? fmtInt(c.base_tps) : "—"}</td>
    <td class="num">${fmtPct(c.solo_pct)}</td>
    <td class="num">${fmtPct(c.ht_a_pct)}</td>
    <td class="num">${fmtPct(c.ht_b_pct)}</td>
    <td class="num">${fmtPct(c.ht_total_pct)}</td>
    <td class="num">${fmtPct(c.full_single_pct)}</td>
    <td class="num">${fmtPct(c.full_pair_pct)}</td>
    <td class="num">${fmtTimes(c.lag_ratio)}</td>
    ${f ? `<td class="num">${fmtMHz(c.mhz_idle)}</td><td class="num">${fmtMHz(c.mhz_full_single)}</td><td class="num">${fmtMHz(c.mhz_full_pair)}</td>` : ""}
    <td>${[c.weak ? "медленное" : "", c.drift ? `дрейф ${Math.round(c.drift_pct)}%` : "", c.noisy ? "помехи" : ""].filter(Boolean).join(", ")}</td>
  </tr>`).join("");
  return `<div class="scroll"><table class="compact"><thead><tr>
    <th>Ядро</th><th>Потоки</th><th class="num">Тиков/с, один поток</th><th class="num">Скорость ядра</th>
    <th class="num">1-й поток, 2-й нагружен</th><th class="num">2-й поток, 1-й нагружен</th><th class="num">Оба вместе</th>
    <th class="num">Нода забита, 1 на ядро</th><th class="num">Нода забита, 2 на ядро</th>
    <th class="num">Лаги</th>${f ? `<th class="num">Частота, пусто</th><th class="num">Частота, забита, 1 на ядро</th><th class="num">Частота, забита, 2 на ядро</th>` : ""}<th>Заметки</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

export async function renderRun(app, id) {
  const { run, summary: s } = await api(`/api/runs/${id}`);
  const p = run.params;
  const node = s.node;
  const kind = groupKindWord(run.cpu);
  const warnings = [...(s.warnings || []), ...(run.warnings || [])];
  const multiL3 = run.cpu.l3_groups.length > 1;
  const orderNote = multiL3
    ? `Ядра занимаются в таком порядке: сначала ${p.order === "far" ? "чужой" : "свой"} ${kind}, потом ${p.order === "far" ? "свой" : "другой"}.`
    : "";
  const forced = node.uniform ? "" : `<div class="notice">В этом прогоне соседние ядра всегда загружались ${p.bg_threads === 1 ? "одним потоком" : "обоими потоками"} (<code>--bg-threads ${p.bg_threads}</code>).
    Линии показывают сервер на тестируемом ядре среди таких соседей. Выводы и таблица плотности строятся для прогонов без этого флага.</div>`;

  app.innerHTML = `
    <h1>${esc(run.label || run.hostname)}${run.partial ? ' <span class="badge">прерван</span>' : ""}</h1>
    <p class="sub">${esc(run.cpu.model)} · профиль <b>${esc(p.profile)}</b> · ${fmtDate(run.started_at)}</p>
    <div class="cards">${cards(s, node.uniform)}</div>
    ${node.conclusions.length ? `<div class="panel conclusions"><b>Выводы</b><ul>${node.conclusions.map((c) => `<li>${esc(c)}</li>`).join("")}</ul></div>` : ""}
    <div class="facts">
      <span>Скорость одного потока на пустой машине: <b>${s.baseline_tps > 0 ? fmtInt(s.baseline_tps) : "—"}</b> тиков/с — это 100%</span>
      ${s.has_freq ? `<span>Частота ядра: <b>${fmtMHz(s.mhz_idle)}</b> на пустой машине, <b>${fmtMHz(s.mhz_full_single)}</b> на забитой ноде по одному серверу на ядро${s.has_pair ? `, <b>${fmtMHz(s.mhz_full_pair)}</b> по два` : ""}</span>` : ""}
    </div>
    ${warnings.length ? `<div class="warnings"><b>Обратите внимание</b><ul>${warnings.map((w) => `<li>${esc(w)}</li>`).join("")}</ul></div>` : ""}

    <h2>Как падает скорость серверов, когда их на ноде становится больше</h2>
    <div class="panel">
      ${forced}
      <div id="nodechart"></div>
      <p class="muted small">По горизонтали — сколько серверов на ноде, по вертикали — скорость каждого из них. 100% — сервер один на пустой машине.
        ${node.uniform ? "Синяя линия: на каждом занятом ядре один сервер, вторые потоки свободны. Оранжевая: на каждом занятом ядре два сервера, по одному на поток." : ""}
        ${orderNote} В подсказке — сколько работы делает вся нода, в серверах на пустой машине${s.has_freq ? ", и частота в МГц" : ""}.</p>
      ${densityTable(s)}
      <details><summary>Таблица по числу серверов</summary>${nodeTable(node, s.has_freq)}</details>
    </div>

    ${s.has_pair ? `<h2>Ядро и его второй поток (HT)</h2>
    <div class="panel">
      <div class="toolbar"><label>Показать: <select id="core"></select></label></div>
      <div id="chart"></div>
      <p class="muted small">Каждый поток по отдельности (сосед по ядру свободен), каждый поток при нагруженном соседе и оба вместе — в зависимости от того, сколько других ядер занято.
        100% — тот же поток один на пустой машине. «Оба вместе» — сумма двух потоков: 170% значит, что ядро с двумя нагруженными потоками делает работу 1,7 потока.
        ${node.uniform ? "Соседние ядра загружены так же, как тестируемое: одним потоком в замерах одного потока, двумя — в замерах обоих." : ""}
        Линии первого и второго потока обычно почти совпадают: потоки одного ядра равноправны. Полый кружок и ⚠ — замер с помехами.</p>
      <div id="threadtable"></div>
    </div>` : ""}

    <h2>Карта ядер</h2>
    <div class="panel">
      <div class="toolbar"><label>Цвет: <select id="metric">${Object.entries(metricText)
        .filter(([k]) => s.has_pair || !(k.startsWith("ht_") || k === "full_pair_pct"))
        .map(([k, v]) => `<option value="${k}">${v}</option>`).join("")}</select></label></div>
      <div id="map"></div>
    </div>

    <details><summary>Подробно по ядрам и параметры теста</summary>
      <div class="panel" style="margin-top:10px">${detailsTable(run, s)}
      <p class="muted small">Параметры: смесь ${esc(Object.entries(p.mix || {}).map(([k, v]) => `${k}=${v}`).join(", "))}
        · буфер памяти ${p.mem_size_mb} МБ · замер ${p.duration_ms / 1000} с · прогрев ${p.warmup_ms / 1000} с
        · шаг ${p.step} · соседние ядра: ${p.bg_threads ? `${p.bg_threads} пот.` : "как тестируемое"} · версия ${esc(run.tool_version)} · частота: ${esc(run.cpu.freq_source)}</p></div>
    </details>`;

  const ns = nodeSeries(node);
  renderChart(app.querySelector("#nodechart"), {
    xs: ns.xs,
    series: ns.series,
    xTitle: "серверов на ноде",
    tooltipTitle: (x) => `серверов на ноде: ${x}`,
  });

  const coreSel = app.querySelector("#core");
  if (coreSel) {
    coreSel.innerHTML = `<option value="avg">в среднем по ядрам</option>` +
      s.cores.map((c) => `<option value="${c.core}">ядро ${c.core}</option>`).join("");
  }
  const metricSel = app.querySelector("#metric");
  let selected = null;

  const draw = () => {
    if (coreSel) {
      const curve = selected == null ? s.avg : s.cores.find((c) => c.core === selected).curve;
      const names = threadNames(run, selected);
      renderChart(app.querySelector("#chart"), {
        xs: curve.loaded,
        series: threadSeries(curve, names),
        band: multiL3 ? { from: curve.boundary, label: boundaryLabel(run) } : null,
        xTitle: "загружено других ядер",
        tooltipTitle: (x) => `загружено других ядер: ${x}`,
      });
      app.querySelector("#threadtable").innerHTML = threadTable(curve, names, s.has_freq);
    }
    renderCoreMap(app.querySelector("#map"), run, s, metricSel.value, selected, (id) => {
      selected = id;
      if (coreSel) coreSel.value = String(id);
      draw();
    });
  };
  if (coreSel) {
    coreSel.addEventListener("change", () => {
      selected = coreSel.value === "avg" ? null : Number(coreSel.value);
      draw();
    });
  }
  metricSel.addEventListener("change", draw);
  draw();
}
