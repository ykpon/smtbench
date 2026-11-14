import { esc } from "../texts.js";

const profiles = [
  ["source", "Source/CS2: логика, физика, немного памяти"],
  ["minecraft", "Minecraft (Java): логика и много обходов памяти"],
  ["rust", "Rust (Unity): логика, физика и память поровну"],
  ["int", "только целочисленная логика"],
  ["fp", "только арифметика с плавающей точкой"],
  ["avx", "только 256-битный AVX"],
  ["mem", "только обходы памяти"],
];

export function renderHowto(app) {
  const o = location.origin;
  const cmds = {
    linux: `curl -fsSLo smtbench ${o}/download/smtbench-linux-amd64 && chmod +x smtbench
sudo ./smtbench run --profile minecraft --label имя-ноды --send ${o}`,
    windows: `iwr ${o}/download/smtbench-windows-amd64.exe -OutFile smtbench.exe
.\\smtbench.exe run --profile rust --label имя-ноды --send ${o}`,
  };
  app.innerHTML = `<h1>Как запустить тест</h1>
    <p class="sub">Тест загружает все потоки машины на 100%: на 16 ядрах полный прогон идёт около 52 минут. Игровые серверы на ноде в это время будут тормозить, а их нагрузка исказит результат — запускайте на свободной ноде.</p>
    <div class="panel">
      <div class="tabs"><button data-os="linux" class="active">Linux</button><button data-os="windows">Windows</button></div>
      <pre class="cmd" id="cmd"></pre>
      <p class="muted small" id="note"></p>
    </div>
    <h2>Профили нагрузки</h2>
    <div class="panel scroll"><table><tbody>
      ${profiles.map(([n, d]) => `<tr><td><code>--profile ${n}</code></td><td>${esc(d)}</td></tr>`).join("")}
    </tbody></table>
    <p class="muted small">Профили — синтетические приближения, а не настоящие игровые серверы. Своя смесь: <code>--mix int=50,mem=40,fp=10</code>.</p></div>
    <h2>Полезные флаги</h2>
    <div class="panel scroll"><table><tbody>
      <tr><td><code>--dry-run</code></td><td>показать план и время без запуска</td></tr>
      <tr><td><code>--step 2</code></td><td>загружать ядра по два за шаг — вдвое быстрее</td></tr>
      <tr><td><code>--cores 0-3</code></td><td>тестировать только часть ядер</td></tr>
      <tr><td><code>--duration 2s</code></td><td>короче замер каждой точки (по умолчанию 3 с)</td></tr>
      <tr><td><code>--order far</code></td><td>сначала загружать чужой чиплет или сокет (полезно на AMD X3D с двумя чиплетами)</td></tr>
      <tr><td><code>--modes single,pair</code></td><td>не мерить второй поток ядра в одиночку на всей лесенке — на треть быстрее</td></tr>
      <tr><td><code>--bg-threads 2</code></td><td>соседние ядра всегда грузить обоими потоками: сервер с целым ядром на ноде, проданной по потокам</td></tr>
    </tbody></table></div>`;

  const notes = {
    linux: "Под root утилита дополнительно меряет частоту ядер. Результат сохраняется в файл рядом; если отправить не удалось, команда для повторной отправки будет в выводе.",
    windows: "Запускайте в PowerShell. Частота на Windows не измеряется.",
  };
  const show = (os) => {
    app.querySelector("#cmd").textContent = cmds[os];
    app.querySelector("#note").textContent = notes[os];
    app.querySelectorAll(".tabs button").forEach((b) => b.classList.toggle("active", b.dataset.os === os));
  };
  app.querySelectorAll(".tabs button").forEach((b) => b.addEventListener("click", () => show(b.dataset.os)));
  show("linux");
}
