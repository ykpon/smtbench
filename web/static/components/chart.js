// Line chart of speed (%) against the number of loaded cores.
// All series share one percent axis. Missing values are -1.
import { showTip, hideTip } from "./tooltip.js";
import { esc, fmtPct } from "../texts.js";

const NS = "http://www.w3.org/2000/svg";

function svgEl(name, attrs = {}, text) {
  const e = document.createElementNS(NS, name);
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  if (text != null) e.textContent = text;
  return e;
}

/**
 * opts: {
 *   xs: number[],                       // loaded-core counts
 *   series: [{ name, color, dash, values: number[], noisy?: boolean[] }],
 *   band?: { from: number, label: string },   // shaded area for x > from
 *   xTitle: string,
 *   tooltipTitle: (x) => string,
 * }
 */
export function renderChart(el, opts) {
  el.innerHTML = "";
  el.classList.add("chart");
  const legend = document.createElement("div");
  legend.className = "legend";
  legend.innerHTML = opts.series
    .map((s) => `<span><span class="sw${s.dash ? " dash" : ""}" style="border-color:${s.color}"></span>${esc(s.name)}</span>`)
    .join("");
  el.appendChild(legend);

  const W = Math.max(el.clientWidth || 800, 320);
  const narrow = W < 560;
  const H = narrow ? 260 : 320;
  const m = { l: 44, r: narrow ? 12 : 130, t: 26, b: 42 };
  const xs = opts.xs;
  const xMax = Math.max(1, xs[xs.length - 1] ?? 1);
  let vMax = 100;
  for (const s of opts.series) for (const v of s.values) if (v > vMax) vMax = v;
  const yMax = Math.ceil((vMax + 5) / 20) * 20;
  const X = (x) => m.l + (x / xMax) * (W - m.l - m.r);
  const Y = (v) => m.t + (1 - v / yMax) * (H - m.t - m.b);

  const svg = svgEl("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": opts.xTitle });
  el.appendChild(svg);

  if (opts.band && opts.band.from >= 0 && opts.band.from < xMax) {
    const bx = X(opts.band.from + 0.5);
    svg.appendChild(svgEl("rect", { class: "band", x: bx, y: m.t, width: W - m.r - bx, height: H - m.t - m.b }));
    svg.appendChild(svgEl("text", { class: "band-label", x: bx + 6, y: m.t - 8 }, opts.band.label));
  }

  const grid = svgEl("g", { class: "grid" });
  for (let v = 0; v <= yMax; v += 20) {
    grid.appendChild(svgEl("line", { x1: m.l, x2: W - m.r, y1: Y(v), y2: Y(v), "stroke-width": v === 100 ? 1.5 : 1 }));
    svg.appendChild(svgEl("text", { x: m.l - 8, y: Y(v) + 4, "text-anchor": "end" }, `${v}%`));
  }
  svg.insertBefore(grid, svg.firstChild);
  svg.appendChild(svgEl("line", { class: "axis", x1: m.l, x2: W - m.r, y1: Y(0), y2: Y(0) }));

  const tickEvery = Math.max(1, Math.ceil(xMax / (narrow ? 6 : 16)));
  for (let x = 0; x <= xMax; x += tickEvery) {
    svg.appendChild(svgEl("text", { x: X(x), y: H - m.b + 18, "text-anchor": "middle" }, x));
  }
  svg.appendChild(svgEl("text", { x: (m.l + W - m.r) / 2, y: H - 6, "text-anchor": "middle" }, opts.xTitle));

  const labels = [];
  for (const s of opts.series) {
    let d = "";
    let pen = false;
    xs.forEach((x, i) => {
      const v = s.values[i];
      if (v == null || v < 0) { pen = false; return; }
      d += `${pen ? "L" : "M"}${X(x).toFixed(1)},${Y(v).toFixed(1)}`;
      pen = true;
    });
    if (!d) continue;
    svg.appendChild(svgEl("path", { class: `series${s.dash ? " dash" : ""}`, d, stroke: s.color }));
    if (!s.dash) {
      xs.forEach((x, i) => {
        const v = s.values[i];
        if (v == null || v < 0) return;
        const noisy = s.noisy && s.noisy[i];
        svg.appendChild(svgEl("circle", { class: "marker", cx: X(x), cy: Y(v), r: 4, fill: noisy ? "var(--surface)" : s.color, stroke: noisy ? s.color : "var(--surface)" }));
      });
    }
    let last = -1;
    s.values.forEach((v, i) => { if (v >= 0) last = i; });
    if (last >= 0) labels.push({ y: Y(s.values[last]), x: X(xs[last]), name: s.name });
  }

  if (!narrow && opts.series.length <= 4) {
    labels.sort((a, b) => a.y - b.y);
    for (let i = 1; i < labels.length; i++) labels[i].y = Math.max(labels[i].y, labels[i - 1].y + 15);
    for (const l of labels) svg.appendChild(svgEl("text", { class: "direct", x: l.x + 10, y: l.y + 4 }, l.name));
  }

  const cross = svgEl("line", { class: "cross", y1: m.t, y2: H - m.b, visibility: "hidden" });
  svg.appendChild(cross);
  const hit = svgEl("rect", { x: m.l, y: m.t, width: W - m.l - m.r, height: H - m.t - m.b, fill: "transparent" });
  svg.appendChild(hit);

  const nearest = (evt) => {
    const r = svg.getBoundingClientRect();
    const px = ((evt.clientX - r.left) / r.width) * W;
    let best = 0;
    xs.forEach((x, i) => { if (Math.abs(X(x) - px) < Math.abs(X(xs[best]) - px)) best = i; });
    return best;
  };
  const move = (evt) => {
    const i = nearest(evt);
    cross.setAttribute("x1", X(xs[i]));
    cross.setAttribute("x2", X(xs[i]));
    cross.setAttribute("visibility", "visible");
    let noisy = false;
    const rows = opts.series.map((s) => {
      noisy = noisy || (s.noisy && s.noisy[i]);
      return `<div class="row"><span class="key"><i style="background:${s.color}"></i>${esc(s.name)}</span><b>${fmtPct(s.values[i])}</b></div>`;
    });
    showTip(`<div><b>${esc(opts.tooltipTitle(xs[i]))}</b></div>${rows.join("")}${noisy ? '<div class="muted">⚠ измерено с помехами от других процессов</div>' : ""}`, evt.clientX, evt.clientY);
  };
  hit.addEventListener("pointermove", move);
  hit.addEventListener("pointerdown", move);
  hit.addEventListener("pointerleave", () => { cross.setAttribute("visibility", "hidden"); hideTip(); });
}
