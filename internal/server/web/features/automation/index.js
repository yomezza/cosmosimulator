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

export function createAutomation({ roots, act, command, deliveryDetails }) {
  const n=UI.node, tr=(s)=>I18n.t(s), b=UI.button, row=UI.row, field=UI.field, choice=UI.choice;
function renderTasks() {
    const container = roots.tasks;
    if (!container.querySelector(".planner")) reset(container);
    [...container.children]
      .filter((e) => !e.classList.contains("planner"))
      .forEach((e) => e.remove());
    Scheduler.render(container).catch((error) =>
      notify(error.message, "error"),
    );
    for (const delivery of (w().deliveries || []).filter(
      (j) =>
        (j.owner === Session.data.state.user.id ||
          j.carrier === Session.data.state.user.id) &&
        ["bot", "carrying", "open"].includes(j.status),
    ))
      container.append(
        b(
          "#" +
            delivery.id +
            " · " +
            tr(delivery.status) +
            " · " +
            delivery.from_star +
            " → " +
            delivery.to_star,
          () => deliveryDetails(delivery),
        ),
      );
  }
function clear() {}
return {renderTasks,clear};
}
