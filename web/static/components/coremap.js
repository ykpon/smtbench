// Grid of physical cores grouped by L3 group (socket or chiplet).
import { esc, fmtPct, groupName, sizeMB, metricText } from "../texts.js";
import { showTip, hideTip } from "./tooltip.js";

const steps = ["--seq-0", "--seq-1", "--seq-2", "--seq-3", "--seq-4"];

export function renderCoreMap(el, run, summary, metric, selected, onSelect) {
  const byCore = new Map(summary.cores.map((c) => [c.core, c]));
  const vals = summary.cores.map((c) => c[metric]).filter((v) => v >= 0);
  // Fixed domain so that 99% and 100% do not look like opposite ends.
  const lo = Math.min(60, ...vals);
  const hi = Math.max(100, ...vals);
  const step = (v) => {
    if (v < 0) return null;
    return Math.max(0, Math.min(4, Math.floor(((v - lo) / (hi - lo)) * 5)));
  };
  const dark = matchMedia("(prefers-color-scheme: dark)").matches;
  const cpuCores = new Map(run.cpu.cores.map((c) => [c.id, c]));

  el.innerHTML = `<div class="coremap">${run.cpu.l3_groups
    .map((g) => {
      const big = g.big ? " · большой L3" : "";
      const cells = g.cores
        .filter((id) => byCore.has(id))
        .map((id) => {
          const c = byCore.get(id);
          const s = step(c[metric]);
          const bg = s == null ? "var(--grid)" : `var(${steps[s]})`;
          const lightInk = s != null && (dark ? s <= 1 : s >= 3);
          const cls = ["cell", lightInk ? "ink-light" : "", c.weak ? "weak" : "", id === selected ? "selected" : ""].join(" ");
          const threads = (cpuCores.get(id)?.threads || []).join("/");
          return `<button class="${cls}" data-core="${id}" style="background:${bg}">
            <div class="n">Ядро ${id}</div><div class="t">потоки ${esc(threads)}</div>
            <div class="v">${fmtPct(c[metric])}${c.noisy ? " ⚠" : ""}</div></button>`;
        })
        .join("");
      if (!cells) return "";
      return `<div class="group"><h3>${esc(groupName(run.cpu, g))} · L3 ${sizeMB(g.size_kb)}${big}</h3><div class="cells">${cells}</div></div>`;
    })
    .join("")}</div>
    <div class="scale"><span>${fmtPct(lo)}</span><span class="bar"></span><span>${fmtPct(hi)}</span>
    <span>· пунктирная рамка — ядро медленнее остальных больше чем на 5%</span></div>`;

  el.querySelectorAll(".cell").forEach((b) => {
    const id = Number(b.dataset.core);
    const c = byCore.get(id);
    b.addEventListener("click", () => onSelect(id));
    b.addEventListener("pointermove", (e) => {
      const lines = [
        `<div><b>Ядро ${id}</b></div>`,
        `<div class="row"><span>${esc(metricText[metric])}</span><b>${fmtPct(c[metric])}</b></div>`,
      ];
      if (c.weak) lines.push('<div class="muted">медленнее остальных ядер больше чем на 5%</div>');
      if (c.drift) lines.push(`<div class="muted">скорость в начале и в конце теста различается на ${Math.round(c.drift_pct)}%</div>`);
      if (c.noisy) lines.push('<div class="muted">⚠ часть замеров с помехами от других процессов</div>');
      lines.push('<div class="muted">нажмите, чтобы показать на графике</div>');
      showTip(lines.join(""), e.clientX, e.clientY);
    });
    b.addEventListener("pointerleave", hideTip);
  });
}
