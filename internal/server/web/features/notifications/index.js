import { Session, RequestScope } from "../../core/session.js";
import { scopedView } from "../../core/scoped-view.js";
import { I18n } from "../../core/i18n.js";
import { Format } from "../../core/format.js";
import { api } from "../../core/api.js";
import { refresh } from "../../core/snapshot-store.js";
import { UI, Modules } from "../../ui/runtime.js";
import { notify } from "../../ui/feedback.js";
import { Game, GalaxyAdmin, Scheduler, AdminResources, Logistics, scenes } from "../../features/runtime.js";
import { world, loadWorld } from "../../core/world-store.js";
import { performExpedition } from "../../features/commands.js";
import { d, w, res, num, station, fmtTime, money, known, fly, cargoCapacity, fuelCapacity } from "../../features/selectors.js";
import { reset } from "../../ui/view-state.js";
import { actionAmount } from "../../ui/quantity.js";
import { resourceName } from "../../features/inventory/resource-label.js";
import { can } from "../../features/corporations/permissions.js";

export function createNotifications({ roots, act, command }) {
  const n=UI.node, tr=(s)=>I18n.t(s), b=UI.button, row=UI.row, field=UI.field, choice=UI.choice;
let notifySelected = new Set();
let noticeCursor = null;
let eventNames = {
    arrival: "Корабль прибыл",
    mining_complete: "Добыча завершена",
    research_complete: "Исследование завершено",
    delivery_complete: "Доставка завершена",
    company_invitation: "Приглашение в корпорацию",
    project_complete: "Проект завершён",
    portal_open: "Портал активирован",
  };
function eventText(item) {
    const data = item.data || {};
    if (item.kind === "colony")
      return (
        tr("Колонизация завершена") +
        " · " +
        data.planet +
        " · " +
        tr("Сектор") +
        " " +
        data.sector
      );
    return (
      (item.message
        ? tr(item.message) +
          (data.ship_name ? " · " + data.ship_name : "") +
          (data.reason ? " · " + tr(data.reason) : "")
        : "") ||
      tr(eventNames[item.kind] || item.kind) +
        (data.star
          ? " · " +
            (world.data?.stars.find((s) => s.id === data.star)?.name ||
              "#" + data.star)
          : "") +
        (data.resource
          ? " · " + tr(res(data.resource)?.name || data.resource)
          : "") +
        (data.quantity ? " × " + data.quantity : "")
    );
  }
async function loadNotifications() {
    if (!Session.signedIn) return;
    const epoch = Session.epoch,
      items = await api("notifications");
    if (epoch !== Session.epoch) return;
    Session.data.notifications = items;
    notificationBadge();
    if (Modules.isOpen("notifications")) renderNotifications();
  }
function notificationBadge() {
    const info = Session.data.notifications || { count: 0, items: [] },
      badge = document.getElementById("notification-count");
    badge.hidden = !info.count;
    badge.textContent = info.count > 100 ? "99+" : info.count;
    const latest = info.items[0]?.id || 0;
    if (noticeCursor !== null) {
      const fresh = info.items.filter(
        (item) => item.id > noticeCursor && item.kind !== "message",
      );
      if (fresh.length) notify(eventText(fresh[0]), fresh[0].level, false);
    }
    noticeCursor = latest;
  }
function renderNotifications() {
    const root = roots.notifications;
    reset(root);
    const items = Session.data.notifications?.items || [];
    notifySelected = new Set(
      [...notifySelected].filter((id) => items.some((item) => item.id === id)),
    );
    const tools = n("div", undefined, "inline-controls sticky-toolbar"),
      remove = async (body) => {
        await api("notifications", body);
        notifySelected.clear();
        await loadNotifications();
      };
    tools.append(
      b(
        "Удалить выбранные",
        () => remove({ action: "delete", ids: [...notifySelected] }),
        !notifySelected.size,
      ),
      n("span", String(notifySelected.size)),
      b("Очистить все", () => remove({ action: "clear" })),
    );
    root.append(tools);
    const all = n("input");
    all.type = "checkbox";
    all.setAttribute("aria-label", tr("Выбрать все"));
    all.checked = items.length > 0 && notifySelected.size === items.length;
    all.indeterminate =
      notifySelected.size > 0 && notifySelected.size < items.length;
    all.onchange = () => {
      notifySelected = all.checked
        ? new Set(items.map((v) => v.id))
        : new Set();
      renderNotifications();
    };
    root.append(all, n("span", tr("Выбрать все")));
    const groups = new Map();
    for (const item of items) {
      const key = item.kind === "message" ? "message-" + item.id : item.kind;
      const group = groups.get(key) || [];
      group.push(item);
      groups.set(key, group);
    }
    for (const group of groups.values()) {
      let host = root;
      if (group.length > 1) {
        const details = UI.fold("notifications:group:" + group[0].kind);
        details.append(
          n(
            "summary",
            tr(eventNames[group[0].kind] || group[0].kind) +
              " × " +
              group.length,
          ),
        );
        root.append(details);
        host = details;
      }
      for (const item of group) {
        const card = n("article", undefined, "notification-row"),
          check = n("input");
        check.type = "checkbox";
        check.checked = notifySelected.has(item.id);
        check.setAttribute("aria-label", eventText(item));
        check.onchange = () => {
          check.checked
            ? notifySelected.add(item.id)
            : notifySelected.delete(item.id);
          renderNotifications();
        };
        const detail = UI.fold("notification:" + item.id);
        detail.append(
          n("summary", eventText(item)),
          n("p", fmtTime(item.created_at), "muted"),
        );
        if (item.kind === "colony")
          detail.append(
            b("Открыть проект", () =>
              UI.loadPlanet().then((p) => p.project(item.data.project)),
            ),
          );
        if (item.data?.star)
          detail.append(
            b("На карте", () => {
              window.dispatchEvent(
                new CustomEvent("galaxyselection", { detail: item.data.star }),
              );
              Modules.open("galaxy");
            }),
          );
        if (item.data?.ship)
          detail.append(
            b("Корабль", async () => {
              await command({ action: "select_ship", ship: item.data.ship });
              Modules.open("ship");
            }),
          );
        if (item.kind === "mining_complete")
          detail.append(b("Трюм", () => Modules.open("cargo")));
        card.dataset.level = item.level;
        card.append(
          check,
          n(
            "span",
            item.level === "error" ? "×" : item.level === "warning" ? "!" : "✓",
            "notification-icon",
          ),
          detail,
          b("×", () => remove({ action: "delete", ids: [item.id] })),
        );
        host.append(card);
      }
    }
    if (!items.length) UI.empty(root, "Уведомлений пока нет");
  }
function clear() {notifySelected=new Set();noticeCursor=null;}
return {eventText,loadNotifications,notificationBadge,renderNotifications,clear};
}
