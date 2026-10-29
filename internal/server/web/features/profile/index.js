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

export function createProfile({ roots, act, command }) {
  const n=UI.node, tr=(s)=>I18n.t(s), b=UI.button, row=UI.row, field=UI.field, choice=UI.choice;
function renderProfile() {
    const root = roots.profile;
    reset(root);
    row(root, "Позывной", Session.data.state.user.username);
    row(
      root,
      "Роль",
      tr(Session.data.state.user.role === "admin" ? "АДМИНИСТРАТОР" : "ИГРОК"),
    );
    row(root, "Корпорация", Session.data.profile.company || tr("Без компании"));
    row(root, "Кораблей", Session.data.fleet.ships.length);
    root.append(
      b("История финансов", () => Modules.open("finance")),
      b("Настройки", () => Modules.open("settings")),
    );
  }
function clear() {}
return {renderProfile,clear};
}
