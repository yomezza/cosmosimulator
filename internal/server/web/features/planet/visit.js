import { api } from "../../core/api.js";
import { Session, RequestScope } from "../../core/session.js";
import { I18n } from "../../core/i18n.js";
import { UI, Modules } from "../../ui/runtime.js";
import { notify } from "../../ui/feedback.js";
const key = "planet-visit",
  scope = new RequestScope();
let socket,
  polling,
  busy = false;
const tr = (s) => I18n.t(s);
function stop() {
  scope.invalidate();
  socket?.close();
  socket = null;
  clearInterval(polling);
  polling = null;
}
async function requestScene(w, signal) {
  const response = await fetch(
    `/planet/api/v1/scene?visit=${encodeURIComponent(w.visit)}&epoch=${w.connection_epoch}`,
    { signal },
  );
  if (!response.ok) {
    const error = await response.json();
    throw new Error(tr(error.error));
  }
  return response.json();
}
export async function openVisit(site) {
  if (busy) return;
  busy = true;
  stop();
  const task = scope.begin();
  const root = Modules.create(key, "Планетарная локация", 1000, 760);
  const status = UI.node("p", tr("Подключение к планетарной службе")),
    tools = UI.node("div", undefined, "visit-tools"),
    canvas = UI.node("canvas");
  canvas.width = 1024;
  canvas.height = 1024;
  canvas.className = "visit-preview";
  canvas.hidden = true;
  const resume = UI.button("Продолжить высадку", () => openVisit()),
    leave = UI.button("Вернуться в космос", async () => {
      leave.disabled = true;
      returning = true;
      try {
        const state = await api(
          "planet/return",
          { operation: crypto.randomUUID(), visit: welcome?.visit || "" },
          task.signal,
        );
        if (!task.current()) return;
        socket?.close();
        socket = null;
        status.textContent = tr(
          state.state === "none"
            ? "Возвращение завершено"
            : "Завершаем высадку",
        );
        canvas.hidden = true;
        clearInterval(polling);
        polling = setInterval(async () => {
          try {
            const v = await api("planet/visit", undefined, task.signal);
            if (!task.current()) return;
            if (v.state === "none") {
              clearInterval(polling);
              polling = null;
              status.textContent = tr("Возвращение завершено");
              leave.disabled = true;
              resume.hidden = true;
            }
          } catch {}
        }, 1500);
      } catch (e) {
        if (task.current()) notify(e.message, "error");
        leave.disabled = false;
      }
    });
  tools.append(resume, leave);
  root.replaceChildren(
    status,
    tools,
    UI.node(
      "p",
      tr("Обзор локации. Управление персонажем появится на следующем этапе."),
      "muted",
    ),
    canvas,
  );
  Modules.open(key);
  let welcome,
    returning = false;
  try {
    const state = await api("planet/visit", undefined, task.signal);
    if (!task.current()) return;
    let hello;
    if (["active", "disconnected"].includes(state.state)) {
      hello = { resume_visit: state.visit };
    } else if (state.state === "legacy" || state.state === "return_pending") {
      status.textContent = tr("Завершаем высадку");
      return;
    } else if (site || state.state === "pending") {
      const ticket = await api(
        "planet/handoff",
        {
          operation: state.operation || crypto.randomUUID(),
          landing_site: String(state.landing_site || site),
          ship: String(state.ship || Session.data.expedition.ship.id),
        },
        task.signal,
      );
      if (!task.current()) return;
      if (ticket.ticket) {
        hello = { ticket: ticket.ticket };
        delete ticket.ticket;
      } else if (ticket.visit) {
        hello = { resume_visit: ticket.visit };
      } else throw new Error(tr("Повторите подключение"));
    } else {
      status.textContent = tr("Выберите доступную посадочную площадку");
      leave.disabled = true;
      return;
    }
    const url = new URL("/planet/ws/v1", location.href);
    url.protocol = location.protocol === "https:" ? "wss:" : "ws:";
    const currentSocket = new WebSocket(url);
    socket = currentSocket;
    currentSocket.onopen = () => {
      if (!task.current()) {
        currentSocket.close();
        return;
      }
      currentSocket.send(
        JSON.stringify({ v: 1, type: "hello", seq: 1, payload: hello }),
      );
      hello = null;
    };
    currentSocket.onmessage = async (event) => {
      if (!task.current()) return;
      try {
        const message = JSON.parse(event.data);
        if (message.type === "error") {
          status.textContent = tr(message.payload.code);
          return;
        }
        if (message.type !== "welcome") return;
        welcome = message.payload;
        resume.hidden = true;
        status.textContent =
          welcome.planet + " · " + tr("Сектор") + " " + welcome.sector;
        const scene = await requestScene(welcome, task.signal);
        if (!task.current()) return;
        await draw(canvas, scene, welcome, task);
        if (task.current()) canvas.hidden = false;
      } catch (e) {
        if (task.current()) status.textContent = e.message;
      }
    };
    currentSocket.onclose = () => {
      if (task.current() && !returning) {
        resume.hidden = false;
        status.textContent = tr(
          "Соединение прервано. Можно продолжить высадку или вернуться на корабль.",
        );
      }
    };
    currentSocket.onerror = () => {
      if (task.current()) status.textContent = tr("Не удалось подключиться");
    };
  } catch (e) {
    if (task.current()) {
      status.textContent = e.message;
      notify(e.message, "error");
    }
  } finally {
    busy = false;
  }
}
const specs = {
  tree_old: [225, 0.5, 0.82],
  tree_young: [160, 0.5, 0.82],
  bush_sparse: [88, 0.5, 0.8],
  grass_tuft: [57, 0.5, 0.87],
  rock_flat: [98, 0.5, 0.7],
  rock_large_cracked: [168, 0.5, 0.78],
  grass_dry: [63, 0.5, 0.78],
  colonist: [104, 0.5, 0.86],
};
async function draw(canvas, scene, w, task) {
  const images = new Map();
  await Promise.all(
    [
      ...new Set(
        scene.objects.map((o) => o.asset.split("/").pop().replace(".png", "")),
      ),
      "colonist",
    ].map(async (name) => {
      if (!specs[name]) return;
      const image = new Image();
      image.src = "/planet-art/" + name + ".png";
      try {
        await image.decode();
        images.set(name, image);
      } catch {}
    }),
  );
  if (!task.current()) return;
  const ctx = canvas.getContext("2d"),
    cells = Uint8Array.from(atob(scene.cells), (c) => c.charCodeAt(0));
  ctx.clearRect(0, 0, 1024, 1024);
  ctx.setTransform(4, 0, 0, 4, 0, 0);
  for (let y = 0; y < 256; y++)
    for (let x = 0; x < 256; x++) {
      const cell = cells[y * 256 + x];
      if (!(cell & 1)) continue;
      ctx.fillStyle = cell & 2 ? "#24586c" : "#3b5041";
      ctx.fillRect(x, y, 1, 1);
    }
  const objects = scene.objects.map((o) => ({
    name: o.asset.split("/").pop().replace(".png", ""),
    x: o.position.X / 1024,
    y: o.position.Y / 1024,
  }));
  objects.push({ name: "colonist", x: w.x_q / 1024, y: w.y_q / 1024 });
  objects.sort((a, b) => a.y - b.y);
  for (const obj of objects) {
    const img = images.get(obj.name),
      spec = specs[obj.name];
    if (!img || !spec) continue;
    const size = spec[0] / 16;
    ctx.drawImage(
      img,
      obj.x - size * spec[1],
      obj.y - size * spec[2],
      size,
      size,
    );
  }
  if (scene.landing) {
    ctx.strokeStyle = "#80f4d6";
    ctx.lineWidth = 0.5;
    ctx.beginPath();
    ctx.ellipse(
      scene.landing.X / 1024,
      scene.landing.Y / 1024,
      117 / 16,
      66 / 16,
      0,
      0,
      Math.PI * 2,
    );
    ctx.stroke();
  }
  ctx.setTransform(1, 0, 0, 1, 0, 0);
}
window.addEventListener("signedout", stop);
window.addEventListener("moduleclose", (event) => {
  if (event.detail === key) stop();
});
window.addEventListener("moduledestroy", (event) => {
  if (event.detail === key) stop();
});
