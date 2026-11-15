import { api } from "../app.js";
import { renderChart } from "../components/chart.js";
import { renderCoreMap } from "../components/coremap.js";
import { esc, fmtDate, fmtDec, fmtInt, fmtMHz, fmtPct, fmtTimes, groupKindWord, metricText, othersLoaded, ratingHTML, valueWithRating } from "../texts.js";

const C = ["var(--series-1)", "var(--series-2)", "var(--series-3)", "var(--series-4)", "var(--series-5)"];

function boundaryLabel(run) {
  const kind = groupKindWord(run.cpu);
  return run.params.order === "far" ? `дальше — свой ${kind}` : `дальше — другой ${kind}`;
}

// The four test conditions at a glance: thread alone / both threads loaded,
// other cores idle / other cores loaded.
function matrix(s, full) {
  const r = s.ratings || {};
  const rows = [`<tr><th>Поток один, второй поток ядра свободен</th>
    <td class="num">100%</td><td class="num">${valueWithRating(fmtPct(s.full_alone_pct), r.full_single)}</td></tr>`];
  if (s.has_pair) {
    rows.push(`<tr><th>Оба потока ядра нагружены: скорость каждого</th>
      <td class="num">${valueWithRating(fmtPct(s.ht_per_thread_pct), r.ht)}</td>
      <td class="num">${valueWithRating(fmtPct(s.full_pair_pct), r.full_pair)}</td></tr>`);
    rows.push(`<tr><th>Оба потока ядра вместе</th>
      <td class="num">${fmtPct(s.ht_total_pct)}</td><td class="num">${fmtPct(s.full_pair_sum_pct)}</td></tr>`);
  }
  return `<div class="scroll"><table class="matrix"><thead><tr><th></th>
    <th class="num">Остальные ядра свободны</th><th class="num">${esc(full)}</th></tr></thead>
    <tbody>${rows.join("")}</tbody></table></div>
    <p class="muted small">100% — скорость потока, когда он один на пустой машине${s.baseline_tps > 0 ? `: ${fmtInt(s.baseline_tps)} тиков/с` : ""}.
      ${s.has_pair ? "«Оба потока вместе» — сумма двух потоков: 170% значит, что ядро с двумя нагруженными потоками делает работу 1,7 потока." : "HT выключен или недоступен: второй поток не тестировался."}</p>`;
}

// Every core as a pair of its threads, in percent or in ticks per second.
function pairTable(s, full, unit) {
  const val = (p, base) => {
    if (p == null || p < 0) return "—";
    return unit === "tps" ? (base > 0 ? fmtInt((p / 100) * base) : "—") : fmtPct(p);
  };
  const idle = (base) => {
    if (!(base > 0)) return "—";
    return unit === "tps" ? fmtInt(base) : `100% <span class="muted small">${fmtInt(base)}&nbsp;тик/с</span>`;
  };
  const rows = s.cores.map((c) => {
    const two = c.cpu_b >= 0;
    const span = two ? ' rowspan="2"' : "";
    const notes = [c.weak ? "медленное" : "", c.drift ? `дрейф ${Math.round(c.drift_pct)}%` : "", c.noisy ? "помехи" : ""].filter(Boolean).join(", ");
    const baseB = c.base_b_tps > 0 ? c.base_b_tps : c.base_tps;
    return `<tr class="first"><td${span}>${c.core}</td><td>CPU ${c.cpu_a}</td>
        <td class="num grp">${idle(c.base_tps)}</td><td class="num">${val(c.ht_a_pct, c.base_tps)}</td><td class="num"${span}>${val(c.ht_total_pct, c.base_tps)}</td>
        <td class="num grp">${val(c.full_single_pct, c.base_tps)}</td><td class="num">${val(c.full_pair_a_pct, c.base_tps)}</td><td class="num"${span}>${val(c.full_pair_sum_pct, c.base_tps)}</td>
        <td${span} class="grp">${notes}</td></tr>` +
      (two ? `<tr><td>CPU ${c.cpu_b}</td>
        <td class="num grp">${idle(c.base_b_tps)}</td><td class="num">${val(c.ht_b_pct, baseB)}</td>
        <td class="num grp">${val(c.full_solo_b_pct, baseB)}</td><td class="num">${val(c.full_pair_b_pct, baseB)}</td></tr>` : "");
  }).join("");
  return `<div class="scroll"><table class="compact pairs"><thead>
    <tr><th rowspan="2">Ядро</th><th rowspan="2">Поток</th><th colspan="3" class="grp">Остальные ядра свободны</th>
      <th colspan="3" class="grp">${esc(full)}</th><th rowspan="2" class="grp">Заметки</th></tr>
    <tr><th class="num grp">один</th><th class="num">сосед нагружен</th><th class="num">оба вместе</th>
      <th class="num grp">один</th><th class="num">сосед нагружен</th><th class="num">оба вместе</th></tr>
    </thead><tbody>${rows}</tbody></table></div>`;
}

