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
  full_single_pct: "Нода забита по одному серверу на ядро",
  full_pair_pct: "Нода забита по два сервера на ядро",
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
