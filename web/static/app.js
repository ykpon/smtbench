import { renderList } from "./pages/list.js";
import { renderRun } from "./pages/run.js";
import { renderCompare } from "./pages/compare.js";
import { renderHowto } from "./pages/howto.js";
import { esc } from "./texts.js";

export async function api(path) {
  const resp = await fetch(path);
  const body = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new Error(body.error || `ошибка ${resp.status}`);
  return body;
}

const app = document.getElementById("app");

async function route() {
  const hash = location.hash.replace(/^#/, "") || "/";
  const [, page, arg] = hash.split("/");
  window.scrollTo(0, 0);
  try {
    if (page === "run" && arg) await renderRun(app, arg);
    else if (page === "compare" && arg) await renderCompare(app, arg.split(","));
    else if (page === "howto") renderHowto(app);
    else await renderList(app);
  } catch (e) {
    app.innerHTML = `<div class="panel"><p>Не удалось загрузить данные: ${esc(e.message)}</p><p><a href="#/">К списку прогонов</a></p></div>`;
  }
}

window.addEventListener("hashchange", route);
route();