// nodeText names the two node lines depending on how neighbour cores were loaded.
function nodeText(bg) {
  if (bg === 2) {
    return {
      iso: "сервер с целым ядром", packed: "по два сервера на ядро",
      note: "Нода заполняется по два сервера на ядро. Оранжевая линия — скорость каждого сервера на такой ноде. Синяя — сервер, которому отдано ядро целиком (его второй поток свободен), среди таких соседей.",
    };
  }
  if (bg === 1) {
    return {
      iso: "по одному серверу на ядро", packed: "два сервера на одном ядре",
      note: "Нода заполняется по одному серверу на ядро. Синяя линия — скорость каждого сервера на такой ноде. Оранжевая — два сервера на одном ядре среди таких соседей.",
    };
  }
  return {
    iso: "по одному серверу на ядро", packed: "по два сервера на ядро",
    note: "Синяя линия: на каждом занятом ядре один сервер, вторые потоки свободны. Оранжевая: на каждом занятом ядре два сервера, по одному на поток.",
  };
}

function nodeSeries(node, text) {
  const xs = [...new Set([...node.isolated, ...node.packed].map((p) => p.servers))].sort((a, b) => a - b);
  const line = (points, name, color) => {
    const by = new Map(points.map((p) => [p.servers, p]));
    return {
      name, color,
      values: xs.map((x) => (by.has(x) ? by.get(x).per_server_pct : -1)),
      labels: xs.map((x) => {
        const p = by.get(x);
        if (!p) return "";
        return `${fmtPct(p.per_server_pct)}${p.mhz > 0 ? ` · ${fmtMHz(p.mhz)}` : ""}`;
      }),
    };
  };
  const series = [line(node.isolated, text.iso, C[0])];
  if (node.packed.length) series.push(line(node.packed, text.packed, C[1]));
  return { xs, series };
}

function densityTable(s, text) {
  const d = s.node.density || [];
  const iso = d.some((x) => x.isolated >= 0);
  const packed = d.some((x) => x.packed >= 0);
  if (!iso && !packed) return "";
  const n = (v) => (v > 0 ? `до ${v}` : "ни одного");
  return `<h3 class="sub-h">Сколько серверов помещается на ноду</h3>
    <div class="scroll"><table class="compact narrow"><thead><tr><th>Чтобы каждый сервер работал</th>
      ${iso ? `<th class="num">${esc(text.iso)}</th>` : ""}${packed ? `<th class="num">${esc(text.packed)}</th>` : ""}</tr></thead><tbody>
    ${d.map((x) => `<tr><td>не медленнее ${x.min_pct}%</td>${iso ? `<td class="num">${n(x.isolated)}</td>` : ""}${packed ? `<td class="num">${n(x.packed)}</td>` : ""}</tr>`).join("")}
    </tbody></table></div>`;
}

function nodeTable(node, text, hasFreq) {
  const xs = [...new Set([...node.isolated, ...node.packed].map((p) => p.servers))].sort((a, b) => a - b);
  const iso = new Map(node.isolated.map((p) => [p.servers, p]));
  const pk = new Map(node.packed.map((p) => [p.servers, p]));
  const cells = (p) => (p
    ? `<td class="num grp">${fmtPct(p.per_server_pct)}</td>${hasFreq ? `<td class="num">${fmtMHz(p.mhz)}</td>` : ""}`
    : `<td class="num muted grp">—</td>${hasFreq ? '<td class="num muted">—</td>' : ""}`);
  const span = hasFreq ? 2 : 1;
  const sub = `<th class="num grp">скорость сервера</th>${hasFreq ? '<th class="num">частота</th>' : ""}`;
  return `<div class="scroll"><table class="compact"><thead>
    <tr><th rowspan="2">Серверов на ноде</th><th colspan="${span}" class="grp">${esc(text.iso)}</th><th colspan="${span}" class="grp">${esc(text.packed)}</th></tr>
    <tr>${sub}${sub}</tr></thead><tbody>
    ${xs.map((x) => `<tr><td>${x}</td>${cells(iso.get(x))}${cells(pk.get(x))}</tr>`).join("")}
    </tbody></table></div>`;
}

