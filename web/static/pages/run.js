import { api } from "../app.js";
import { renderChart } from "../components/chart.js";
import { renderCoreMap } from "../components/coremap.js";
import { esc, fmtDate, fmtInt, fmtPct, fmtTimes, groupKindWord, metricText, ratingHTML } from "../texts.js";

export function boundaryLabel(run) {
  const kind = groupKindWord(run.cpu);
  return run.params.order === "far" ? `дальше — свой ${kind}` : `дальше — другой ${kind}`;
}

export function curveSeries(curve, hasFreq) {
  const s = [
    { name: "1 сервер на ядро", color: "var(--series-1)", values: curve.single, noisy: curve.noisy },
  ];
  if (curve.pair.some((v) => v >= 0)) s.push({ name: "2 сервера на ядро", color: "var(--series-2)", values: curve.pair, noisy: curve.noisy });
  if (hasFreq) s.push({ name: "частота", color: "var(--series-3)", dash: true, values: curve.freq_pct });
  return s;
}

function card(title, value, explain, rating) {
  return `<div class="card"><div class="title">${title}</div><div class="value">${value}</div>
    <div class="explain">${explain}</div>${ratingHTML(rating)}</div>`;
}

function cards(s) {
  const r = s.ratings || {};
  const out = [];
  if (s.has_pair && s.ht_per_thread_pct >= 0) {
    const gain = Math.round(s.ht_total_pct - 100);
    out.push(card("2 сервера на одном ядре (HT)", `каждый ${fmtPct(s.ht_per_thread_pct)}`,
      `Ядро в сумме даёт ${fmtPct(s.ht_total_pct)}: второй поток добавляет ${gain}% к производительности ядра, но каждый сервер работает медленнее.`, r.ht));
  } else {
    out.push(card("2 сервера на одном ядре (HT)", "—", "HT выключен или недоступен, этот режим не тестировался.", null));
  }
  out.push(card("1 сервер на ядро, машина загружена целиком", fmtPct(s.full_single_pct),
    "Скорость сервера, когда все остальные ядра заняты такой же нагрузкой. 100% — скорость на пустой машине.", r.full_single));
  if (s.has_pair) {
    out.push(card("2 сервера на ядро, машина загружена целиком", `каждый ${fmtPct(s.full_pair_pct)}`,
      "Все потоки машины заняты: так работает нода, заполненная серверами по 2 на ядро.", r.full_pair));
  }
  if (s.lag_ratio > 0) {
    out.push(card("Лаги при полной загрузке", `в ${fmtTimes(s.lag_ratio).replace("×", " раза")}`,
      "Во столько раз самые долгие тики (1 из 100) длиннее, чем на пустой машине.", r.lag));
  }
  return out.join("");
}

