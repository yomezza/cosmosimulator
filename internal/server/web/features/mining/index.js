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

export function createMining({ roots, act, command }) {
  const n=UI.node, tr=(s)=>I18n.t(s), b=UI.button, row=UI.row, field=UI.field, choice=UI.choice;
function renderResearch() {
    const root = roots.research;
    reset(root);
    row(root, "Текущая система", d().system.name + " · " + tr(d().region));
    const layout = n("div", undefined, "research-layout"),
      main = n("section"),
      side = n("aside");
    layout.append(main, side);
    root.append(layout);
    if (d().surveyed) {
      const rows = [];
      for (const site of d().sites.filter((s) => s.resources?.length)) {
        for (const id of site.resources) {
          const q = site.quotas?.[id],
            near = d().ship.object_id === site.id && !fly(d().ship),
            controls = n("div", undefined, "inline-controls"),
            available = q?.remaining || 0;
          controls.append(
            b(
              near ? "Добыть" : "Приблизиться",
              () =>
                near
                  ? performExpedition({
                      action: "mine",
                      planet: site.planet,
                      resource: id,
                    })
                  : performExpedition({
                      action: "move",
                      target: site.id,
                      mode: site.details ? "orbit" : "approach",
                    }),
              !!d().job || fly(d().ship) || available <= 0,
              available <= 0
                ? "Лимит добычи исчерпан"
                : "Дождитесь завершения действия",
            ),
          );
          const details = UI.fold(
            "mining:" + d().system.id + ":" + site.id + ":" + id,
          );
          details.append(
            n("summary", site.name),
            n(
              "p",
              `${tr(site.kind)} · ${site.quality} · ${site.concentration}%`,
            ),
            b("Информация", () => UI.object(site, d().system)),
          );
          const quota = n("span", num(available) + " / " + num(q?.capacity));
          if (q) {
            quota.dataset.resetAt = q.reset_at;
            quota.dataset.remaining = available;
            quota.dataset.capacity = q.capacity;
          }
          rows.push([details, resourceName(id), quota, controls]);
        }
      }
      main.append(UI.table(["Объект", "Ресурс", "Доступно", "Действие"], rows));
    } else UI.empty(main, "Данные о месторождениях недоступны");
    const cycle = n("section", undefined, "production-card");
    cycle.append(n("h3", tr("Цикл добычи")));
    if (d().job) {
      cycle.append(
        resourceName(d().job.resource),
        n("p", "× " + d().job.quantity),
      );
      const p = n("progress");
      p.max = 100;
      p.dataset.jobStarted = d().job.started_at;
      p.dataset.jobEnd = new Date(
        new Date(Session.data.clock.last_tick).getTime() +
          Session.data.clock.settings.tick_seconds * 1000,
      ).toISOString();
      cycle.append(
        p,
        n("p", tr("Завершение на следующей фазе"), "muted"),
        b("Остановить добычу", () =>
          performExpedition({ action: "cancel_mine" }),
        ),
      );
    } else UI.empty(cycle, "Выберите месторождение");
    side.append(cycle);
    const training = n("section", undefined, "production-card");
    training.append(
      n("h3", tr("Учебное производство")),
      n("p", num(Session.data.state.user.ore) + " ORE"),
      n("p", tr("1 ORE за фазу · продажа по 5 CR"), "muted"),
      b(
        "Продать учебную руду",
        () => act("sell", {}),
        Session.data.state.user.ore < 1,
      ),
    );
    side.append(training);
  }
function clear() {}
return {renderResearch,clear};
}
