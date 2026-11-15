// Formatting and all user-facing wording.

const nbsp = " ";

export function fmtInt(v) {
  return Math.round(v).toString().replace(/\B(?=(\d{3})+(?!\d))/g, nbsp);
}

export function fmtPct(v) {
  return v == null || v < 0 ? "—" : `${Math.round(v)}%`;
}

export function fmtDec(v) {
  return v.toFixed(1).replace(".", ",");
}

export function fmtTimes(v) {
  return v == null || v < 0 ? "—" : `${fmtDec(v)}×`;
}

export function fmtDate(s) {
  const d = new Date(s);
  return d.toLocaleString("ru-RU", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });
}

export const ratingWord = { good: "хорошо", warn: "средне", bad: "плохо" };

export function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
}

export function ratingHTML(r) {
  if (!r) return "";
  return `<span class="rating ${r}"><span class="dot"></span><span class="word">${ratingWord[r]}</span></span>`;
}

export function valueWithRating(text, r) {
  return `<span class="rating ${r || ""}">${r ? '<span class="dot"></span>' : ""}<span>${text}</span></span>`;
}

export const metricText = {
  solo_pct: "Скорость ядра (100% — типичное ядро этой машины)",
  ht_a_pct: "1-й поток, когда нагружен 2-й",
  ht_b_pct: "2-й поток, когда нагружен 1-й",
  full_single_pct: "Поток один, остальные ядра нагружены",
  full_pair_pct: "Оба потока нагружены, остальные ядра нагружены",
};

export const modeText = { single: "1 сервер на ядро", pair: "2 сервера на ядро", solo_b: "второй поток", final: "повтор" };

// groupName names an L3 group: a socket when each package has one group, otherwise a chiplet.
export function groupName(cpu, g) {
  const inPkg = cpu.l3_groups.filter((x) => x.package === g.package).length;
  if (inPkg === 1) return cpu.packages > 1 ? `Сокет ${g.package}` : "Процессор";
  return `Чиплет ${g.id}`;
}

export function groupKindWord(cpu) {
  const multiPerPkg = cpu.l3_groups.some((g) => cpu.l3_groups.filter((x) => x.package === g.package).length > 1);
  return multiPerPkg ? "чиплет" : "сокет";
}

export function sizeMB(kb) {
  return `${Math.round(kb / 1024)}${nbsp}МБ`;
}

export function fmtMHz(v) {
  return v == null || v <= 0 ? "—" : `${Math.round(v)}${nbsp}МГц`;
}

// othersLoaded names the "other cores loaded" condition for a run's bg_threads.
export function othersLoaded(bgThreads) {
  if (bgThreads === 2) return "Все остальные ядра нагружены";
  if (bgThreads === 1) return "Остальные ядра нагружены одним потоком";
  return "Остальные ядра нагружены так же, как тестируемое";
}

// bgText describes how neighbour cores were loaded (params.bg_threads).
export function bgText(bg) {
  if (bg === 2) return "оба потока";
  if (bg === 1) return "один поток";
  return "как у тестируемого ядра (auto)";
}

// cpuList formats [0,1,2,3,8] as "0–3, 8".
export function cpuList(list) {
  if (!list || !list.length) return "все";
  const s = [...list].sort((a, b) => a - b);
  const out = [];
  for (let i = 0; i < s.length; i++) {
    let j = i;
    while (j + 1 < s.length && s[j + 1] === s[j] + 1) j++;
    out.push(j > i + 1 ? `${s[i]}–${s[j]}` : j === i + 1 ? `${s[i]}, ${s[j]}` : `${s[i]}`);
    i = j;
  }
  return out.join(", ");
}

function secs(ms) {
  return ms % 1000 === 0 ? `${ms / 1000} с` : `${ms} мс`;
}

function flagDur(ms) {
  return ms % 1000 === 0 ? `${ms / 1000}s` : `${ms}ms`;
}

function mixText(mix) {
  return Object.entries(mix || {}).sort().map(([k, v]) => `${k}=${v}`).join(",");
}

const defaultModes = "single,solo_b,pair";

// paramsShort is a one-line summary of the settings that change the results.
export function paramsShort(p) {
  if (!p) return "";
  const parts = [`соседи: ${bgText(p.bg_threads).replace(" (auto)", "")}`, `ядра: ${cpuList(p.cores)}`];
  if (p.step > 1) parts.push(`шаг ${p.step}`);
  if (p.order === "far") parts.push("сначала чужой чиплет/сокет");
  if ((p.modes || []).join(",") !== defaultModes) parts.push(`режимы: ${(p.modes || []).join(",")}`);
  parts.push(`замер ${secs(p.duration_ms)}`);
  if (p.reps > 1) parts.push(`повторов ${p.reps}`);
  return parts.join(" · ");
}

// paramsRows lists every setting of a run as [name, value].
export function paramsRows(run) {
  const p = run.params;
  const rows = [
    ["Профиль нагрузки", p.profile === "custom" ? `своя смесь: ${mixText(p.mix)}` : `${p.profile} (${mixText(p.mix)})`],
  ];
  if (p.mem_size_mb > 0) rows.push(["Буфер памяти на поток", `${p.mem_size_mb} МБ`]);
  rows.push(
    ["Нагрузка на соседние ядра (--bg-threads)", bgText(p.bg_threads)],
    ["Тестируемые ядра (--cores)", cpuList(p.cores)],
    ["Что мерилось (--modes)", (p.modes || []).map((m) => ({ single: "1-й поток один", solo_b: "2-й поток один", pair: "оба потока" })[m] || m).join(", ")],
    ["Шаг нагрузки (--step)", `${p.step} ${p.step === 1 ? "ядро" : "ядра"} за шаг`],
    ["Порядок загрузки (--order)", p.order === "far" ? "far: сначала чужой чиплет/сокет" : "near: сначала свой чиплет/сокет"],
    ["Замер и прогрев одной точки", `${secs(p.duration_ms)} + ${secs(p.warmup_ms)}`],
    ["Повторов точки (--reps)", String(p.reps)],
    ["Порог помех (--noise-max)", `${p.noise_max_pct}%`],
    ["Источник частоты", run.cpu.freq_source === "none" ? "не измерялась" : run.cpu.freq_source],
    ["Версия утилиты", run.tool_version],
  );
  return rows;
}

// commandLine rebuilds the command that repeats the run.
export function commandLine(run) {
  const p = run.params;
  const a = ["smtbench run"];
  if (p.profile === "custom") a.push(`--mix ${mixText(p.mix)}`);
  else a.push(`--profile ${p.profile}`);
  if (p.mem_size_mb > 0) a.push(`--mem-size ${p.mem_size_mb}MB`);
  a.push(`--bg-threads ${p.bg_threads === 0 ? "auto" : p.bg_threads}`);
  if (p.cores && p.cores.length) a.push(`--cores ${[...p.cores].sort((x, y) => x - y).join(",")}`);
  if ((p.modes || []).join(",") !== defaultModes) a.push(`--modes ${(p.modes || []).join(",")}`);
  if (p.step !== 1) a.push(`--step ${p.step}`);
  if (p.order !== "near") a.push(`--order ${p.order}`);
  if (p.duration_ms !== 3000) a.push(`--duration ${flagDur(p.duration_ms)}`);
  if (p.warmup_ms !== 1000) a.push(`--warmup ${flagDur(p.warmup_ms)}`);
  if (p.reps !== 1) a.push(`--reps ${p.reps}`);
  if (p.noise_max_pct !== 3) a.push(`--noise-max ${p.noise_max_pct}`);
  a.push(`--label ${run.label}`);
  return a.join(" ");
}
