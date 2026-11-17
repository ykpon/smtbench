import { api } from "../app.js";
import { renderChart } from "../components/chart.js";
import { renderCoreMap } from "../components/coremap.js";
import { esc, fmtDate, fmtFrac, fmtInt, fmtMHz, fmtMs, fmtPct, fmtTimes, groupKindWord, metricText, othersLoaded, paramsShort, paramsRows, commandLine, ratingHTML, valueWithRating, variantDims, variantName } from "../texts.js";

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
  }
  return `<div class="scroll"><table class="matrix"><thead><tr><th></th>
    <th class="num">Остальные ядра свободны</th><th class="num">${esc(full)}</th></tr></thead>
    <tbody>${rows.join("")}</tbody></table></div>
    <p class="muted small">100% — скорость потока, когда он один на пустой машине${s.baseline_tps > 0 ? `: ${fmtInt(s.baseline_tps)} тиков/с` : ""}.
      ${s.has_pair ? "" : "HT выключен или недоступен: второй поток не тестировался."}</p>`;
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
        <td class="num grp">${idle(c.base_tps)}</td><td class="num">${val(c.ht_a_pct, c.base_tps)}</td>
        <td class="num grp">${val(c.full_single_pct, c.base_tps)}</td><td class="num">${val(c.full_pair_a_pct, c.base_tps)}</td>
        <td${span} class="grp">${notes}</td></tr>` +
      (two ? `<tr><td>CPU ${c.cpu_b}</td>
        <td class="num grp">${idle(c.base_b_tps)}</td><td class="num">${val(c.ht_b_pct, baseB)}</td>
        <td class="num grp">${val(c.full_solo_b_pct, baseB)}</td><td class="num">${val(c.full_pair_b_pct, baseB)}</td></tr>` : "");
  }).join("");
  return `<div class="scroll"><table class="compact pairs"><thead>
    <tr><th rowspan="2">Ядро</th><th rowspan="2">Поток</th><th colspan="2" class="grp">Остальные ядра свободны</th>
      <th colspan="2" class="grp">${esc(full)}</th><th rowspan="2" class="grp">Заметки</th></tr>
    <tr><th class="num grp">один</th><th class="num">сосед нагружен</th>
      <th class="num grp">один</th><th class="num">сосед нагружен</th></tr>
    </thead><tbody>${rows}</tbody></table></div>`;
}

// paramsBlock shows the settings of the run and the command that repeats it.
function paramsBlock(run) {
  return `<details class="panel params-panel"><summary>Параметры прогона: ${esc(paramsShort(run.params))}</summary>
    <dl class="params">${paramsRows(run).map(([k, v]) => `<dt>${esc(k)}</dt><dd>${esc(v)}</dd>`).join("")}</dl>
    <p class="muted small" style="margin:12px 0 4px">Повторить этот прогон:</p>
    <pre class="cmd">${esc(commandLine(run))}</pre></details>`;
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
  ];
  return all.filter((s) => s.values.some((v) => v >= 0));
}

function threadTable(curve, names, hasFreq) {
  const cell = (v) => `<td class="num">${fmtPct(v)}</td>`;
  const rows = curve.loaded.map((k, i) => `<tr><td>${k}${curve.noisy[i] ? " ⚠" : ""}</td>
    ${cell(curve.single[i])}${cell(curve.solo_b[i])}${cell(curve.pair_a[i])}${cell(curve.pair_b[i])}
    ${hasFreq ? `<td class="num">${fmtMHz(curve.mhz[i])}</td><td class="num">${fmtMHz(curve.pair_mhz[i])}</td>` : ""}</tr>`).join("");
  return `<div class="scroll"><table class="compact"><thead><tr><th>Загружено других ядер</th>
    <th class="num">${esc(names.a)} один</th><th class="num">${esc(names.b)} один</th>
    <th class="num">${esc(names.a)}, сосед нагружен</th><th class="num">${esc(names.b)}, сосед нагружен</th>
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

