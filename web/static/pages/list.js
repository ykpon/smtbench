import { api } from "../app.js";
import { esc, fmtDate, fmtPct, paramsShort, valueWithRating, variantDims, variantName } from "../texts.js";

// groupRuns keeps the list order but puts the runs of one command together,
// at the place of the newest of them.
function groupRuns(runs) {
  const out = [];
  const seen = new Map();
  for (const r of runs) {
    if (!r.group_id) {
      out.push({ runs: [r] });
      continue;
    }
    if (!seen.has(r.group_id)) {
      const g = { id: r.group_id, runs: [] };
      seen.set(r.group_id, g);
      out.push(g);
    }
    seen.get(r.group_id).runs.push(r);
  }
  for (const g of out) g.runs.sort((a, b) => (a.group_index || 0) - (b.group_index || 0));
  return out;
}

function stabCell(h) {
  const s = h.stab;
  if (!s) return '<span class="muted">—</span>';
  return valueWithRating(`${fmtPct(s.worst_sec_pct)} · тик ${fmtPct(s.work_p99_pct)}`, h.ratings?.stab);
}

function row(r, name, inGroup) {
  const h = r.headline || {};
  const rt = h.ratings || {};
  const alone = h.has_points === false ? '<span class="muted">—</span>' : valueWithRating(fmtPct(h.full_alone_pct ?? h.full_single_pct), rt.full_single);
  const pair = h.has_pair ? valueWithRating(fmtPct(h.full_pair_pct), rt.full_pair) : '<span class="muted">—</span>';
  return `<tr class="${inGroup ? "in-group" : ""}">
    <td><input type="checkbox" data-id="${r.id}" aria-label="выбрать для сравнения"></td>
    <td><a href="#/run/${r.id}">${esc(name)}</a>${r.partial ? ' <span class="badge">прерван</span>' : ""}</td>
    <td class="cpu">${esc(r.cpu_model)}</td>
    <td>${esc(r.profile)}</td>
    <td class="params">${esc(paramsShort(r.params))}</td>
    <td class="num">${alone}</td>
    <td class="num">${pair}</td>
    <td class="num">${stabCell(h)}</td>
    <td>${fmtDate(r.started_at)}</td>
  </tr>`;
}

export async function renderList(app) {
  const runs = await api("/api/runs");
  if (!runs.length) {
    app.innerHTML = `<h1>Прогоны</h1>
      <div class="panel"><p>Результатов пока нет.</p>
      <p>Запустите тест на ноде с флагом <code>--send ${esc(location.origin)}</code> — <a href="#/howto">как это сделать</a>.</p></div>`;
    return;
  }
  const rows = groupRuns(runs).map((g) => {
    const first = g.runs[0];
    const label = first.label || first.hostname;
    if (!g.id) return row(first, label, false);
    const dims = variantDims.filter((d) => new Set(g.runs.map((r) => d.value(r.params))).size > 1);
    const total = first.group_total || g.runs.length;
    const head = `<tr class="group-head"><td></td><td colspan="8"><b>${esc(label)}</b> · набор из ${total} вариантов${g.runs.length < total ? `, пройдено ${g.runs.length}` : ""}</td></tr>`;
    return head + g.runs.map((r) => row(r, dims.length ? variantName(r.params, dims) : label, true)).join("");
  }).join("");

  app.innerHTML = `<h1>Прогоны</h1>
    <p class="sub">Цифры — скорость потока ядра, когда остальные ядра нагружены (как именно — в колонке «Параметры», «соседи»). 100% — скорость потока одного на пустой машине.
      «Тикрейт под нагрузкой» — худшая секунда сервера, когда все остальные потоки загружены, и самый долгий тик (1 из 100) в процентах бюджета.</p>
    <div class="toolbar"><button class="primary" id="cmp" disabled>Сравнить выбранные</button><span class="muted small">выберите от 2 до 4 прогонов</span></div>
    <div class="panel scroll"><table>
      <thead><tr><th></th><th>Нода</th><th>Процессор</th><th>Профиль</th><th>Параметры</th>
        <th class="num">Поток один</th><th class="num">Оба потока, каждый</th><th class="num">Тикрейт под нагрузкой</th><th>Дата</th></tr></thead>
      <tbody>${rows}</tbody></table></div>`;

  const btn = app.querySelector("#cmp");
  const boxes = [...app.querySelectorAll("input[type=checkbox]")];
  const picked = () => boxes.filter((b) => b.checked).map((b) => b.dataset.id);
  boxes.forEach((b) => b.addEventListener("change", () => {
    const n = picked().length;
    btn.disabled = n < 2 || n > 4;
  }));
  btn.addEventListener("click", () => { location.hash = `#/compare/${picked().join(",")}`; });
}
