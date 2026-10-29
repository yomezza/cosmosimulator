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

export function createPortal({ roots, act, command }) {
  const n=UI.node, tr=(s)=>I18n.t(s), b=UI.button, row=UI.row, field=UI.field, choice=UI.choice;
function renderPortal() {
    const root = roots.portal;
    reset(root);
    const portal = w().portal?.[0];
    if (!portal) return;
    root.append(
      n("h2", tr("Межгалактический переход")),
      n(
        "p",
        tr(
          "Совместный проект межгалактического перехода. Взносы учитываются в общем прогрессе.",
        ),
      ),
      UI.bar(portal.contributed, portal.required, "Вклад, CR"),
    );
    if (!portal.active) {
      const amount = field(
        root,
        "Вклад, CR",
        "portal-amount",
        Math.min(100, portal.required - portal.contributed),
        "number",
      );
      amount.max = portal.required - portal.contributed;
      root.append(
        b("Внести вклад", () =>
          command({
            action: "portal_contribute",
            amount: Number(amount.value),
          }),
        ),
      );
    }
    const current = world.data?.stars[d().ship.star - 1]?.galaxy || "edem",
      destinations = (world.data?.galaxies || []).filter(
        (g) => g.opened && g.id !== current,
      );
    if (destinations.length) {
      let target = destinations[0].id;
      choice(
        root,
        "Галактика назначения",
        destinations.map((g) => [g.id, g.name]),
        target,
        (value) => (target = value),
      );
      root.append(
        b("Перейти в другую галактику", async () => {
          if (await command({ action: "portal_travel", target })) {
            world.data = await api("galaxy");
            window.dispatchEvent(new Event("universechange"));
          }
        }),
      );
    }
    root.append(
      n(
        "p",
        tr("Переход доступен из систем у центральной чёрной дыры."),
        "muted",
      ),
      n(
        "p",
        (w().portal_cores?.[current] || w().core_systems || [])
          .map((id) => world.data?.stars[id - 1]?.name || "")
          .join(" · "),
      ),
    );
  }
function clear() {}
return {renderPortal,clear};
}