// variantsBlock: tabs to switch between runs of one command and their
// headline numbers side by side.
async function variantsBlock(run) {
  if (!run.group_id) return "";
  const sibs = (await api(`/api/runs?group=${encodeURIComponent(run.group_id)}`))
    .sort((a, b) => (a.group_index || 0) - (b.group_index || 0));
  if (sibs.length < 2 && (run.group_total || 0) < 2) return "";
  const dims = variantDims.filter((d) => new Set(sibs.map((r) => d.value(r.params))).size > 1);
  const cur = run.params;
  const tabs = dims.map((d) => {
    const values = [...new Set(sibs.map((r) => d.value(r.params)))];
    const links = values.map((v) => {
      if (v === d.value(cur)) return `<span class="tab active">${esc(v)}</span>`;
      // the run that differs from this one only in this setting
      const target = sibs.find((r) => d.value(r.params) === v && dims.every((o) => o === d || o.value(r.params) === o.value(cur)));
      return target ? `<a class="tab" href="#/run/${target.id}">${esc(v)}</a>` : `<span class="tab off" title="этот вариант не измерен">${esc(v)}</span>`;
    }).join("");
    return `<div class="dim"><span class="dim-title">${esc(d.title)}</span>${links}</div>`;
  }).join("");

  const rows = sibs.map((r) => {
    const h = r.headline || {};
    const rt = h.ratings || {};
    const stab = h.stab;
    return `<tr class="${r.id === run.run_id ? "current" : ""}">
      <td><a href="#/run/${r.id}">${esc(dims.length ? variantName(r.params, dims) : r.profile)}</a>${r.partial ? ' <span class="badge">прерван</span>' : ""}</td>
      <td class="num">${h.has_points === false ? "—" : valueWithRating(fmtPct(h.full_alone_pct), rt.full_single)}</td>
      <td class="num">${h.has_pair ? valueWithRating(fmtPct(h.full_pair_pct), rt.full_pair) : "—"}</td>
      <td class="num">${h.has_points === false ? "—" : valueWithRating(fmtTimes(h.lag_ratio), rt.lag)}</td>
      <td class="num">${stab ? valueWithRating(`${fmtPct(stab.worst_sec_pct)} · тик ${fmtPct(stab.work_p99_pct)}`, rt.stab) : "—"}</td>
    </tr>`;
  }).join("");
  const total = run.group_total || sibs.length;
  return `<div class="panel">
    <h2 class="in">Варианты этого запуска</h2>
    <div class="variants">${tabs}</div>
    <details${sibs.length > 1 ? " open" : ""}><summary>Все варианты рядом${sibs.length < total ? ` (пройдено ${sibs.length} из ${total})` : ""}</summary>
    <div class="scroll"><table class="compact"><thead><tr><th>Вариант</th>
      <th class="num">Поток один, остальные ядра нагружены</th><th class="num">Оба потока, каждый</th><th class="num">Лаги</th>
      <th class="num">Тикрейт под нагрузкой</th></tr></thead>
      <tbody>${rows}</tbody></table></div>
    <p class="muted small">Тикрейт под нагрузкой — сценарий «все остальные потоки загружены на 100%»: худшая секунда в процентах от целевого тикрейта
      и время тика (1 из 100 дольше) в процентах бюджета. Оценка учитывает оба числа.</p></details>
  </div>`;
}

const stabColors = [C[0], C[1], C[2], C[3], C[4], "var(--series-6)"];

function stabTable(st) {
  const rows = st.rows.map((r) => `<tr><td>${esc(r.title)}${r.noisy ? ` <span title="другие процессы забирали до ${fmtFrac(r.noise_pct)} процессора">⚠</span>` : ""}</td><td class="num">${r.servers}</td>
    <td class="num">${fmtPct(r.rate_pct)}</td><td class="num">${valueWithRating(fmtPct(r.worst_sec_pct), r.rating)}</td><td class="num">${fmtPct(r.low1_pct)}</td>
    <td class="num grp">${fmtPct(r.work_pct)}</td><td class="num">${fmtPct(r.work_p99_pct)}</td><td class="num">${fmtPct(r.work_max_pct)}</td>
    <td class="num grp">${fmtFrac(r.overrun_pct)}</td><td class="num">${fmtMs(r.var_ms)}</td></tr>`).join("");
  return `<div class="scroll"><table class="compact"><thead>
    <tr><th rowspan="2">Сценарий</th><th rowspan="2" class="num">Серверов</th><th colspan="3" class="grp">Тикрейт, % от ${st.tick_rate}</th>
      <th colspan="3" class="grp">Время тика, % бюджета</th><th rowspan="2" class="num grp">Не уложились в бюджет</th><th rowspan="2" class="num">var</th></tr>
    <tr><th class="num grp">средний</th><th class="num">худшая секунда</th><th class="num">1% low</th>
      <th class="num grp">среднее</th><th class="num">1 из 100</th><th class="num">самый долгий</th></tr>
    </thead><tbody>${rows}</tbody></table></div>`;
}

