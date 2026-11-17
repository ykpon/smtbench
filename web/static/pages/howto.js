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
      <tr><td><code>--modes single,pair,stability</code></td><td>не мерить второй поток ядра в одиночку на всей лесенке — на треть быстрее</td></tr>
      <tr><td><code>--bg-threads 1,2,auto</code></td><td>как грузить соседние ядра: <code>2</code> — оба потока (по умолчанию), <code>1</code> — один поток, <code>auto</code> — как тестируемое ядро</td></tr>
    </tbody></table></div>
    <h2>Несколько вариантов за один запуск</h2>
    <div class="panel">
      <p>У <code>--profile</code>, <code>--bg-threads</code> и <code>--order</code> можно указать несколько значений через запятую, у профиля ещё <code>all</code> — все профили.
        Тест пройдёт по разу для каждого сочетания, поэтому время умножается на число вариантов: <code>--profile source,minecraft --bg-threads 1,2</code> — это четыре полных прогона.
        <code>--dry-run</code> покажет все варианты и общее время.</p>
      <pre class="cmd">sudo ./smtbench run --profile all --bg-threads 1,2 --label имя-ноды --send ${esc(o)}</pre>
      <p class="muted small">Каждый вариант сохраняется в свой файл и отправляется сразу, как закончится. На странице прогона варианты одного запуска переключаются вкладками.</p>
    </div>
    <h2>Тест стабильности тикрейта</h2>
    <div class="panel">
      <p>Запускается после основного теста и идёт около минуты. Сервер работает как игровой: заданное число раз в секунду делает тик и спит до следующего.
        Тест показывает, держит ли сервер тикрейт и насколько дольше становится тик, когда занят второй поток ядра, загружены остальные ядра или нагрузка соседей скачет.</p>
      <table><tbody>
        <tr><td><code>--modes stability</code></td><td>только тест тикрейта, без основного теста</td></tr>
        <tr><td><code>--modes single,solo_b,pair</code></td><td>только основной тест, без тикрейта</td></tr>
        <tr><td><code>--tickrate 20</code></td><td>тиков в секунду (по умолчанию 20 для minecraft, 64 для остальных профилей)</td></tr>
        <tr><td><code>--tick-load 70</code></td><td>сколько процентов бюджета тика занимает работа сервера при непрерывной нагрузке на пустой машине (по умолчанию 50)</td></tr>
        <tr><td><code>--stab-duration 20s</code></td><td>длительность каждого сценария (по умолчанию 10 с)</td></tr>
      </tbody></table>
    </div>`;

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
