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

export function createFinance({ roots, act, command }) {
  const n=UI.node, tr=(s)=>I18n.t(s), b=UI.button, row=UI.row, field=UI.field, choice=UI.choice;
function ledgerAmount(value) {
    const amount = Number(value),
      span = n(
        "span",
        (amount > 0 ? "+" : "") + money(amount),
        amount > 0
          ? "ledger-income"
          : amount < 0
            ? "ledger-expense"
            : "ledger-neutral",
      );
    if (amount !== 0) span.dataset.help = amount > 0 ? "Доход" : "Расход";
    return span;
  }
function renderFinance() {
    const root = roots.finance;
    reset(root);
    root.append(
      UI.table(
        ["Дата", "Операция", "Изменение", "Баланс"],
        (w().ledger || []).map((item) => [
          fmtTime(item.created_at),
          tr(item.reason),
          ledgerAmount(item.amount),
          money(item.balance),
        ]),
      ),
    );
    if (!w().ledger?.length) UI.empty(root, "Операций пока нет");
  }
function clear() {}
return {ledgerAmount,renderFinance,clear};
}
