import { createFleet } from "./features/fleet/index.js";
import { createMining } from "./features/mining/index.js";
import { createMarkets } from "./features/markets/index.js";
import { createCorporations } from "./features/corporations/index.js";
import { createProfile } from "./features/profile/index.js";
import { createAdministrationUsers } from "./features/administration/users/index.js";
import { createAdministrationGameplay } from "./features/administration/gameplay/index.js";
import { createAdministrationBenchmark } from "./features/administration/benchmark/index.js";
import { createNotifications } from "./features/notifications/index.js";
import { createGuide } from "./features/guide/index.js";
import { createAutomation } from "./features/automation/index.js";
import { createFinance } from "./features/finance/index.js";
import { createPortal } from "./features/portal/index.js";
import { Session, RequestScope } from "./core/session.js";
import { scopedView } from "./core/scoped-view.js";
import { I18n } from "./core/i18n.js";
import { Format } from "./core/format.js";
import { api } from "./core/api.js";
import { refresh } from "./core/snapshot-store.js";
import { UI, Modules } from "./ui/runtime.js";
import { notify } from "./ui/feedback.js";
import { Game, GalaxyAdmin, Scheduler, AdminResources, Logistics, scenes } from "./features/runtime.js";
import { world, loadWorld } from "./core/world-store.js";
import { performExpedition } from "./features/commands.js";
import { d, w, res, num, station, fmtTime, money, known, fly, cargoCapacity, fuelCapacity } from "./features/selectors.js";
import { reset } from "./ui/view-state.js";
import { actionAmount } from "./ui/quantity.js";
import { resourceName } from "./features/inventory/resource-label.js";
import { can } from "./features/corporations/permissions.js";
export function initExpedition() {

  const n = UI.node,
    tr = (s) => I18n.t(s),
    b = UI.button,
    row = UI.row,
    field = UI.field,
    choice = UI.choice;
  const ids = [
      "ship",
      "cargo",
      "research",
      "market",
      "corporations",
      "profile",
      "users",
      "gameplay",
      "galaxies",
      "adminchanges",
      "notifications",
      "guide",
      "tasks",
      "finance",
      "benchmark",
      "portal",
    ],
    roots = Object.fromEntries(
      ids.map((id) => [
        id,
        document.querySelector("#window-" + id + " .module-content"),
      ]),
    );



  async function act(path, body, message = "Действие выполнено") {
    if (busy) return false;
    const epoch = Session.epoch;
    busy = true;
    try {
      await api(path, body);
      if (epoch !== Session.epoch || !Session.signedIn) return false;
      notify(message);
      await refresh(true);
      return true;
    } catch (error) {
      if (epoch !== Session.epoch) return false;
      notify(error.message, "error");
      if (error.status === 409) await refresh(true);
      return false;
    } finally {
      if (epoch === Session.epoch) { busy = false; renderVisible(true); }
    }
  }
  let busy=false;
  const command = (body, message) => act("workbench", body, message);
const fleetFeature=createFleet({roots:{ship:roots.ship,cargo:roots.cargo},act,command});
const miningFeature=createMining({roots:{research:roots.research},act,command});
const marketsFeature=createMarkets({roots:{market:roots.market},act,command,renderShipyards:fleetFeature.renderShipyards});
const corporationsFeature=createCorporations({roots:{corporations:roots.corporations},act,command});
const profileFeature=createProfile({roots:{profile:roots.profile},act,command});
const usersFeature=createAdministrationUsers({roots:{users:roots.users},act,command});
const gameplayFeature=createAdministrationGameplay({roots:{gameplay:roots.gameplay},act,command});
const benchmarkFeature=createAdministrationBenchmark({roots:{benchmark:roots.benchmark},act,command});
const notificationsFeature=createNotifications({roots:{notifications:roots.notifications},act,command});
const guideFeature=createGuide({roots:{guide:roots.guide},act,command,shipStats:fleetFeature.shipStats,filterShipyards:fleetFeature.filterShipyards});
const automationFeature=createAutomation({roots:{tasks:roots.tasks},act,command,deliveryDetails:marketsFeature.deliveryDetails});
const financeFeature=createFinance({roots:{finance:roots.finance},act,command});
const portalFeature=createPortal({roots:{portal:roots.portal},act,command});
const cargoMenu=(...args)=>fleetFeature.cargoMenu(...args);
const renderCargo=(...args)=>fleetFeature.renderCargo(...args);
const refuelReason=(...args)=>fleetFeature.refuelReason(...args);
const renderShip=(...args)=>fleetFeature.renderShip(...args);
const shipStats=(...args)=>fleetFeature.shipStats(...args);
const renderShipyards=(...args)=>fleetFeature.renderShipyards(...args);
const renderResearch=(...args)=>miningFeature.renderResearch(...args);
const tradeForm=(...args)=>marketsFeature.tradeForm(...args);
const renderMarket=(...args)=>marketsFeature.renderMarket(...args);
const renderDeliveries=(...args)=>marketsFeature.renderDeliveries(...args);
const deliveryDetails=(...args)=>marketsFeature.deliveryDetails(...args);
const flagPicker=(...args)=>corporationsFeature.flagPicker(...args);
const renderCompanies=(...args)=>corporationsFeature.renderCompanies(...args);
const renderInvitations=(...args)=>corporationsFeature.renderInvitations(...args);
const renderCorpCreate=(...args)=>corporationsFeature.renderCorpCreate(...args);
const renderFlagDesign=(...args)=>corporationsFeature.renderFlagDesign(...args);
const renderProjects=(...args)=>corporationsFeature.renderProjects(...args);
const renderProfile=(...args)=>profileFeature.renderProfile(...args);
const renderUsers=(...args)=>usersFeature.renderUsers(...args);
const editUser=(...args)=>usersFeature.editUser(...args);
const renderGameplay=(...args)=>gameplayFeature.renderGameplay(...args);
const renderBenchmark=(...args)=>benchmarkFeature.renderBenchmark(...args);
const eventText=(...args)=>notificationsFeature.eventText(...args);
const loadNotifications=(...args)=>notificationsFeature.loadNotifications(...args);
const notificationBadge=(...args)=>notificationsFeature.notificationBadge(...args);
const renderNotifications=(...args)=>notificationsFeature.renderNotifications(...args);
const renderGuide=(...args)=>guideFeature.renderGuide(...args);
const renderTasks=(...args)=>automationFeature.renderTasks(...args);
const ledgerAmount=(...args)=>financeFeature.ledgerAmount(...args);
const renderFinance=(...args)=>financeFeature.renderFinance(...args);
const renderPortal=(...args)=>portalFeature.renderPortal(...args);

  Object.assign(Game, {
    command,
    teleport(to) {
      return act(
        "admin/teleport",
        { to, ship: d().ship.id },
        "Телепортация выполнена",
      );
    },
    async explore(to) {
      const plan = await api("estimate?to=" + to);
      const text =
        (plan.path
          ?.map(
            (id) => world.data?.stars.find((s) => s.id === id)?.name || id,
          )
          .join(" → ") || tr("Нет маршрута")) +
        " · " +
        tr("Объекты") +
        ": " +
        plan.remaining +
        "/" +
        plan.objects +
        " · " +
        tr("Время") +
        ": " +
        (plan.travel_seconds + plan.scan_seconds) +
        " s · " +
        tr("Топливо") +
        ": " +
        plan.fuel +
        " · " +
        money(plan.cost) +
        (d().ship.fuel < plan.return_fuel
          ? " · " + tr("Топлива на обратный путь может не хватить")
          : "");
      UI.confirm("План экспедиции", text, () =>
        command({ action: "explore", to }),
      );
      if (!plan.can_depart) {
        document.querySelector(
          "#window-confirm .module-content button",
        ).disabled = true;
      }
    },
    known,
    openGuide(id) {
      guideFeature.open(id);
    },
    openShipyards(star, stationID) {
      marketsFeature.show("shipyards");
      if (star) fleetFeature.setYardLocation(star + "|" + stationID);
      Modules.open("market");
    },
    openTasks() {
      Modules.open("tasks");
    },
  });
  document.getElementById("nav-shipyards").onclick = () => Game.openShipyards();
  Game.performExpedition = (body) =>
    act(
      "expedition",
      { ship: d().ship.id, ...body },
      body.action === "mine"
        ? "Добыча запущена: груз поступит на следующем тике"
        : "Действие выполнено",
    );
  async function ensureMap() {
    if (!world.data) await loadWorld();
  }












































  const renderers = {
    ship: renderShip,
    cargo: renderCargo,
    research: renderResearch,
    market: renderMarket,
    corporations: renderCompanies,
    profile: renderProfile,
    users: renderUsers,
    gameplay: renderGameplay,
    galaxies: () => GalaxyAdmin.render(roots.galaxies),
    adminchanges: () => GalaxyAdmin.logs(roots.adminchanges),
    notifications: renderNotifications,
    guide: renderGuide,
    tasks: renderTasks,
    finance: renderFinance,
    benchmark: renderBenchmark,
    portal: renderPortal,
  };
  const onDemand = new Set([
    "users",
    "gameplay",
    "galaxies",
    "adminchanges",
    "guide",
    "corporations",
  ]);
  function renderVisible(force = false) {
    if (!Session.signedIn || !d()) return;
    for (const [id, render] of Object.entries(renderers)) {
      if (!Modules.isOpen(id) || (!force && onDemand.has(id))) continue;
      const root = roots[id];
      if (id === "tasks") {
        render();
        continue;
      }
      if (
        root.querySelector('[data-dirty="true"]') ||
        (!force &&
          root.contains(document.activeElement) &&
          document.activeElement.matches("input,select,.picker-button")) ||
        document.querySelector(".picker-popover")
      )
        continue;
      const scroll = root.scrollTop,
        epoch = Session.epoch,
        inputs = [...root.querySelectorAll("input[id]")].map((el) => ({
          id: el.id,
          value: el.value,
          checked: el.checked,
        })),
        picks = [...root.querySelectorAll("[data-field].picker-button")].map(
          (el) => ({ key: el.dataset.field, value: el.value }),
        );
      Promise.resolve(render())
        .then((accepted) => {
          if (accepted === false || epoch !== Session.epoch) return;
          root.scrollTop = scroll;
          if (!force) {
            for (const value of inputs) {
              const el = document.getElementById(value.id);
              if (el) {
                el.value = value.value;
                el.checked = value.checked;
                el.dispatchEvent(new Event("input"));
              }
            }
            for (const value of picks) {
              const el = [
                ...root.querySelectorAll("[data-field].picker-button"),
              ].find((el) => el.dataset.field === value.key);
              if (el && el.value !== value.value) el.setValue(value.value, { emit: true });
            }
          }
        })
        .catch((error) => notify(error.message, "error"));
    }
  }
  window.addEventListener("moduleopen", async (e) => {
    if (!Session.signedIn) return;
    if (
      ["users", "gameplay", "benchmark", "galaxies", "adminchanges"].includes(
        e.detail,
      ) &&
      Session.data.state?.user.role !== "admin"
    ) {
      Modules.close(e.detail);
      return;
    }
    const epoch = Session.epoch;
    try {
      await ensureMap();
      if (epoch !== Session.epoch || !Session.signedIn || !Modules.isOpen(e.detail)) return;
      await renderers[e.detail]?.();
      if (epoch !== Session.epoch || !Session.signedIn || !Modules.isOpen(e.detail)) return;
      if (e.detail === "notifications") {
        const dialog = document.getElementById("window-notifications");
        if (!dialog.dataset.positioned) {
          dialog.style.left =
            Math.max(6, innerWidth - dialog.offsetWidth - 12) + "px";
          dialog.style.top = "65px";
          dialog.dataset.positioned = "true";
        }
      }
    } catch (error) {
      notify(error.message, "error");
    }
  });
  window.addEventListener("gamedata", () => {
    const model = roots.ship.querySelector(".ship-inline-model"),
      vessel =
        model &&
        (Session.data.fleet?.ships || []).find(
          (s) => s.id === Number(model.dataset.ship),
        );
    if (vessel)
      UI.updateHologram(
        model,
        vessel.spec.model,
        tr("Голограмма корабля") + " · " + tr(vessel.spec.name),
        fly(vessel),
      );
    notificationBadge();
    renderVisible();
    const population = w().population?.[0];
    document.getElementById("server-status").dataset.help = "Сервер";
    document.getElementById("server-status").tabIndex = 0;
    document.getElementById("server-status").dataset.helpRaw =
      tr("Сервер") +
      ": " +
      tr(w().server_name || "Иггдрасиль") +
      "\n" +
      tr("Игроки онлайн") +
      ": " +
      (population?.players || 0) +
      "\n" +
      tr("Боты") +
      ": " +
      (population?.bots || 0);
  });
  window.addEventListener("notificationchange", () =>
    loadNotifications().catch(() => {}),
  );
  window.addEventListener("languagechange", () => renderVisible(true));
  window.addEventListener("moduleclose", (e) => {
    if(e.detail === "market") marketsFeature.close();
    ({users:usersFeature,gameplay:gameplayFeature,benchmark:benchmarkFeature,corporations:corporationsFeature})[e.detail]?.close?.();
  });
  window.addEventListener("signedout", () => { busy=false; fleetFeature.clear(); miningFeature.clear(); marketsFeature.clear(); corporationsFeature.clear(); profileFeature.clear(); usersFeature.clear(); gameplayFeature.clear(); benchmarkFeature.clear(); notificationsFeature.clear(); guideFeature.clear(); automationFeature.clear(); financeFeature.clear(); portalFeature.clear(); });
  window.addEventListener("stationmarket", () => marketsFeature.show("local"));
  setInterval(() => {
    document.querySelectorAll("[data-reset-at]").forEach((el) => {
      const sec = Math.max(
        0,
        Math.ceil(
          (new Date(el.dataset.resetAt).getTime() -
            Date.now() -
            (Session.data.serverOffset || 0)) /
            1000,
        ),
      );
      el.textContent =
        num(el.dataset.remaining) +
        " / " +
        num(el.dataset.capacity) +
        " · " +
        [Math.floor(sec / 3600), Math.floor(sec / 60) % 60, sec % 60]
          .map((v) => String(v).padStart(2, "0"))
          .join(":");
    });
    document
      .querySelectorAll("[data-job-started]")
      .forEach(
        (el) =>
          (el.value = Math.min(
            99,
            Math.max(
              1,
              ((Date.now() +
                (Session.data.serverOffset || 0) -
                new Date(el.dataset.jobStarted).getTime()) /
                Math.max(
                  1,
                  new Date(el.dataset.jobEnd).getTime() -
                    new Date(el.dataset.jobStarted).getTime(),
                )) *
                100,
            ),
          )),
      );
  }, 500);

}
