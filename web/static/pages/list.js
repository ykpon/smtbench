import { api } from "../app.js";
import { esc, fmtDate, fmtPct, paramsShort, valueWithRating } from "../texts.js";

export async function renderList(app) {
  const runs = await api("/api/runs");
  if (!runs.length) {
    app.innerHTML = `<h1>Прогоны</h1>
      <div class="panel"><p>Результатов пока нет.</p>
      <p>Запустите тест на ноде с флагом <code>--send ${esc(location.origin)}</code> — <a href="#/howto">как это сделать</a>.</p></div>`;
    return;
  }
  const rows = runs
    .map((r) => {
      const h = r.headline || {};
      const rt = h.ratings || {};
      const pair = h.has_pair ? valueWithRating(fmtPct(h.full_pair_pct), rt.full_pair) : '<span class="muted">HT нет</span>';
      return `<tr>
        <td><input type="checkbox" data-id="${r.id}" aria-label="выбрать для сравнения"></td>
        <td><a href="#/run/${r.id}">${esc(r.label || r.hostname)}</a>${r.partial ? ' <span class="badge">прерван</span>' : ""}</td>
        <td>${esc(r.cpu_model)}</td>
        <td>${esc(r.profile)}</td>
        <td class="params">${esc(paramsShort(r.params))}</td>
        <td class="num">${valueWithRating(fmtPct(h.full_alone_pct ?? h.full_single_pct), rt.full_single)}</td>
        <td class="num">${pair}</td>
        <td>${fmtDate(r.started_at)}</td>
      </tr>`;
    })
    .join("");
  app.innerHTML = `<h1>Прогоны</h1>
    <p class="sub">Цифры — скорость потока ядра, когда остальные ядра нагружены (как именно — в колонке «Параметры», «соседи»). 100% — скорость потока одного на пустой машине.</p>
    <div class="toolbar"><button class="primary" id="cmp" disabled>Сравнить выбранные</button><span class="muted small">выберите от 2 до 4 прогонов</span></div>
    <div class="panel scroll"><table>
      <thead><tr><th></th><th>Нода</th><th>Процессор</th><th>Профиль</th><th>Параметры</th>
        <th class="num">Поток один</th><th class="num">Оба потока, каждый</th><th>Дата</th></tr></thead>
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