function detailsTable(run, s) {
  const threads = new Map(run.cpu.cores.map((c) => [c.id, c.threads.join("/")]));
  const rows = s.cores.map((c) => `<tr>
    <td>${c.core}</td><td>${threads.get(c.core) || ""}</td>
    <td class="num">${c.base_tps > 0 ? fmtInt(c.base_tps) : "—"}</td>
    <td class="num">${fmtPct(c.solo_pct)}</td>
    <td class="num">${fmtPct(c.ht_per_thread_pct)}</td>
    <td class="num">${fmtPct(c.full_single_pct)}</td>
    <td class="num">${fmtPct(c.full_pair_pct)}</td>
    <td class="num">${fmtTimes(c.lag_ratio)}</td>
    <td>${[c.weak ? "медленное" : "", c.drift ? `дрейф ${Math.round(c.drift_pct)}%` : "", c.noisy ? "помехи" : ""].filter(Boolean).join(", ")}</td>
  </tr>`).join("");
  return `<div class="scroll"><table><thead><tr>
    <th>Ядро</th><th>Потоки</th><th class="num">Тиков/с</th><th class="num">Скорость ядра</th>
    <th class="num">2 сервера</th><th class="num">1 сервер, всё занято</th><th class="num">2 сервера, всё занято</th>
    <th class="num">Лаги</th><th>Заметки</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

export async function renderRun(app, id) {
  const { run, summary: s } = await api(`/api/runs/${id}`);
  const p = run.params;
  const warnings = [...(s.warnings || []), ...(run.warnings || [])];
  const knee = s.knee_single >= 0
    ? `<span>Просадка меньше 5%, пока загружено не больше <b>${s.knee_single}</b> других ядер</span>` : "";

  app.innerHTML = `
    <h1>${esc(run.label || run.hostname)}${run.partial ? ' <span class="badge">прерван</span>' : ""}</h1>
    <p class="sub">${esc(run.cpu.model)} · профиль <b>${esc(p.profile)}</b> · ${fmtDate(run.started_at)}</p>
    <div class="cards">${cards(s)}</div>
    <div class="facts">
      <span>Скорость одного ядра на пустой машине: <b>${s.baseline_tps > 0 ? fmtInt(s.baseline_tps) : "—"}</b> тиков/с</span>
      ${knee}
    </div>
    ${warnings.length ? `<div class="warnings"><b>Обратите внимание</b><ul>${warnings.map((w) => `<li>${esc(w)}</li>`).join("")}</ul></div>` : ""}

    <h2>Как падает скорость, когда загружаются остальные ядра</h2>
    <div class="panel">
      <div class="toolbar"><label>Показать: <select id="core"></select></label>
        ${s.has_freq ? "" : '<span class="muted small">частота не измерялась</span>'}</div>
      <div id="chart"></div>
      <p class="muted small">Порядок загрузки: ${p.order === "far" ? "сначала чужой" : "сначала свой"} ${groupKindWord(run.cpu)}.
        Фоновые ядра загружены ${p.bg_threads === 1 ? "одним потоком (1 сервер на ядро)" : "обоими потоками (2 сервера на ядро)"}.
        Частота — в процентах от частоты на пустой машине. Полый кружок — замер с помехами.</p>
    </div>

    <h2>Карта ядер</h2>
    <div class="panel">
      <div class="toolbar"><label>Цвет: <select id="metric">${Object.entries(metricText)
        .filter(([k]) => s.has_pair || k !== "ht_per_thread_pct")
        .map(([k, v]) => `<option value="${k}">${v}</option>`).join("")}</select></label></div>
      <div id="map"></div>
    </div>

    <details><summary>Подробно по ядрам и параметры теста</summary>
      <div class="panel" style="margin-top:10px">${detailsTable(run, s)}
      <p class="muted small">Параметры: смесь ${esc(Object.entries(p.mix || {}).map(([k, v]) => `${k}=${v}`).join(", "))}
        · буфер памяти ${p.mem_size_mb} МБ · замер ${p.duration_ms / 1000} с · прогрев ${p.warmup_ms / 1000} с
        · шаг ${p.step} · версия ${esc(run.tool_version)} · частота: ${esc(run.cpu.freq_source)}</p></div>
    </details>`;

  const coreSel = app.querySelector("#core");
  coreSel.innerHTML = `<option value="avg">в среднем по ядрам</option>` +
    s.cores.map((c) => `<option value="${c.core}">ядро ${c.core}</option>`).join("");
  const metricSel = app.querySelector("#metric");
  let selected = null;

  const draw = () => {
    const curve = selected == null ? s.avg : s.cores.find((c) => c.core === selected).curve;
    renderChart(app.querySelector("#chart"), {
      xs: curve.loaded,
      series: curveSeries(curve, s.has_freq),
      band: { from: curve.boundary, label: boundaryLabel(run) },
      xTitle: "загружено других ядер",
      tooltipTitle: (x) => `загружено других ядер: ${x}`,
    });
    renderCoreMap(app.querySelector("#map"), run, s, metricSel.value, selected, (id) => {
      selected = id;
      coreSel.value = String(id);
      draw();
    });
  };
  coreSel.addEventListener("change", () => {
    selected = coreSel.value === "avg" ? null : Number(coreSel.value);
    draw();
  });
  metricSel.addEventListener("change", draw);
  draw();
}
