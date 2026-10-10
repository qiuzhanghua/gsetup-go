// GUI-Setup 的前端：三个界面都是这一页里的三段。
//
//   A 这个家里已经装过了（有就显示，可以卸载 / 升级）
//   B 要装到哪儿（选工具链、改路径、要不要进 PATH）
//   C 执行过程（只读地先给你看要跑什么，再把输出原样搬过来）
//
// 这里不做任何判断：参数怎么拼、能不能卸，全是 Go 那边算好的。

const $ = (id) => document.getElementById(id);

let state = null;
let plan = null;
let outputEl = null;
let running = false;
let pendingRemoval = null;

document.addEventListener("DOMContentLoaded", async () => {
  $("close-button").addEventListener("click", () => {
    if (window.go && window.go.main) window.go.main.App.Quit();
  });
  $("start-button").addEventListener("click", start);
  $("remove-confirm").addEventListener("click", confirmRemoval);
  $("remove-cancel").addEventListener("click", cancelRemoval);
  $("home").addEventListener("input", refresh);
  $("add-to-path").addEventListener("change", refresh);
  outputEl = $("output");

  const rt = await waitForRuntime();
  if (!rt) {
    showFatal("界面和 Go 那边还没接上（window.go 没出现）。");
    return;
  }
  window.runtime.EventsOn("output", onOutput);
  window.runtime.EventsOn("start", () => setStatus("正在跑…"));
  window.runtime.EventsOn("done", onDone);

  try {
    state = await window.go.main.App.Bootstrap();
  } catch (err) {
    showFatal(String(err));
    return;
  }
  if (!state.ok) {
    showFatal(state.message);
    return;
  }
  renderShell();
  refresh();
});

async function waitForRuntime() {
  for (let i = 0; i < 50; i++) {
    if (window.go && window.go.main && window.runtime) return true;
    await new Promise((r) => setTimeout(r, 100));
  }
  return false;
}

function showFatal(message) {
  $("error-message").textContent = message;
  $("error-box").classList.remove("hidden");
  $("choose-box").classList.add("hidden");
  $("command-box").classList.add("hidden");
  $("removal-box").classList.add("hidden");
  $("title").textContent = "这个包打不开";
  $("status").textContent = "";
}

function renderShell() {
  $("title").textContent = `${state.name} ${state.version} 安装向导`;
  $("subtitle").textContent =
    `包根：${state.root}　·　简称：${state.short}　·　平台：${state.goos}`;
  $("cmd-name").textContent = state.cmd;

  renderInstalls();

  // B 区：工具链单选
  const box = $("toolchains");
  box.innerHTML = "";
  for (const t of state.toolchains) {
    const label = document.createElement("label");
    const input = document.createElement("input");
    input.type = "radio";
    input.name = "toolchain";
    input.value = t;
    input.checked = t === state.defaultToolchain;
    input.addEventListener("change", () => {
      $("home").value = t === state.defaultToolchain ? state.defaultHome : `~/${t}`;
      refresh();
    });
    label.appendChild(input);
    label.appendChild(document.createTextNode(t));
    label.classList.toggle("on", input.checked);
    box.appendChild(label);
  }
  $("home").value = state.defaultHome;

  $("choose-box").classList.remove("hidden");
}

// A 区：两个家都报一句。装完 / 卸完之后要重新画一遍，见 reloadInstalls。
function renderInstalls() {
  const list = $("installed-list");
  list.innerHTML = "";
  const anyInstalled = state.installs.some((i) => i.installed);
  $("installed-box").classList.toggle("hidden", !anyInstalled);
  for (const inst of state.installs) {
    list.appendChild(installRow(inst));
  }
}

