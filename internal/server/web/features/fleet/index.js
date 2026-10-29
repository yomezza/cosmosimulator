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

export function createFleet({ roots, act, command }) {
  const n=UI.node, tr=(s)=>I18n.t(s), b=UI.button, row=UI.row, field=UI.field, choice=UI.choice;
let cargoCategory = "all";
let cargoSort = "name";
let yardQuery = "";
let yardLocation = "";
let yardClass = "all";
let yardRole = "all";
let yardPage = 1;
function cargoMenu(id, q, tile, e) {
    e.preventDefault();
    const r = tile.getBoundingClientRect();
    UI.menu(
      [
        { heading: res(id)?.name || id },
        { label: "Информация", action: () => UI.resource(id) },
        {
          label: "Переместить ресурс",
          action: () =>
            Logistics.chooseDestination(
              { kind: "ship", ship: d().ship.id },
              id,
              q,
            ),
        },
        { label: "Руководство пилота", action: () => Game.openGuide(id) },
        {
          label: "Заправить",
          disabled:
            id !== "agni" || fly(d().ship) || d().ship.fuel >= fuelCapacity(),
          reason:
            id !== "agni"
              ? "Этот ресурс не является топливом"
              : refuelReason(d().ship),
          action: () => performExpedition({ action: "refuel" }),
        },
        {
          label: "Продать",
          disabled: !d().docked,
          reason: "Для торговли требуется стыковка с портом",
          action: () =>
            actionAmount("Продажа груза", "Количество", q, (quantity) =>
              performExpedition({ action: "sell", resource: id, quantity }),
            ),
        },
      ],
      e.clientX || r.x,
      e.clientY || r.bottom,
    );
  }
function renderCargo() {
    if (Logistics?.dragging) return;
    const root = roots.cargo;
    reset(root);
    Modules.title("cargo", tr("Трюм") + " · " + UI.shipName(d().ship));
    const used = Object.values(d().ship.cargo).reduce((a, b) => a + b, 0),
      reserved = d().job?.quantity || 0;
    root.append(UI.bar(used + reserved, cargoCapacity(), "Вместимость"));
    row(root, "Занято", num(used));
    row(root, "Зарезервировано", num(reserved));
    const tools = n("div", undefined, "inline-controls");
    choice(
      tools,
      "Категория",
      [
        ["all", "Все категории"],
        ...new Set(d().resources.map((r) => r.kind)),
      ].map((x) => (Array.isArray(x) ? x : [x, x])),
      cargoCategory,
      (v) => {
        cargoCategory = v;
        renderCargo();
      },
    );
    choice(
      tools,
      "Сортировка",
      [
        ["name", "Название"],
        ["quantity", "Количество"],
        ["rarity", "Редкость"],
      ],
      cargoSort,
      (v) => {
        cargoSort = v;
        renderCargo();
      },
    );
    root.append(tools);
    Logistics?.selectionToolbar(root);
    const grid = n("div", undefined, "cargo-grid");
    const items = Object.entries(d().ship.cargo).filter(
      ([id, q]) =>
        q > 0 && (cargoCategory === "all" || res(id)?.kind === cargoCategory),
    );
    items.sort((a, b) =>
      cargoSort === "quantity"
        ? b[1] - a[1]
        : cargoSort === "rarity"
          ? tr(res(a[0]).rarity).localeCompare(tr(res(b[0]).rarity))
          : tr(res(a[0]).name).localeCompare(tr(res(b[0]).name)),
    );
    for (const [id, q] of items) {
      const r = res(id),
        tile = b("", (e) => cargoMenu(id, q, tile, e));
      tile.className = "cargo-tile" + (r.rarity === "Редкий" ? " rare" : "");
      tile.dataset.resource = id;
      tile.append(n("small", tr(r.kind)), n("b", r.code), n("span", num(q)));
      tile.oncontextmenu = (e) => cargoMenu(id, q, tile, e);
      Logistics?.draggable(
        tile,
        { kind: "ship", ship: d().ship.id },
        id,
        q,
      );
      grid.append(tile);
    }
    if (!items.length) UI.empty(grid, "Трюм пуст");
    Logistics?.dropzone(root, { kind: "ship", ship: d().ship.id });
    root.append(
      grid,
      b(
        Modules.linked("cargo") ? "Отделить окно" : "Связать с кораблём",
        () => {
          Modules.link("ship", "cargo", !Modules.linked("cargo"));
          renderCargo();
          Modules.arrangeShip();
        },
      ),
    );
  }
function refuelReason(ship) {
    if (fly(ship)) return "Корабль в пути";
    if (ship.fuel >= ship.spec.fuel) return "Бак полон";
    if (!ship.cargo.agni) return "Нужен водород в трюме выбранного корабля";
    return "1 ед. водорода = до 5 единиц топлива";
  }
function renderShip() {
    const container = roots.ship,
      s = d().ship;
    let layout = container.querySelector(".ship-inline-layout");
    if (!layout) {
      container.replaceChildren();
      layout = n("div", undefined, "ship-inline-layout");
      const projection = n("aside", undefined, "ship-projection"),
        model = UI.hologram(
          s.spec.model,
          tr("Голограмма корабля"),
          "ship-inline-model",
        ),
        caption = n("p", undefined, "ship-projection-caption"),
        controls = n("section", undefined, "ship-controls"),
        information = n("section", undefined, "ship-information");
      model.draggable = false;
      projection.append(model, caption, controls);
      layout.append(projection, information);
      container.append(layout);
    }
    const model = layout.querySelector(".ship-inline-model");
    model.dataset.ship = String(s.id);
    UI.updateHologram(
      model,
      s.spec.model,
      tr("Голограмма корабля") + " · " + tr(s.spec.name),
      fly(s),
    );
    layout.querySelector(".ship-projection-caption").textContent =
      tr(s.spec.name) + " · " + tr(s.spec.role);
    const root = layout.querySelector(".ship-information"),
      controls = layout.querySelector(".ship-controls");
    reset(root);
    reset(controls);
    Modules.title(
      "ship",
      tr("Корабль") + " · " + UI.shipName(s) + " · " + d().system.name,
    );
    const ships = Session.data.fleet.ships;
    row(root, "Лимит флота", ships.length + " / " + Session.data.fleet.ship_limit);
    choice(
      controls,
      "Корабль",
      ships.map((s) => [
        s.id,
        UI.shipName(s) +
          " · " +
          (world.data?.stars.find((star) => star.id === s.star)?.name ||
            s.star),
      ]),
      s.id,
      (id) => command({ action: "select_ship", ship: Number(id) }),
    );
    const meters = n("div", undefined, "two-columns");
    meters.append(
      UI.bar(s.fuel, s.spec.fuel, "Топливо"),
      UI.bar(
        Object.values(s.cargo).reduce((a, b) => a + b, 0) +
          (d().job?.quantity || 0),
        s.spec.cargo,
        "Груз",
      ),
    );
    root.append(meters);
    const identity = n("div", undefined, "ship-identity"),
      rename = field(identity, "Название корабля", "ship-rename", s.name);
    rename.maxLength = 40;
    identity.append(
      b("Переименовать корабль", () =>
        command({ action: "rename_ship", ship: s.id, name: rename.value }),
      ),
    );
    controls.append(identity);
    row(root, "Класс корабля", tr(s.spec.role));
    row(root, "Длина корпуса", s.spec.length + " " + tr("м"));
    row(root, "Время перехода", s.spec.jump_seconds + " s");
    row(root, "Расход за переход", s.spec.jump_fuel);
    const mining = n("b", undefined, "ship-mining");
    mining.textContent =
      s.mining.current ??
      (s.mining.min === s.mining.max
        ? s.mining.min
        : `${s.mining.min}–${s.mining.max}`);
    mining.tabIndex = 0;
    mining.dataset.help =
      s.mining.current == null
        ? "Добыча за цикл с установленным добытчиком. Вне совместимого месторождения показан диапазон; у месторождения — точное значение. Остаток ресурсов и свободное место в трюме могут ограничить добычу."
        : "Добыча за цикл с учётом корпуса и установленного добытчика. Для специализированного добытчика учтены условия текущего месторождения. Остаток ресурсов и свободное место в трюме могут ограничить добычу.";
    row(root, "Добыча", mining);
    row(root, "Местоположение", d().system.name + " · " + tr(d().region));
    row(
      root,
      "Статус",
      tr(
        s.destination
          ? "Гиперпереход"
          : s.local_target
            ? "Манёвр"
            : s.position_mode,
      ),
    );
    row(
      root,
      "Положение",
      station()
        ? UI.stationRef(s.star, s.object_id)
        : d().sites.find((v) => v.id === s.object_id)?.name ||
            tr("Точка гиперперехода"),
    );
    row(root, "Зарезервировано топлива", s.reserved_fuel);
    const tools = n("div", undefined, "inline-controls");
    tools.append(
      b("Трюм", () => {
        Modules.link("ship", "cargo", true);
        Modules.open("cargo");
        Modules.arrangeShip();
      }),
      b("Найти на карте", () => {
        window.dispatchEvent(new CustomEvent("findship", { detail: s.id }));
        Modules.open("galaxy");
      }),
      b(
        s.fuel >= s.spec.fuel ? "Бак полон" : "Заправить",
        () => performExpedition({ action: "refuel", ship: s.id }),
        fly(s) || s.fuel >= s.spec.fuel || !s.cargo.agni,
        refuelReason(s),
      ),
    );
    controls.append(tools);
    const equip = UI.fold("ship:equipment");
    equip.append(n("summary", tr("Оборудование")));
    choice(
      equip,
      "Добытчик",
      [
        [
          "universal",
          tr("Базовый добытчик") + " · " + s.spec.mining + " " + tr("ед./цикл"),
        ],
        ["ore", "Рудный добытчик"],
        ["ice", "Ледовый добытчик"],
        ["gas", "Газовый добытчик"],
      ],
      s.extractor,
      (target) =>
        UI.confirm("Установка добытчика", tr("Стоимость") + ": 250 CR", () =>
          command({ action: "extractor", target }),
        ),
    );
    equip.append(
      n(
        "p",
        tr(
          "Установка в порту. Итоговая добыча учитывает корпус, установленный добытчик и условия месторождения.",
        ),
        "muted",
      ),
    );
    controls.append(equip);
    controls.append(
      b("Верфи", () => Game.openShipyards(d().ship.star, d().ship.object_id)),
    );
    const shipActions = n("div", undefined, "inline-controls ship-actions");
    controls.append(shipActions);
    shipActions.append(
      b(
        "Эвакуация",
        () =>
          UI.confirm(
            "Эвакуация",
            tr("Буксировка к ближайшему доступному порту") +
              " · " +
              money(Session.data.clock.settings.rescue_cost) +
              " · 30 s",
            () => command({ action: "rescue" }),
          ),
        fly(s),
      ),
      b("Задания", () => Modules.open("tasks")),
    );
  }
function shipStats(root, c) {
    row(root, "Класс корабля", tr(c.role));
    row(root, "Требуемый стаж", c.required_level);
    row(root, "Длина корпуса", c.length + " " + tr("м"));
    row(root, "Вместимость", c.cargo);
    row(root, "Топливный бак", c.fuel);
    row(root, "Время перехода", c.jump_seconds + " s");
    row(root, "Расход за переход", c.jump_fuel);
    const mining = n("b", c.mining);
    mining.tabIndex = 0;
    mining.dataset.help =
      "Добыча за цикл в базовой комплектации. Установленный добытчик может изменить значение.";
    row(root, "Добыча", mining);
  }
function renderShipyards(root) {
    root.append(
      n(
        "p",
        tr(
          "Корабли продаются только на верфях. Набор моделей и количество различаются; покупка требует стыковки с выбранной верфью.",
        ),
        "muted",
      ),
    );
    const layout = n("div", undefined, "shipyard-layout"),
      left = n("section"),
      right = n("section", undefined, "shipyard-models");
    layout.append(left, right);
    root.append(layout);
    const search = field(
      left,
      "Поиск верфи или модели",
      "yard-search",
      yardQuery,
      "search",
    );
    choice(
      left,
      "Модель корабля",
      [
        ["all", "Все модели"],
        ...(w().ship_classes || []).map((c) => [c.id, c.name]),
      ],
      yardClass,
      (value) => {
        yardClass = value;
        yardPage = 1;
        draw();
      },
    );
    const list = n("div", undefined, "shipyard-list"),
      pagination = n("div", undefined, "pagination");
    left.append(list, pagination);
    const classes = w().ship_classes || [],
      yards = new Map();
    choice(
      left,
      "Назначение корабля",
      [
        ["all", "Все назначения"],
        ...[...new Set(classes.map((c) => c.role))].map((role) => [role, role]),
      ],
      yardRole,
      (value) => {
        yardRole = value;
        yardPage = 1;
        draw();
      },
    );
    for (const offer of w().shipyards || []) {
      const key = offer.star + "|" + offer.station;
      if (!yards.has(key))
        yards.set(key, {
          key,
          star: offer.star,
          station: offer.station,
          name: offer.station_name,
          offers: [],
        });
      yards.get(key).offers.push(offer);
    }
    function draw() {
      list.replaceChildren();
      right.replaceChildren();
      pagination.replaceChildren();
      const q = yardQuery.toLowerCase().trim(),
        filtered = [...yards.values()].filter((y) => {
          const models = y.offers
            .map((o) => classes.find((c) => c.id === o.ship_class))
            .filter(Boolean);
          return (
            models.some(
              (c) =>
                (yardClass === "all" || c.id === yardClass) &&
                (yardRole === "all" || c.role === yardRole),
            ) &&
            [
              y.name,
              String(y.star),
              world.data?.stars.find((s) => s.id === y.star)?.name || "",
              ...models.flatMap((c) => [tr(c.name), tr(c.role)]),
            ].some((v) => v.toLowerCase().includes(q))
          );
        });
      const pages = Math.max(1, Math.ceil(filtered.length / 8));
      yardPage = Math.min(yardPage, pages);
      if (!filtered.some((y) => y.key === yardLocation))
        yardLocation =
          filtered.find(
            (y) => y.star === d().ship.star && y.station === d().ship.object_id,
          )?.key ||
          filtered[0]?.key ||
          "";
      for (const y of filtered.slice((yardPage - 1) * 8, yardPage * 8)) {
        const button = b("", () => {
          yardLocation = y.key;
          draw();
        });
        button.className = "shipyard-location";
        button.classList.toggle("selected", y.key === yardLocation);
        const available = y.offers.reduce((sum, o) => sum + o.quantity, 0);
        button.append(
          n("span", String(y.star), "system-id"),
          UI.stationLabel(y.star, y.station, y.name),
          n("small", num(available)),
        );
        list.append(button);
      }
      pagination.append(
        b(
          "‹",
          () => {
            yardPage--;
            draw();
          },
          yardPage === 1,
        ),
        n("span", yardPage + " / " + pages),
        b(
          "›",
          () => {
            yardPage++;
            draw();
          },
          yardPage === pages,
        ),
      );
      if (!filtered.length) {
        UI.empty(list, "Ничего не найдено");
        return;
      }
      const y = filtered.find((v) => v.key === yardLocation),
        here =
          d().docked &&
          d().ship.star === y.star &&
          d().ship.object_id === y.station;
      const yardTitle = n("h3");
      yardTitle.append(UI.stationLabel(y.star, y.station, y.name));
      right.append(yardTitle);
      const route = b("Построить маршрут", () => {
        Modules.open("galaxy");
        window.dispatchEvent(new CustomEvent("showstar", { detail: y.star }));
      });
      right.append(route);
      const cards = n("div", undefined, "shipyard-cards");
      right.append(cards);
      for (const offer of y.offers) {
        const c = classes.find((c) => c.id === offer.ship_class);
        if (
          !c ||
          (yardClass !== "all" && c.id !== yardClass) ||
          (yardRole !== "all" && c.role !== yardRole)
        )
          continue;
        const card = n("article", undefined, "shipyard-card"),
          model = UI.hologram(c.model, tr(c.name));
        card.dataset.hull = c.id;
        card.append(
          model,
          n("h3", tr(c.name)),
          n("p", tr(c.description), "muted"),
        );
        shipStats(card, c);
        row(card, "В наличии", num(offer.quantity));
        row(card, "Стоимость", money(offer.price));
        card.append(
          b(
            "Купить корабль",
            () =>
              UI.confirm(
                "Покупка корабля",
                tr(c.name) + " · " + money(offer.price) + " · " + y.name,
                () =>
                  command({
                    action: "buy_ship",
                    target: c.id,
                    price: offer.price,
                    star: y.star,
                    station: y.station,
                  }),
              ),
            !here ||
              offer.quantity < 1 ||
              d().credits < offer.price ||
              Session.data.profile.level < c.required_level ||
              Session.data.fleet.ships.length >= Session.data.fleet.ship_limit,
            !here
              ? "Прибудьте на выбранную верфь"
              : offer.quantity < 1
                ? "На верфи нет доступных кораблей"
                : Session.data.profile.level < c.required_level
                  ? "Недостаточный стаж для покупки"
                  : Session.data.fleet.ships.length >= Session.data.fleet.ship_limit
                    ? "Достигнут лимит кораблей по стажу"
                    : d().credits < offer.price
                      ? "Недостаточно кредитов"
                      : "",
          ),
        );
        cards.append(card);
      }
    }
    search.oninput = () => {
      yardQuery = search.value;
      yardPage = 1;
      draw();
    };
    draw();
  }
UI.shipActivity = (card, s, hasQueue = false) => {
    if (s.destination)
      card.append(
        n(
          "p",
          `${world.data?.stars[s.star - 1]?.name || s.star} → ${world.data?.stars[s.destination - 1]?.name || s.destination} · ${fmtTime(s.arrival)}`,
        ),
      );
    if (s.local_target)
      card.append(
        n(
          "p",
          tr("Манёвр") +
            " → " +
            s.local_target +
            " · " +
            fmtTime(s.local_arrival),
        ),
      );
    if (s.waypoints?.length)
      card.append(
        n(
          "p",
          tr("Маршрут") +
            ": " +
            s.waypoints
              .map((id) => world.data?.stars[id - 1]?.name || id)
              .join(" → "),
        ),
        b("Отменить оставшийся маршрут", () =>
          command({ action: "cancel_route", ship: s.id }),
        ),
      );
    const research = w().research?.find((j) => j.ship === s.id);
    if (research) {
      card.append(
        UI.bar(research.scanned, research.total, "Объекты"),
        n("p", tr(research.paused ? "Приостановлено" : "Сканирование")),
        b(research.paused ? "Продолжить" : "Приостановить", () =>
          command({
            action: research.paused ? "resume_research" : "pause_research",
            ship: s.id,
          }),
        ),
      );
    }
    for (const job of (w().mining || []).filter((j) => j.ship_id === s.id))
      card.append(
        n(
          "p",
          tr("Добыча") +
            " · " +
            tr(res(job.resource).name) +
            " × " +
            job.quantity,
        ),
      );
    if (
      !hasQueue &&
      !fly(s) &&
      !research &&
      !w().mining?.some((j) => j.ship_id === s.id)
    )
      card.append(n("p", tr("Ожидает задания"), "muted"));
  };
function clear() {yardQuery="";yardLocation="";yardClass="all";yardRole="all";yardPage=1;}
return {cargoMenu,renderCargo,refuelReason,renderShip,shipStats,renderShipyards,clear, filterShipyards({classID=yardClass,role=yardRole,query=yardQuery}={}) {yardClass=classID;yardRole=role;yardQuery=query;yardPage=1;}, setYardLocation(value){yardLocation=value;}};
}
