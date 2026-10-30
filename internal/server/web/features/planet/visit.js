import { createController } from "./controller.js";
import { api } from "../../core/api.js";
import { Session, RequestScope } from "../../core/session.js";
import { I18n } from "../../core/i18n.js";
import { UI, Modules } from "../../ui/runtime.js";
import { notify } from "../../ui/feedback.js";
const key = "planet-visit",
  scope = new RequestScope();
let socket,
  renderer,
  polling,
  busy = false;
const tr = (s) => I18n.t(s);
function stop() {
  renderer?.stop();
  renderer = null;
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
  canvas.className = "visit-canvas";
  const viewport = UI.node("div", undefined, "visit-viewport");
  viewport.append(canvas);
  root.classList.add("visit-content");
  let seq = 1,
    lastState,
    lastPath,
    finishLeave;
  const send = (type, payload) => {
    if (socket?.readyState !== WebSocket.OPEN || !welcome) return false;
    socket.send(
      JSON.stringify({
        v: 1,
        type,
        seq: ++seq,
        visit: welcome.visit,
        connection_epoch: welcome.connection_epoch,
        payload,
      }),
    );
    return true;
  };
  canvas.hidden = true;
  const resume = UI.button("Продолжить высадку", () => openVisit()),
    leave = UI.button("Вернуться в космос", async () => {
      leave.disabled = true;
      returning = true;
      renderer?.stop();
      renderer = null;
      try {
        if (socket?.readyState === WebSocket.OPEN && welcome) {
          await new Promise((resolve) => {
            const timer = setTimeout(resolve, 4000);
            finishLeave = () => {
              clearTimeout(timer);
              resolve();
            };
            if (!send("leave", { operation: crypto.randomUUID() })) {
              clearTimeout(timer);
              resolve();
            }
          });
        }
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
  leave.disabled = true;
  tools.append(
    resume,
    leave,
    UI.button("−", () => renderer?.zoom(1 / 1.2)),
    UI.button("+", () => renderer?.zoom(1.2)),
    UI.button("К персонажу", () => renderer?.centre()),
    UI.button("Весь сектор", () => renderer?.fit()),
  );
  root.replaceChildren(
    status,
    tools,
    UI.node("p", tr("ЛКМ — идти · Колесо — масштаб · ПКМ — обзор"), "muted"),
    viewport,
  );
  Modules.open(key);
  let welcome,
    returning = false,
    canReturn = false;
  try {
    const state = await api("planet/visit", undefined, task.signal);
    if (!task.current()) return;
    canReturn = state.state !== "none";
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
    canReturn = true;
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
        if (message.type === "visit.ended") {
          finishLeave?.();
          return;
        }
        if (returning) return;
        if (message.type === "path.accepted") {
          lastPath = message.payload;
          renderer?.accepted(lastPath);
          return;
        }
        if (message.type === "state.delta") {
          const wasSuspended = lastState?.suspended;
          lastState = message.payload;
          renderer?.update(lastState);
          if (lastState.suspended)
            status.textContent = tr(
              "Движение приостановлено: проверяем связь с сервером",
            );
          return;
        }
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
        if (!task.current() || returning) return;
        renderer?.stop();
        canvas.hidden = false;
        const controller = await createController(canvas, scene, welcome, {
          current: () =>
            task.current() &&
            !returning &&
            currentSocket.readyState === WebSocket.OPEN,
          target: (x, y) => send("move", { x_q: x, y_q: y }),
        });
        if (
          !task.current() ||
          returning ||
          currentSocket.readyState !== WebSocket.OPEN
        ) {
          controller?.stop();
          return;
        }
        renderer = controller;
        if (lastState) renderer?.update(lastState);
        if (lastPath) renderer?.accepted(lastPath);
        if (task.current() && !returning) canvas.hidden = false;
      } catch (e) {
        if (task.current()) status.textContent = e.message;
      }
    };
    currentSocket.onclose = () => {
      if (task.current()) {
        renderer?.stop();
        renderer = null;
      }
      finishLeave?.();
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
    if (task.current()) leave.disabled = !canReturn;
    busy = false;
  }
}
window.addEventListener("signedout", stop);
window.addEventListener("moduleclose", (event) => {
  if (event.detail === key) stop();
});
window.addEventListener("moduledestroy", (event) => {
  if (event.detail === key) stop();
});