// 装完或卸完之后重新问 Go 那边一次，A 区才会从"装着 0.3.2 / 重新安装 / 卸载"
// 变回"里没装过 / 装到这里"。Bootstrap 可以反复调，它每次都重读两个账本。
async function reloadInstalls() {
  let fresh;
  try {
    fresh = await window.go.main.App.Bootstrap();
  } catch (err) {
    setRowsBusy(false);
    return;
  }
  if (!fresh || !fresh.ok) {
    setRowsBusy(false);
    return;
  }
  state.installs = fresh.installs;
  renderInstalls();
}

// 正在跑的时候 A 区的按钮一起变灰；跑完由 renderInstalls 重新造出新按钮。
// data-locked 记着"这个按钮天生不可用"（家里没有 bin/gpm 时卸载就是这种）。
function setRowsBusy(busy) {
  document.querySelectorAll("#installed-list button").forEach((b) => {
    b.disabled = busy || b.dataset.locked === "1";
  });
}

function installRow(inst) {
  const row = document.createElement("div");
  row.className = "row";

  const text = document.createElement("div");
  text.className = "grow";
  if (inst.installed) {
    text.innerHTML =
      `<span class="ver">${escapeHTML(inst.homeDisplay)}</span> 里装着 ` +
      `<span class="ver">${escapeHTML(inst.version || "?")}</span>` +
      (inst.needsUpgrade ? ` <span class="tag">（比包里的旧，可以升级）</span>` : "");
  } else {
    text.innerHTML = `<span class="ver">${escapeHTML(inst.homeDisplay)}</span> <span class="tag">里没装过</span>`;
  }
  row.appendChild(text);

  if (inst.installed) {
    const up = document.createElement("button");
    up.className = "tiny";
    up.textContent = inst.needsUpgrade ? "升级到 " + state.version : "重新安装";
    up.addEventListener("click", () => {
      selectToolchain(inst.toolchain, inst.home);
      start();
    });
    row.appendChild(up);

    const rm = document.createElement("button");
    rm.className = "tiny";
    rm.textContent = "卸载";
    rm.disabled = !inst.uninstallable;
    if (!inst.uninstallable) rm.dataset.locked = "1";
    rm.title = inst.uninstallable ? "" : "这个家里没有 bin/gpm，得在终端里自己卸";
    rm.addEventListener("click", () => uninstall(inst));
    row.appendChild(rm);
  } else {
    const to = document.createElement("button");
    to.className = "tiny";
    to.textContent = "装到这里";
    to.addEventListener("click", () => {
      selectToolchain(inst.toolchain, inst.home);
      start();
    });
    row.appendChild(to);
  }
  return row;
}

function selectToolchain(toolchain, home) {
  const radios = document.querySelectorAll('input[name="toolchain"]');
  radios.forEach((r) => {
    r.checked = r.value === toolchain;
    r.parentElement.classList.toggle("on", r.checked);
  });
  $("home").value = home;
}

function currentChoice() {
  const checked = document.querySelector('input[name="toolchain"]:checked');
  return {
    toolchain: checked ? checked.value : state.defaultToolchain,
    home: $("home").value,
    addToPath: $("add-to-path").checked,
  };
}

async function refresh() {
  if (running) return;
  const c = currentChoice();
  plan = await window.go.main.App.Plan(c.toolchain, c.home, c.addToPath);
  $("command-box").classList.remove("hidden");
  $("command").textContent = plan.ok ? plan.display : plan.message;
  const warn = $("warning");
  if (plan.ok && plan.warning) {
    warn.textContent = "注意：" + plan.warning;
    warn.classList.remove("hidden");
  } else {
    warn.classList.add("hidden");
  }
  $("start-button").disabled = !plan.ok;
  // 选中的这个家里已经装过了（Go 那边查的账本）：那就不给"开始安装"这个按钮，
  // 要重装 / 升级 / 卸载都到上面 A 区那几颗按钮去。
  $("start-button").classList.toggle("hidden", plan.ok && plan.installed);
  setStatus(plan.ok ? `将装到 ${plan.homeDisplay}` : "还不能开始");
}

function setStatus(text) {
  $("status").textContent = text;
}