function stabBlock(st) {
  if (!st || !st.rows.length) return "";
  return `<h2>Стабильность тикрейта</h2>
    <div class="panel">
      <p>Сервер работает как игровой с тикрейтом ${st.tick_rate}: делает тик и спит до следующего. Бюджет тика — <b>${fmtMs(st.budget_ms)}</b>,
        работа тика подобрана так, чтобы при непрерывной нагрузке занимать <b>${fmtMs(st.idle_work_ms)}</b> (${st.tick_load_pct}% бюджета). Если тик не укладывается в бюджет, тикрейт падает, как FPS у сервера Source или TPS у Minecraft.</p>
      ${stabTable(st)}
      ${st.conclusions.length > 1 ? `<div class="conclusions"><ul>${st.conclusions.slice(1).map((c) => `<li>${esc(c)}</li>`).join("")}</ul></div>` : ""}
      <div class="toolbar" style="margin-top:16px"><span>График по секундам:</span>
        <div class="tabs" id="stabview"><button data-v="rate" class="active">тикрейт самого медленного сервера</button><button data-v="work">самый долгий тик</button></div></div>
      <div id="stabchart"></div>
      <p class="muted small">Худшая секунда — самая медленная секунда самого медленного сервера. 1% low — тикрейт по 1% самых длинных интервалов между тиками.
        «1 из 100» — 1% тиков длиннее этого. var — разброс интервала между тиками (как var в stats у Source). Тики, опоздавшие на половину бюджета и больше, сервер пропускает. ⚠ — другие процессы забирали у серверов процессор (больше порога --noise-max).
        Каждый сценарий длится ${Math.round(st.duration_ms / 1000)} с.</p>
    </div>`;
}

function drawStab(el, st, view) {
  const n = Math.max(...st.rows.map((r) => r.sec_rate_pct.length));
  const xs = Array.from({ length: n }, (_, i) => i + 1);
  renderChart(el, {
    xs,
    series: st.rows.map((r, i) => {
      const vals = view === "rate" ? r.sec_rate_pct : r.sec_max_work_pct;
      return {
        name: r.title, color: stabColors[i % stabColors.length],
        values: xs.map((_, j) => vals[j] ?? -1),
        labels: xs.map((_, j) => (vals[j] == null ? "" : view === "rate"
          ? `${Math.round((vals[j] * st.tick_rate) / 100)} из ${st.tick_rate}`
          : `${fmtPct(vals[j])} · ${fmtMs((vals[j] * st.budget_ms) / 100)}`)),
      };
    }),
    xTitle: "секунда сценария",
    tooltipTitle: (x) => `секунда ${x}`,
  });
}

export async function renderRun(app, id) {
  const { run, summary: s } = await api(`/api/runs/${id}`);
  const variants = await variantsBlock(run).catch(() => "");
  const hasRamp = s.cores.length > 0;
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
    ${variants}
    ${paramsBlock(run)}
    ${hasRamp ? `<div class="panel">
      <h2 class="in">Сводка: скорость потока ядра в четырёх условиях</h2>
      ${matrix(s, full)}
      <div class="facts">${facts.join("")}</div>
    </div>
    ${node.conclusions.length ? `<div class="panel conclusions"><b>Выводы</b><ul>${node.conclusions.map((c) => `<li>${esc(c)}</li>`).join("")}</ul></div>` : ""}` : ""}
    ${warnings.length ? `<div class="warnings"><b>Обратите внимание</b><ul>${warnings.map((w) => `<li>${esc(w)}</li>`).join("")}</ul></div>` : ""}
    ${stabBlock(s.stab)}
  ` + (!hasRamp ? "" : `
    <h2>По парам: ядро и его второй поток</h2>
    <div class="panel">
      <div class="toolbar"><span>Показывать:</span>
        <div class="tabs" id="unit"><button data-unit="pct" class="active">проценты</button><button data-unit="tps">тики в секунду</button></div></div>
      <div id="pairs"></div>
      <p class="muted small">«Один» — второй поток этого ядра свободен. «Сосед нагружен» — занят.
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
      <details><summary>Таблица по числу серверов</summary>${nodeTable(node, text, s.has_freq)}</details>
    </div>

    <h2>Карта ядер</h2>
    <div class="panel">
      <div class="toolbar"><label>Цвет: <select id="metric">${Object.entries(metricText)
        .filter(([k]) => s.has_pair || !(k.startsWith("ht_") || k === "full_pair_pct"))
        .map(([k, v]) => `<option value="${k}">${v}</option>`).join("")}</select></label></div>
      <div id="map"></div>
    </div>

    <details><summary>Лаги и частота по ядрам</summary>
      <div class="panel" style="margin-top:10px">${extraTable(s)}</div>
    </details>`);

  if (s.stab && s.stab.rows.length) {
    const el = app.querySelector("#stabchart");
    const btns = app.querySelectorAll("#stabview button");
    btns.forEach((b) => b.addEventListener("click", () => {
      btns.forEach((x) => x.classList.toggle("active", x === b));
      drawStab(el, s.stab, b.dataset.v);
    }));
    drawStab(el, s.stab, "rate");
  }
  if (!hasRamp) return;

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