function threadNames(run, core) {
  const t = core == null ? null : run.cpu.cores.find((c) => c.id === core)?.threads;
  const a = t ? `CPU ${t[0]}` : "1-й поток";
  const b = t && t.length > 1 ? `CPU ${t[1]}` : "2-й поток";
  return { a, b };
}

function threadSeries(curve, names) {
  const all = [
    { name: `${names.a} один`, color: C[0], values: curve.single, noisy: curve.noisy },
    { name: `${names.b} один`, color: C[1], values: curve.solo_b },
    { name: `${names.a}, сосед нагружен`, color: C[2], values: curve.pair_a, noisy: curve.noisy },
    { name: `${names.b}, сосед нагружен`, color: C[3], values: curve.pair_b, noisy: curve.noisy },
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
    <th class="num">${esc(names.a)}, сосед нагружен</th><th class="num">${esc(names.b)}, сосед нагружен</th>
    <th class="num">оба вместе</th>
    ${hasFreq ? '<th class="num">частота, один поток</th><th class="num">частота, оба потока</th>' : ""}
    </tr></thead><tbody>${rows}</tbody></table></div>`;
}

function extraTable(s) {
  const f = s.has_freq;
  const rows = s.cores.map((c) => `<tr><td>${c.core}</td>
    <td class="num">${fmtPct(c.solo_pct)}</td><td class="num">${fmtTimes(c.lag_ratio)}</td>
    ${f ? `<td class="num">${fmtMHz(c.mhz_idle)}</td><td class="num">${fmtMHz(c.mhz_full_single)}</td><td class="num">${fmtMHz(c.mhz_full_pair)}</td>` : ""}</tr>`).join("");
  return `<div class="scroll"><table class="compact"><thead><tr><th>Ядро</th>
    <th class="num">Скорость ядра относительно остальных</th><th class="num">Лаги при полной загрузке</th>
    ${f ? '<th class="num">Частота, пусто</th><th class="num">Частота, загружено, поток один</th><th class="num">Частота, загружено, оба потока</th>' : ""}
    </tr></thead><tbody>${rows}</tbody></table></div>`;
}

export async function renderRun(app, id) {
  const { run, summary: s } = await api(`/api/runs/${id}`);
  const p = run.params;
  const node = s.node;
  const kind = groupKindWord(run.cpu);
  const full = othersLoaded(p.bg_threads);
  const text = nodeText(p.bg_threads);
  const warnings = [...(s.warnings || []), ...(run.warnings || [])];
  const multiL3 = run.cpu.l3_groups.length > 1;
  const orderNote = multiL3
    ? `Ядра занимаются в таком порядке: сначала ${p.order === "far" ? "чужой" : "свой"} ${kind}, потом ${p.order === "far" ? "свой" : "другой"}.`
    : "";
  const facts = [];
  if (s.lag_ratio > 0) {
    facts.push(`<span>Лаги при полной загрузке: самые долгие тики (1 из 100) в <b>${fmtTimes(s.lag_ratio).replace("×", "")}</b> раза длиннее, чем на пустой машине ${ratingHTML(s.ratings.lag)}</span>`);
  }
  if (s.has_freq) {
    facts.push(`<span>Частота ядра: <b>${fmtMHz(s.mhz_idle)}</b> на пустой машине, <b>${fmtMHz(s.mhz_full_single)}</b> при полной загрузке (поток один)${s.has_pair ? `, <b>${fmtMHz(s.mhz_full_pair)}</b> (оба потока)` : ""}</span>`);
  }

  app.innerHTML = `
    <h1>${esc(run.label || run.hostname)}${run.partial ? ' <span class="badge">прерван</span>' : ""}</h1>
    <p class="sub">${esc(run.cpu.model)} · профиль <b>${esc(p.profile)}</b> · ${fmtDate(run.started_at)}</p>

    <div class="panel">
      <h2 class="in">Сводка: скорость потока ядра в четырёх условиях</h2>
      ${matrix(s, full)}
      <div class="facts">${facts.join("")}</div>
    </div>
    ${node.conclusions.length ? `<div class="panel conclusions"><b>Выводы</b><ul>${node.conclusions.map((c) => `<li>${esc(c)}</li>`).join("")}</ul></div>` : ""}
    ${warnings.length ? `<div class="warnings"><b>Обратите внимание</b><ul>${warnings.map((w) => `<li>${esc(w)}</li>`).join("")}</ul></div>` : ""}

    <h2>По парам: ядро и его второй поток</h2>
    <div class="panel">
      <div class="toolbar"><span>Показывать:</span>
        <div class="tabs" id="unit"><button data-unit="pct" class="active">проценты</button><button data-unit="tps">тики в секунду</button></div></div>
      <div id="pairs"></div>
      <p class="muted small">«Один» — второй поток этого ядра свободен. «Сосед нагружен» — занят. «Оба вместе» — сумма двух потоков.
        Проценты — от скорости этого же потока, когда он один на пустой машине.</p>
    </div>

    ${s.has_pair ? `<h2>Как меняется скорость потоков ядра, когда загружаются остальные ядра</h2>
    <div class="panel">
      <div class="toolbar"><label>Показать: <select id="core"></select></label></div>
      <div id="chart"></div>
      <p class="muted small">По горизонтали — сколько других ядер загружено${p.bg_threads === 2 ? " (у каждого оба потока)" : ""}. Линии «один»: второй поток ядра свободен. Линии «сосед нагружен»: заняты оба.
        Линии двух потоков одного ядра обычно почти совпадают: потоки равноправны. Точные цифры — в таблице ниже и в подсказке. Полый кружок и ⚠ — замер с помехами.</p>
      <details><summary>Таблица по числу загруженных ядер</summary><div id="threadtable"></div></details>
    </div>` : ""}

    <h2>Как падает скорость серверов, когда их на ноде становится больше</h2>
    <div class="panel">
      <div id="nodechart"></div>
      <p class="muted small">По горизонтали — сколько серверов на ноде (один сервер занимает один поток), по вертикали — скорость сервера. ${text.note} ${orderNote}</p>
      ${densityTable(s, text)}
      <details><summary>Таблица по числу серверов</summary>${nodeTable(node, text, s.has_freq)}</details>
    </div>

    <h2>Карта ядер</h2>
    <div class="panel">
      <div class="toolbar"><label>Цвет: <select id="metric">${Object.entries(metricText)
        .filter(([k]) => s.has_pair || !(k.startsWith("ht_") || k === "full_pair_pct"))
        .map(([k, v]) => `<option value="${k}">${v}</option>`).join("")}</select></label></div>
      <div id="map"></div>
    </div>

    <details><summary>Лаги и частота по ядрам, параметры теста</summary>
      <div class="panel" style="margin-top:10px">${extraTable(s)}
      <p class="muted small">Параметры: смесь ${esc(Object.entries(p.mix || {}).map(([k, v]) => `${k}=${v}`).join(", "))}
        · буфер памяти ${p.mem_size_mb} МБ · замер ${p.duration_ms / 1000} с · прогрев ${p.warmup_ms / 1000} с
        · шаг ${p.step} · у соседних ядер загружено потоков: ${p.bg_threads || "как у тестируемого"} · версия ${esc(run.tool_version)} · частота: ${esc(run.cpu.freq_source)}</p></div>
    </details>`;

  let unit = "pct";
  const drawPairs = () => { app.querySelector("#pairs").innerHTML = pairTable(s, full, unit); };
  app.querySelectorAll("#unit button").forEach((b) => b.addEventListener("click", () => {
    unit = b.dataset.unit;
    app.querySelectorAll("#unit button").forEach((x) => x.classList.toggle("active", x === b));
    drawPairs();
  }));
  drawPairs();

  const ns = nodeSeries(node, text);
  renderChart(app.querySelector("#nodechart"), {
    xs: ns.xs,
    series: ns.series,
    xTitle: "серверов на ноде",
    tooltipTitle: (x) => `серверов на ноде: ${x}`,
  });

  const coreSel = app.querySelector("#core");
  if (coreSel) {
    coreSel.innerHTML = `<option value="avg">в среднем по ядрам</option>` +
      s.cores.map((c) => `<option value="${c.core}">ядро ${c.core} (CPU ${c.cpu_a}${c.cpu_b >= 0 ? ` и ${c.cpu_b}` : ""})</option>`).join("");
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