async function start() {
  if (running) return;
  const c = currentChoice();
  running = true;
  pendingRemoval = null;
  $("removal-box").classList.add("hidden");
  $("output-box").classList.remove("hidden");
  outputEl.innerHTML = "";
  $("start-button").disabled = true;
  setRowsBusy(true);
  setStatus("正在跑…");
  try {
    await window.go.main.App.Start(c.toolchain, c.home, c.addToPath);
  } catch (err) {
    appendLine({ stream: "stderr", text: String(err) });
  }
}

async function uninstall(inst) {
  if (running) return;
  let preview;
  try {
    preview = await window.go.main.App.PlanUninstall(inst.home);
  } catch (err) {
    showLines([{ stream: "stderr", text: String(err) }]);
    return;
  }
  if (!preview.ok) {
    showLines([{ stream: "stderr", text: preview.message }]);
    return;
  }
  renderRemoval(preview);
}

// 需求 §6：先把 gpm 即将回放删掉的东西摊开，再让用户点"确认卸载"。
function renderRemoval(preview) {
  $("removal-title").textContent =
    `${preview.name} ${preview.version}（${preview.homeDisplay}）—— 由 ${preview.program} 执行`;
  $("removal-command").textContent = preview.display;

  const list = $("removal-list");
  list.innerHTML = "";
  for (const item of preview.items) {
    const li = document.createElement("li");
    if (!item.present) li.className = "gone";

    const kind = document.createElement("span");
    kind.className = "kind";
    kind.textContent = item.kind;
    li.appendChild(kind);

    const path = document.createElement("span");
    path.className = "path";
    path.textContent = item.display;
    li.appendChild(path);

    if (!item.present) {
      const note = document.createElement("span");
      note.className = "note";
      note.textContent = "（已经不在）";
      li.appendChild(note);
    }
    list.appendChild(li);
  }

  const warn = $("removal-warning");
  if (preview.warning) {
    warn.textContent = preview.warning;
    warn.classList.remove("hidden");
  } else {
    warn.classList.add("hidden");
  }

  $("removal-box").classList.remove("hidden");
  pendingRemoval = preview;
  $("remove-confirm").disabled = false;
  setStatus(`要卸掉 ${preview.name} ${preview.version}`);
  $("removal-box").scrollIntoView({ block: "nearest" });
}

function cancelRemoval() {
  pendingRemoval = null;
  $("removal-box").classList.add("hidden");
  setStatus("");
  refresh();
}

async function confirmRemoval() {
  if (running || !pendingRemoval) return;
  const home = pendingRemoval.home;
  pendingRemoval = null;
  $("removal-box").classList.add("hidden");
  $("remove-confirm").disabled = true;
  running = true;
  setRowsBusy(true);
  $("output-box").classList.remove("hidden");
  outputEl.innerHTML = "";
  appendLine({ stream: "system", text: "$ " + $("removal-command").textContent });
  setStatus("正在卸载…");
  try {
    await window.go.main.App.Uninstall(home);
  } catch (err) {
    appendLine({ stream: "stderr", text: String(err) });
  }
}

function showLines(lines) {
  $("output-box").classList.remove("hidden");
  for (const line of lines) appendLine(line);
}

function onOutput(ev) {
  $("output-box").classList.remove("hidden");
  appendLine(ev);
}

function onDone(payload) {
  running = false;
  const message = (payload && payload.message) || "结束";
  appendLine({
    stream: payload && payload.result && payload.result.success ? "ok" : "bad",
    text: "—— " + message + " ——",
  });
  // 装完或卸完之后，A 区那份底稿已经过期了：重新问一次再画。
  Promise.all([refresh(), reloadInstalls()]).then(() => setStatus(message));
}

function appendLine(ev) {
  if (!outputEl) return;
  const line = document.createElement("div");
  line.className = ev.stream || "stdout";
  line.textContent = ev.text;
  outputEl.appendChild(line);
  outputEl.scrollTop = outputEl.scrollHeight;
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (ch) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[ch]);
}
