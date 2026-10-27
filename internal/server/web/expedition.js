"use strict";
(() => {
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
      "events",
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
  let busy = false,
    marketTab = "local",
    corpTab = "overview",
    corpScope = "all",
    userPage = 1,
    userQuery = "",
    guideID = "iron",
    guideQuery = "",
    selectedResource = "agni",
    selectedOrder = null,
    selectedCorp = null,
    notifySelected = new Set(),
    benchmarkSelected = null,
    cargoCategory = "all",
    cargoSort = "name",
    noticeCursor = null,
    requestID = 0;
  const d = () => gameData.expedition,
    w = () => gameData.workbench || {},
    res = (id) => d()?.resources.find((r) => r.id === id),
    num = (v) => Number(v || 0).toLocaleString(I18n.language),
    station = () =>
      w().stations?.find(
        (s) => s.star === d()?.ship.star && s.object_id === d()?.ship.object_id,
      ),
    fmtTime = (value) => new Date(value).toLocaleString(I18n.language),
    money = (v) => num(v) + " CR",
    known = (id) => w().known?.some((v) => v.star === id),
    fly = (s) => !!s.destination || !!s.local_target;
  function reset(root) {
    const scroll = root.scrollTop,
      folds = new Map(
        [...root.querySelectorAll("details")].map((el, i) => [
          (el.querySelector("summary")?.textContent || "") + "|" + i,
          el.open,
        ]),
      );
    root.replaceChildren();
    queueMicrotask(() => {
      root.querySelectorAll("details").forEach((el, i) => {
        const key = (el.querySelector("summary")?.textContent || "") + "|" + i;
        if (folds.has(key)) el.open = folds.get(key);
      });
      root.scrollTop = scroll;
    });
  }
  async function act(path, body, message = "Действие выполнено") {
    if (busy) return false;
    busy = true;
    try {
      await api(path, body);
      notify(message);
      await refresh(true);
      return true;
    } catch (error) {
      notify(error.message, "error");
      if (error.status === 409) await refresh(true);
      return false;
    } finally {
      busy = false;
      renderVisible(true);
    }
  }
  const command = (body, message) => act("workbench", body, message);
  window.Game = {
    command,
    async explore(to) {
      const plan = await api("estimate?to=" + to);
      const text =
        (plan.path
          ?.map(
            (id) => window.MapData?.stars.find((s) => s.id === id)?.name || id,
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
      guideID = id;
      Modules.open("guide");
    },
    openTasks() {
      Modules.open("tasks");
    },
  };
  window.performExpedition = (body) =>
    act(
      "expedition",
      body,
      body.action === "mine"
        ? "Добыча запущена: груз поступит на следующем тике"
        : "Действие выполнено",
    );
  async function ensureMap() {
    if (!window.MapData) window.MapData = await api("galaxy");
  }
  function resourceName(id) {
    const r = res(id),
      span = n("span", tr(r?.name || id));
    span.dataset.resource = id;
    return span;
  }
  function actionAmount(title, label, max, run, initial = 1) {
    const root = Modules.create("amount", title, 420, 330);
    reset(root);
    const q = field(root, label, "action-quantity", initial, "number");
    q.min = 1;
    q.max = max;
    const result = n("p", undefined, "muted");
    root.append(result);
    const submit = b("Подтвердить", async () => {
      if (await run(Number(q.value))) Modules.close("amount");
    });
    const update = () => {
      submit.disabled =
        !Number.isInteger(Number(q.value)) ||
        Number(q.value) < 1 ||
        Number(q.value) > max;
      result.textContent = tr("Доступно") + ": " + num(max);
    };
    q.oninput = update;
    root.append(submit);
    update();
    Modules.open("amount");
  }
  function cargoMenu(id, q, tile, e) {
    e.preventDefault();
    tile.classList.add("selected");
    const r = tile.getBoundingClientRect();
    UI.menu(
      [
        { heading: res(id)?.name || id },
        { label: "Информация", action: () => UI.resource(id) },
        { label: "Руководство пилота", action: () => Game.openGuide(id) },
        {
          label: "Заправить",
          disabled: id !== "agni" || fly(d().ship) || d().ship.fuel >= 40,
          reason:
            id !== "agni"
              ? "Этот ресурс не является топливом"
              : "Манёвр недоступен",
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
        {
          label: "На склад",
          disabled: !d().docked,
          reason: "Стыкуйтесь со станцией",
          action: () =>
            actionAmount("На склад", "Количество", q, (quantity) =>
              command({ action: "store", resource: id, quantity }),
            ),
        },
        {
          label: "На корпоративный склад",
          disabled: !d().docked || !gameData.profile.company_id,
          reason: "Завершите проект корпоративного склада",
          action: () =>
            actionAmount(
              "На корпоративный склад",
              "Количество",
              q,
              (quantity) =>
                command({ action: "company_store", resource: id, quantity }),
            ),
        },
      ],
      e.clientX || r.x,
      e.clientY || r.bottom,
    );
  }
  function renderCargo() {
    const root = roots.cargo;
    reset(root);
    Modules.title(
      "cargo",
      tr("Трюм") + " · " + tr(d().ship.name) + " #" + d().ship.id,
    );
    const used = Object.values(d().ship.cargo).reduce((a, b) => a + b, 0),
      reserved = d().job?.quantity || 0;
    root.append(UI.bar(used + reserved, 60, "Вместимость"));
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
      grid.append(tile);
    }
    if (!items.length) UI.empty(grid, "Трюм пуст");
    root.append(
      grid,
      b(
        Modules.linked("cargo") ? "Отделить окно" : "Связать с кораблём",
        () => {
          Modules.link("ship", "cargo", !Modules.linked("cargo"));
          renderCargo();
        },
      ),
    );
  }
  function renderShip() {
    const root = roots.ship;
    reset(root);
    const s = d().ship;
    Modules.title(
      "ship",
      tr("Корабль") +
        " · " +
        tr(s.name) +
        " #" +
        s.id +
        " · " +
        d().system.name,
    );
    const ships = gameData.fleet.ships;
    choice(
      root,
      "Корабль",
      ships.map((s) => [
        s.id,
        tr(s.name) +
          " #" +
          s.id +
          " · " +
          (window.MapData?.stars.find((star) => star.id === s.star)?.name ||
            s.star),
      ]),
      s.id,
      (id) => command({ action: "select_ship", ship: Number(id) }),
    );
    const model = n("img");
    model.src = "pilgrim.svg";
    model.alt = tr("Корабль «Пилигрим»");
    model.className = "ship-model";
    root.append(model);
    const meters = n("div", undefined, "two-columns");
    meters.append(
      UI.bar(s.fuel, 40, "Топливо"),
      UI.bar(
        Object.values(s.cargo).reduce((a, b) => a + b, 0) +
          (d().job?.quantity || 0),
        60,
        "Груз",
      ),
    );
    root.append(meters);
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
      d().sites.find((v) => v.id === s.object_id)?.name ||
        tr("Точка гиперперехода"),
    );
    row(root, "Зарезервировано топлива", s.reserved_fuel);
    const tools = n("div", undefined, "inline-controls");
    tools.append(
      b("Трюм", () => {
        Modules.open("cargo");
        Modules.adjacent("cargo", "ship");
      }),
      b("Найти на карте", () => {
        window.dispatchEvent(new CustomEvent("findship", { detail: s.id }));
        Modules.open("galaxy");
      }),
      b(
        "Заправить",
        () => performExpedition({ action: "refuel" }),
        fly(s) || s.fuel >= 40 || !s.cargo.agni,
        "Нужен Агни в трюме",
      ),
    );
    root.append(tools);
    const equip = n("details");
    equip.append(n("summary", tr("Оборудование")));
    choice(
      equip,
      "Добытчик",
      [
        ["universal", "Базовый · до 3 единиц за цикл"],
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
          "Установка в порту. Специализированный добытчик: 4–6 единиц с учётом концентрации.",
        ),
        "muted",
      ),
    );
    root.append(equip);
    const port = station();
    if (port?.shipyard) {
      root.append(
        n("h3", tr("Верфь")),
        n(
          "p",
          tr("Пилигрим") +
            " · " +
            money(w().ship_price) +
            " · " +
            tr("В наличии") +
            ": " +
            port.ship_stock,
        ),
        b(
          "Купить корабль",
          () =>
            UI.confirm("Покупка корабля", money(w().ship_price), () =>
              command({ action: "buy_ship" }),
            ),
          !d().docked || port.ship_stock < 1 || d().credits < w().ship_price,
        ),
      );
    }
    const shipActions = n("div", undefined, "inline-controls ship-actions");
    root.append(shipActions);
    shipActions.append(
      b(
        "Эвакуация",
        () =>
          UI.confirm(
            "Эвакуация",
            tr("Буксировка к ближайшему доступному порту") +
              " · " +
              money(gameData.clock.settings.rescue_cost) +
              " · 30 s",
            () => command({ action: "rescue" }),
          ),
        fly(s),
      ),
      b("Задания", () => Modules.open("tasks")),
    );
  }
  function renderResearch() {
    const root = roots.research;
    reset(root);
    row(root, "Текущая система", d().system.name + " · " + tr(d().region));
    const layout = n("div", undefined, "research-layout"),
      main = n("section"),
      side = n("aside");
    layout.append(main, side);
    root.append(layout);
    const job = w().research?.find((j) => j.ship === d().ship.id);
    if (job) {
      main.append(
        n("h3", tr("Исследование")),
        UI.bar(job.scanned, job.total, "Объекты"),
        n(
          "p",
          tr(job.paused ? "Приостановлено" : "Сканирование") +
            " · " +
            job.seconds +
            " s / " +
            tr("Объект"),
        ),
        b(job.paused ? "Продолжить" : "Приостановить", () =>
          command({
            action: job.paused ? "resume_research" : "pause_research",
          }),
        ),
      );
    } else
      main.append(
        b(
          "Исследовать систему · 10 CR",
          () => Game.explore(d().ship.star),
          d().surveyed || fly(d().ship),
          d().surveyed ? "Система уже исследована" : "Корабль в пути",
        ),
      );
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
          const details = n("details");
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
    } else UI.empty(main, "Исследуйте систему, чтобы открыть месторождения");
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
        new Date(gameData.clock.last_tick).getTime() +
          gameData.clock.settings.tick_seconds * 1000,
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
      n("p", num(gameData.state.user.ore) + " ORE"),
      n("p", tr("1 ORE за фазу · продажа по 5 CR"), "muted"),
      b(
        "Продать учебную руду",
        () => act("sell", {}),
        gameData.state.user.ore < 1,
      ),
    );
    side.append(training);
  }
  function tradeForm(root, offer, global = false, starID) {
    const r = res(offer.resource),
      panel = n("aside", undefined, "detail-panel");
    panel.append(n("h3", tr(r.name)), n("p", tr(r.description), "muted"));
    const q = field(panel, "Количество", "trade-quantity", 1, "number");
    q.min = 1;
    q.max = Math.min(60, offer.stock);
    const quote = n("p", undefined, "trade-total"),
      capacity = n("p", undefined, "muted");
    const buy = b(global ? "Забронировать на станции" : "Купить в трюм", () =>
      global
        ? act("market/global", {
            action: "reserve",
            star: starID,
            resource: r.id,
            quantity: Number(q.value),
            expected_price: offer.buy,
          })
        : performExpedition({
            action: "buy",
            resource: r.id,
            quantity: Number(q.value),
            expected_price: offer.buy,
          }),
    );
    buy.classList.add("primary-action");
    const sell = b("Продать из трюма", () =>
      performExpedition({
        action: "sell",
        resource: r.id,
        quantity: Number(q.value),
        expected_price: offer.sell,
      }),
    );
    const update = () => {
      const count = Number(q.value),
        cost = offer.buy * count,
        used =
          Object.values(d().ship.cargo).reduce((a, b) => a + b, 0) +
          (d().job?.quantity || 0);
      quote.textContent = `${money(offer.buy)} × ${num(count)} = ${money(cost)}`;
      capacity.textContent = global
        ? tr(
            "Выдача на станции указанной системы. Полный возврат до получения; комиссия хранения — 0 CR.",
          )
        : tr("Свободно в трюме") + ": " + (60 - used);
      buy.disabled =
        !Number.isInteger(count) ||
        count < 1 ||
        count > offer.stock ||
        count > 60 ||
        cost > d().credits ||
        (!global && (!d().docked || count + used > 60));
      sell.disabled =
        global ||
        !d().docked ||
        count < 1 ||
        count > (d().ship.cargo[r.id] || 0);
    };
    q.oninput = update;
    panel.append(quote, capacity, buy);
    if (!global) panel.append(sell);
    else
      panel.append(
        n(
          "p",
          tr("Срок хранения") +
            ": " +
            gameData.clock.settings.reservation_hours +
            " h",
          "muted",
        ),
      );
    root.append(panel);
    update();
  }
  async function renderMarket() {
    const root = roots.market;
    reset(root);
    const port = station();
    root.append(
      n(
        "div",
        (port?.name || d().system.name) + " · " + d().system.name,
        "location-strip",
      ),
    );
    UI.tabs(
      root,
      [
        ["local", "Локальный рынок"],
        ["global", "Глобальный рынок"],
        ["orders", "Предложения игроков"],
        ["warehouse", "Склады"],
        ["data", "Разведданные"],
        ["delivery", "Перевозки"],
      ],
      marketTab,
      (id) => {
        marketTab = id;
        renderMarket();
      },
    );
    if (marketTab === "local") {
      if (!d().docked) {
        UI.empty(root, "Для локальной торговли пристыкуйтесь к порту.");
        return;
      }
      root.append(
        n(
          "p",
          tr(port?.specialization || "general") +
            " · " +
            tr("Тариф") +
            ": " +
            (port?.fee_percent || 0) +
            "% · " +
            tr("Бюджет станции") +
            ": " +
            money(d().market_credits),
          "muted",
        ),
      );
      const layout = n("div", undefined, "master-detail"),
        list = n("section");
      layout.append(list);
      let offer =
        d().offers.find((o) => o.resource === selectedResource) ||
        d().offers[0];
      const rows = d().offers.map((o) => {
        const link = b("", () => {
          selectedResource = o.resource;
          renderMarket();
        });
        link.append(resourceName(o.resource));
        link.classList.toggle("selected", offer?.resource === o.resource);
        return [link, num(o.stock), money(o.buy), money(o.sell)];
      });
      list.append(UI.table(["Ресурс", "В наличии", "Купить", "Продать"], rows));
      if (offer) tradeForm(layout, offer);
      root.append(layout);
    }
    if (marketTab === "global") {
      const epoch = authEpoch,
        result = await api("market/global");
      if (epoch !== authEpoch || marketTab !== "global") return;
      const eligible = result.ports.sort((a, b) => a.star - b.star);
      let target = Number(root.dataset.target || d().ship.star);
      if (!eligible.some((p) => p.star === target)) target = eligible[0]?.star;
      choice(
        root,
        "Система получения",
        eligible.map((p) => ({
          id: p.star,
          label:
            window.MapData?.stars.find((s) => s.id === p.star)?.name ||
            String(p.star),
          code: String(p.star),
        })),
        target,
        (value) => {
          root.dataset.target = value;
          renderMarket();
        },
      );
      const layout = n("div", undefined, "master-detail"),
        list = n("section");
      layout.append(list);
      const offers = eligible.find((p) => p.star === target)?.offers || [],
        offer =
          offers.find((o) => o.resource === selectedResource) || offers[0];
      list.append(
        UI.table(
          ["Ресурс", "В наличии", "Цена"],
          offers.map((o) => [
            b(tr(res(o.resource).name), () => {
              selectedResource = o.resource;
              renderMarket();
            }),
            num(o.stock),
            money(o.buy),
          ]),
        ),
      );
      if (offer) tradeForm(layout, offer, true, target);
      root.append(layout, n("h3", tr("Мои бронирования")));
      const rows = result.reservations.map((r) => {
        const controls = n("div", undefined, "inline-controls");
        if (r.status === "reserved")
          controls.append(
            b(
              "Получить груз",
              () => act("market/global", { action: "collect", id: r.id }),
              !d().docked || d().ship.star !== r.star,
              "Прибудьте на станцию получения",
            ),
            b("Отменить бронь", () =>
              act("market/global", { action: "cancel", id: r.id }),
            ),
          );
        return [
          "#" + r.id,
          r.star,
          resourceName(r.resource),
          "× " + r.quantity,
          money(r.cost),
          tr(r.status),
          r.expires_at ? fmtTime(r.expires_at) : "—",
          controls,
        ];
      });
      root.append(
        UI.table(
          [
            "ID",
            "Система",
            "Ресурс",
            "Количество",
            "Стоимость",
            "Статус",
            "Хранение до",
            "Действие",
          ],
          rows,
        ),
      );
    }
    if (marketTab === "warehouse") {
      const stores = w().warehouses || [];
      if (!stores.length) UI.empty(root, "На складах пока нет ресурсов");
      for (const store of stores) {
        root.append(n("h3", String(store.star) + " · " + store.station));
        const here =
          d().docked &&
          store.star === d().ship.star &&
          store.station === d().ship.object_id;
        root.append(
          UI.table(
            ["Ресурс", "Количество", "Действие"],
            Object.entries(store.cargo)
              .filter(([, q]) => q > 0)
              .map(([id, q]) => [
                resourceName(id),
                num(q),
                b(
                  "В трюм",
                  () =>
                    actionAmount(
                      "В трюм",
                      "Количество",
                      Math.min(q, 60),
                      (quantity) =>
                        command({ action: "withdraw", resource: id, quantity }),
                    ),
                  !here,
                  "Прибудьте на станцию хранения",
                ),
              ]),
          ),
        );
      }
      root.append(
        b("Открыть трюм", () => {
          Modules.open("cargo");
          Modules.adjacent("cargo", "market");
        }),
      );
    }
    if (marketTab === "orders") {
      const layout = n("div", undefined, "master-detail"),
        list = n("section"),
        side = n("aside", undefined, "detail-panel");
      layout.append(list, side);
      const orders = w().orders || [];
      list.append(
        UI.table(
          ["Ресурс", "Место", "Количество", "Цена"],
          orders.map((o) => [
            b(tr(res(o.resource).name), () => {
              selectedOrder = o.id;
              renderMarket();
            }),
            o.star + " · " + o.station,
            num(o.quantity),
            money(o.price),
          ]),
        ),
      );
      const selected = orders.find((o) => o.id === selectedOrder);
      if (selected) {
        side.append(
          n("h3", tr(res(selected.resource).name)),
          n("p", selected.seller_name),
          n("p", selected.star + " · " + selected.station),
        );
        const q = field(side, "Количество", "order-buy-quantity", 1, "number");
        q.min = 1;
        q.max = selected.quantity;
        const total = n("p");
        q.oninput = () =>
          (total.textContent =
            tr("Итого") + ": " + money(Number(q.value) * selected.price));
        q.oninput();
        side.append(
          total,
          b(
            "Купить на склад",
            () =>
              command({
                action: "order_buy",
                id: selected.id,
                quantity: Number(q.value),
              }),
            selected.seller === gameData.state.user.id,
          ),
        );
        if (selected.seller === gameData.state.user.id)
          side.append(
            b("Снять предложение", () =>
              command({ action: "order_cancel", id: selected.id }),
            ),
          );
      } else UI.empty(side, "Выберите предложение");
      side.append(n("h3", tr("Выставить со склада")));
      const resource = choice(
          side,
          "Ресурс",
          d().resources.map((r) => [r.id, r.name]),
          "iron",
        ),
        quantity = field(
          side,
          "Количество",
          "order-sell-quantity",
          1,
          "number",
        ),
        price = field(
          side,
          "Цена за единицу",
          "order-sell-price",
          10,
          "number",
        );
      side.append(
        b(
          "Выставить на продажу",
          () =>
            command({
              action: "order_sell",
              resource: resource.value,
              quantity: Number(quantity.value),
              price: Number(price.value),
            }),
          !d().docked,
          "Стыкуйтесь со станцией",
        ),
      );
      root.append(layout);
    }
    if (marketTab === "data") {
      const list = n("section");
      for (const offer of w().data_offers || []) {
        const card = n("article", undefined, "data-offer");
        card.append(
          n(
            "h3",
            tr(
              window.MapData?.regions.find((r) => r.id === offer.region)
                ?.name || offer.region,
            ),
          ),
          n(
            "p",
            offer.seller_name + " · " + offer.star + " / " + offer.station,
          ),
        );
        card.append(
          n("small", fmtTime(offer.created_at)),
          n(
            "p",
            tr(
              "Пакет содержит сведения об объектах. Рыночные цены и квоты обновляются отдельно.",
            ),
            "muted",
          ),
        );
        const details = n("details");
        details.append(
          n(
            "summary",
            num(offer.systems.length) +
              " " +
              tr("Системы") +
              " · " +
              num(offer.new_systems) +
              " " +
              tr("Новые сведения"),
          ),
          n(
            "p",
            offer.systems
              .map(
                (id) =>
                  window.MapData?.stars.find((s) => s.id === id)?.name || id,
              )
              .join(" · "),
          ),
        );
        card.append(
          details,
          n("p", money(offer.price)),
          b(
            offer.seller === gameData.state.user.id
              ? "Снять предложение"
              : "Купить разведданные",
            () =>
              command({
                action:
                  offer.seller === gameData.state.user.id
                    ? "data_remove"
                    : "data_buy",
                id: offer.id,
              }),
            offer.seller !== gameData.state.user.id &&
              (!offer.new_systems ||
                !d().docked ||
                d().ship.star !== offer.star ||
                d().ship.object_id !== offer.station),
            offer.new_systems
              ? "Прибудьте на станцию продажи данных"
              : "Все сведения уже известны",
          ),
        );
        list.append(card);
      }
      if (!list.children.length)
        UI.empty(list, "Игроки ещё не выставили разведданные");
      const side = n("aside", undefined, "detail-panel");
      side.append(
        n("h3", tr("Продать свои исследования")),
        n(
          "p",
          tr(
            "В пакет попадут только исследованные вами системы выбранного региона.",
          ),
          "muted",
        ),
      );
      const region = choice(
          side,
          "Регион",
          (window.MapData?.regions || []).map((r) => [r.id, r.name]),
          d().system.region,
        ),
        price = field(side, "Цена пакета", "data-price", 100, "number");
      side.append(
        b(
          "Выставить пакет",
          () =>
            command({
              action: "data_sell",
              region: Number(region.value),
              price: Number(price.value),
            }),
          !d().docked,
          "Стыкуйтесь со станцией",
        ),
      );
      const layout = n("div", undefined, "master-detail");
      layout.append(list, side);
      root.append(layout);
    }
    if (marketTab === "delivery") {
      renderDeliveries(root);
    }
  }
  function renderDeliveries(root) {
    const list = n("section");
    list.append(
      UI.table(
        ["ID", "Маршрут", "Груз", "Награда", "Статус", "Действие"],
        (w().deliveries || []).map((job) => {
          const actions = n("div", undefined, "inline-controls"),
            own = job.owner === gameData.state.user.id;
          actions.append(b("Информация", () => deliveryDetails(job)));
          if (job.status === "open") {
            actions.append(
              b(own ? "Нанять бота" : "Принять", () =>
                command({
                  action: own ? "delivery_bot" : "delivery_accept",
                  id: job.id,
                }),
              ),
            );
            if (own)
              actions.append(
                b("Отменить", () =>
                  command({ action: "delivery_cancel", id: job.id }),
                ),
              );
          }
          if (
            job.status === "carrying" &&
            job.carrier === gameData.state.user.id
          )
            actions.append(
              b("Сдать груз", () =>
                command({
                  action: "delivery_finish",
                  id: job.id,
                  ship: job.ship_id,
                }),
              ),
            );
          return [
            "#" + job.id,
            job.from_star + " → " + job.to_star,
            tr(res(job.resource).name) + " × " + job.quantity,
            money(job.reward),
            tr(job.status),
            actions,
          ];
        }),
      ),
    );
    root.append(list);
    const form = n("details");
    form.append(n("summary", tr("Создать доставку")));
    const fields = n("div", undefined, "settings-fields"),
      resource = choice(
        fields,
        "Ресурс",
        d().resources.map((r) => [r.id, r.name]),
        "iron",
      ),
      quantity = field(fields, "Количество", "delivery-quantity", 1, "number"),
      target = choice(
        fields,
        "Станция назначения",
        (w().stations || []).map((s) => [s.star + "|" + s.object_id, s.name]),
        "5|station",
      ),
      reward = field(
        fields,
        "Вознаграждение",
        "delivery-reward",
        100,
        "number",
      ),
      collateral = field(
        fields,
        "Обеспечение",
        "delivery-collateral",
        100,
        "number",
      ),
      hours = field(fields, "Срок, часов", "delivery-hours", 24, "number");
    form.append(
      fields,
      b(
        "Создать заказ",
        () => {
          const [to, station] = target.value.split("|");
          return command({
            action: "delivery_create",
            resource: resource.value,
            quantity: Number(quantity.value),
            to: Number(to),
            station,
            amount: Number(reward.value),
            collateral: Number(collateral.value),
            hours: Number(hours.value),
          });
        },
        !d().docked,
        "Стыкуйтесь со станцией",
      ),
    );
    root.append(form);
  }
  function deliveryDetails(job) {
    const root = Modules.create("delivery-info", "Доставка", 520, 430);
    reset(root);
    row(root, "Отправление", job.from_star + " · " + job.from_station);
    row(root, "Назначение", job.to_star + " · " + job.to_station);
    row(root, "Груз", tr(res(job.resource).name) + " × " + job.quantity);
    row(root, "Вознаграждение", money(job.reward));
    row(root, "Обеспечение", money(job.collateral));
    row(root, "Срок", fmtTime(job.deadline));
    row(root, "Исполнитель", job.carrier_name || tr("Не назначен"));
    if (job.arrival) row(root, "Прибытие", fmtTime(job.arrival));
    Modules.open("delivery-info");
  }
  function can(permission) {
    if (w().company_owner === gameData.state.user.id) return true;
    const member = w().members?.find((m) => m.id === gameData.state.user.id),
      role = w().roles?.find((r) => r.id === member?.company_role);
    return role?.permissions.includes(permission) || false;
  }
  let companyList = [],
    createCorp = false;
  function flagPicker(root, initial, change) {
    const host = n("div", undefined, "flag-picker"),
      preview = b(initial, () => {
        const old = host.querySelector(".flag-grid");
        if (old) {
          old.remove();
          return;
        }
        const grid = n("div", undefined, "flag-grid");
        for (const flag of [..."◈◆★☀☾⚑⚙✦"]) {
          const tile = b(flag, () => {
            preview.textContent = flag;
            preview.value = flag;
            grid.remove();
            change?.(flag);
          });
          tile.classList.toggle("active", flag === preview.value);
          grid.append(tile);
        }
        host.append(grid);
      });
    preview.value = initial;
    preview.className = "flag-preview";
    host.append(preview);
    root.append(host);
    return preview;
  }
  async function renderCompanies() {
    const key = ++requestID,
      root = roots.corporations;
    const result = await api("companies?scope=" + corpScope);
    if (key !== requestID || !signedIn) return;
    companyList = result;
    reset(root);
    const tools = n("div", undefined, "inline-controls");
    choice(
      tools,
      "Показывать",
      [
        ["all", "Все корпорации"],
        ["region", "Корпорации региона"],
        ["system", "Корпорации системы"],
      ],
      corpScope,
      (value) => {
        corpScope = value;
        renderCompanies();
      },
    );
    tools.append(
      b(
        "Создать корпорацию",
        () => {
          createCorp = !createCorp;
          renderCompanies();
        },
        !!gameData.profile.company_id,
        "Сначала выйдите из текущей корпорации",
      ),
    );
    root.append(tools);
    const layout = n("div", undefined, "master-detail"),
      list = n("section");
    for (const c of companyList) {
      const button = b("", () => {
        selectedCorp = c.id;
        createCorp = false;
        renderCompanies();
      });
      button.className = "company-list-row";
      const badge = n("span", c.flag + " " + c.tag, "corp-badge");
      UI.paintFlag(badge, c);
      button.append(
        badge,
        n("span", c.starter || c.founder ? tr(c.name) : c.name),
        n("small", c.founder ? tr("Корпорация-основатель") : num(c.members)),
      );
      button.classList.toggle(
        "selected",
        (selectedCorp || gameData.profile.company_id) === c.id,
      );
      list.append(button);
    }
    layout.append(list);
    const side = n("aside", undefined, "detail-panel");
    layout.append(side);
    root.append(layout);
    if (createCorp) {
      renderCorpCreate(side);
      return;
    }
    const c =
      companyList.find(
        (c) => c.id === (selectedCorp || gameData.profile.company_id),
      ) || companyList[0];
    if (!c) return;
    side.append(n("h3", c.starter || c.founder ? tr(c.name) : c.name));
    const my = c.id === gameData.profile.company_id;
    if (!my) {
      row(side, "Сокращение", c.tag);
      row(side, "Участников", c.members);
      if (c.founder) {
        const rep = w().reputation?.find((r) => r.company_id === c.id);
        row(side, "Репутация", rep?.points || 0);
        side.append(
          n("p", tr("Тарифы и услуги доступны в карточках портов."), "muted"),
        );
      } else
        side.append(
          b(
            c.starter ? "Присоединиться" : "Подать заявку",
            () =>
              c.starter
                ? act("companies", { action: "join", id: c.id })
                : command({ action: "apply", id: c.id }),
            !!gameData.profile.company_id,
            "Сначала выйдите из текущей корпорации",
          ),
        );
      renderInvitations(side);
      return;
    }
    UI.tabs(
      side,
      [
        ["overview", "Обзор"],
        ["members", "Участники"],
        ["finance", "Финансы"],
        ["storage", "Склады"],
        ["projects", "Проекты"],
        ["manage", "Управление"],
      ],
      corpTab,
      (id) => {
        corpTab = id;
        renderCompanies();
      },
    );
    if (corpTab === "overview") {
      row(side, "Участников", c.members);
      row(
        side,
        "Владелец",
        w().members?.find((m) => m.id === c.owner)?.username || "—",
      );
      if (can("finance"))
        row(side, "Корпоративный кошелёк", money(w().company_balance));
      side.append(
        b(
          "Выйти из корпорации",
          () =>
            UI.confirm("Выход из корпорации", c.name, () =>
              act("companies", { action: "leave" }),
            ),
          c.owner === gameData.state.user.id,
          "Сначала передайте владение",
        ),
      );
      renderInvitations(side);
    }
    if (corpTab === "members") {
      side.append(
        UI.table(
          ["Игрок", "Роль", "Действие"],
          (w().members || []).map((m) => {
            const role =
              can("owner") && m.id !== c.owner
                ? UI.picker(
                    [
                      [0, "Участник"],
                      ...(w().roles || []).map((r) => [r.id, r.name]),
                    ],
                    m.company_role || 0,
                    (value) =>
                      command({
                        action: "role_assign",
                        id: m.id,
                        amount: Number(value),
                      }),
                  )
                : n("span", tr(m.role_name));
            return [
              m.username,
              role,
              can("owner") && m.id !== c.owner
                ? b("Передать владение", () =>
                    UI.confirm("Передача владения", m.username, () =>
                      command({ action: "company_transfer", id: m.id }),
                    ),
                  )
                : "—",
            ];
          }),
        ),
      );
      if (can("manage")) {
        const name = field(side, "Игрок: имя или ID", "invite-name", "");
        side.append(
          b("Пригласить", () =>
            command({ action: "invite", name: name.value }),
          ),
        );
        for (const req of (w().requests || []).filter(
          (r) => r.company_id === c.id && r.kind === "application",
        )) {
          const line = n("div", undefined, "inline-controls");
          line.append(
            n("span", req.username),
            b("Принять", () =>
              command({ action: "request_accept", id: req.id }),
            ),
            b("Отклонить", () =>
              command({ action: "request_reject", id: req.id }),
            ),
          );
          side.append(line);
        }
      }
    }
    if (corpTab === "finance") {
      const amount = field(side, "Сумма, CR", "company-amount", 100, "number");
      side.append(
        b("Пополнить", () =>
          command({ action: "company_deposit", amount: Number(amount.value) }),
        ),
      );
      if (can("finance")) {
        side.append(
          b("Снять", () =>
            command({
              action: "company_withdraw",
              amount: Number(amount.value),
            }),
          ),
        );
        row(side, "Баланс", money(w().company_balance));
        side.append(
          UI.table(
            ["Дата", "Участник", "Изменение"],
            (w().company_finance || []).map((f) => [
              fmtTime(f.created_at),
              f.username,
              money(f.amount),
            ]),
          ),
        );
      } else UI.empty(side, "Для просмотра финансов нужны полномочия");
    }
    if (corpTab === "storage") {
      for (const store of w().company_storage || []) {
        side.append(n("h3", store.star + " · " + store.station));
        side.append(
          UI.table(
            ["Ресурс", "Количество", "Действие"],
            Object.entries(store.cargo)
              .filter(([, q]) => q > 0)
              .map(([id, q]) => [
                resourceName(id),
                q,
                b(
                  "В трюм",
                  () =>
                    actionAmount(
                      "В трюм",
                      "Количество",
                      Math.min(60, q),
                      (quantity) =>
                        command({
                          action: "company_cargo_withdraw",
                          resource: id,
                          quantity,
                        }),
                    ),
                  !can("warehouse") ||
                    !d().docked ||
                    d().ship.star !== store.star ||
                    d().ship.object_id !== store.station,
                  "Прибудьте на станцию хранения",
                ),
              ]),
          ),
        );
      }
      if (!w().company_storage?.length)
        UI.empty(side, "На складах пока нет ресурсов");
    }
    if (corpTab === "projects") renderProjects(side);
    if (corpTab === "manage") {
      if (!can("owner")) {
        UI.empty(side, "Управление ролями доступно владельцу");
        return;
      }
      side.append(
        n("h3", tr("Роли") + " · " + (w().roles?.length || 0) + " / 7"),
      );
      const permissionNames = [
        ["finance", "Финансы"],
        ["manage", "Управление участниками"],
        ["projects", "Создание проектов"],
        ["warehouse", "Выдача со склада"],
      ];
      side.append(
        UI.table(
          ["Роль", ...permissionNames.map(([, name]) => name), "Действие"],
          (w().roles || []).map((role) => {
            const values = new Set(role.permissions),
              name = n("input");
            name.value = role.name;
            const boxes = permissionNames.map(([key]) => {
              const box = n("input");
              box.type = "checkbox";
              box.checked = values.has(key);
              box.onchange = () =>
                box.checked ? values.add(key) : values.delete(key);
              return box;
            });
            const controls = n("div", undefined, "inline-controls");
            controls.append(
              b("Сохранить", () =>
                command({
                  action: "role_save",
                  id: role.id,
                  name: name.value,
                  permissions: [...values],
                }),
              ),
              b("Удалить", () =>
                command({ action: "role_delete", id: role.id }),
              ),
            );
            return [name, ...boxes, controls];
          }),
        ),
      );
      const name = field(side, "Название новой роли", "role-name", "");
      side.append(
        b(
          "Добавить роль",
          () =>
            command({ action: "role_save", name: name.value, permissions: [] }),
          w().roles.length >= 7,
          "Максимум 7 настраиваемых ролей",
        ),
        n(
          "p",
          tr("Роли владельца и первоначального участника не входят в лимит."),
          "muted",
        ),
      );
      renderFlagDesign(side, c);
      side.append(
        b("Распустить корпорацию", () =>
          UI.confirm(
            "Роспуск корпорации",
            tr(
              "Остаток средств получит владелец. Активные проекты и склад должны быть закрыты. Построенные порты перейдут корпорации-основателю 01.",
            ),
            () => command({ action: "company_dissolve" }),
          ),
        ),
      );
    }
  }
  function renderInvitations(root) {
    for (const req of (w().requests || []).filter(
      (r) => r.user_id === gameData.state.user.id,
    )) {
      const line = n("div", undefined, "inline-controls");
      line.append(n("span", tr(req.kind) + " #" + req.company_id));
      if (req.kind === "invitation")
        line.append(
          b(
            "Принять приглашение",
            () => command({ action: "invitation_accept", id: req.id }),
            !!gameData.profile.company_id,
            "Сначала выйдите из текущей корпорации",
          ),
        );
      line.append(
        b("Отменить", () => command({ action: "request_cancel", id: req.id })),
      );
      root.append(line);
    }
  }
  function renderCorpCreate(root) {
    root.append(n("h3", tr("Создать корпорацию")));
    const form = n("form"),
      head = n("div", undefined, "corp-create-header"),
      preview = flagPicker(head, "◈"),
      fields = n("div");
    head.append(fields);
    const name = field(fields, "Полное название", "company-full-name", ""),
      tag = field(fields, "Сокращение · 3–4 символа", "company-tag", ""),
      color = field(fields, "Цвет", "company-color", "#efad58", "color");
    name.maxLength = 60;
    tag.maxLength = 4;
    const stripe = n("div", undefined, "color-preview");
    color.oninput = () => {
      stripe.style.background = color.value;
      preview.style.color = color.value;
    };
    color.oninput();
    form.append(head, stripe);
    const cost = gameData.clock.settings.company_cost;
    form.append(
      n("p", money(cost) + " · " + tr("Баланс") + ": " + money(d().credits)),
    );
    const submit = b(
      "Создать корпорацию",
      async () => {
        const ok = await act("companies", {
          action: "create",
          name: name.value,
          tag: tag.value,
          color: color.value,
          flag: preview.value,
        });
        if (ok) {
          createCorp = false;
          selectedCorp = gameData.profile.company_id;
          await renderCompanies();
        }
      },
      d().credits < cost,
      "Недостаточно кредитов",
    );
    submit.classList.add("primary-action");
    form.append(submit);
    form.onsubmit = (e) => {
      e.preventDefault();
      submit.click();
    };
    root.append(form);
  }
  function renderFlagDesign(root, c) {
    const form = n("section");
    form.append(n("h3", tr("Конструктор флага")));
    const preview = flagPicker(form, c.flag),
      color = field(form, "Цвет корпорации", "flag-color", c.color, "color"),
      bg = field(
        form,
        "Фон флага",
        "flag-background",
        c.flag_design?.background || "#17232d",
        "color",
      ),
      stripe = field(
        form,
        "Полоса",
        "flag-stripe",
        c.flag_design?.stripe || c.color,
        "color",
      ),
      update = () => {
        preview.style.color = color.value;
        preview.style.background = `linear-gradient(135deg,${bg.value} 38%,${stripe.value} 39%,${stripe.value} 53%,${bg.value} 54%)`;
      };
    for (const input of [color, bg, stripe]) input.oninput = update;
    update();
    form.append(
      b("Сохранить флаг", () =>
        command({
          action: "flag_save",
          flag: preview.value,
          color: color.value,
          flag_design: {
            background: bg.value,
            stripe: stripe.value,
            symbol: color.value,
          },
        }),
      ),
    );
    root.append(form);
  }
  function renderProjects(root) {
    for (const p of w().projects || []) {
      const card = n("article", undefined, "project-card");
      card.append(
        n("h3", p.name),
        n("p", tr(p.kind) + " · " + p.star + " · " + tr(p.status)),
      );
      for (const [id, need] of Object.entries(p.required)) {
        const done = p.contributed[id] || 0;
        card.append(resourceName(id), UI.bar(done, need, "Вклад"));
        if (p.status === "active")
          card.append(
            b(
              "Внести со склада",
              () =>
                actionAmount(
                  "Вклад в проект",
                  "Количество",
                  need - done,
                  (quantity) =>
                    command({
                      action: "project_contribute",
                      id: p.id,
                      resource: id,
                      quantity,
                    }),
                ),
              done >= need,
            ),
          );
      }
      const contributions = n("details");
      contributions.append(n("summary", tr("Вклады участников")));
      for (const item of (w().project_contributions || []).filter(
        (x) => x.project_id === p.id,
      ))
        contributions.append(
          n(
            "p",
            (item.username || "#" + item.user_id) +
              " · " +
              tr(res(item.resource).name) +
              " × " +
              item.quantity,
          ),
        );
      card.append(contributions);
      root.append(card);
    }
    if (can("projects")) {
      const name = field(root, "Название проекта", "project-name", ""),
        type = choice(
          root,
          "Тип проекта",
          [
            ["warehouse", "Корпоративный склад"],
            ["port", "Порт"],
            ["station", "Станция"],
          ],
          "warehouse",
        );
      root.append(
        b(
          "Создать проект",
          () =>
            command({
              action: "project_create",
              name: name.value,
              target: type.value,
            }),
          !d().docked,
          "Стыкуйтесь со станцией",
        ),
      );
    }
  }
  function renderProfile() {
    const root = roots.profile;
    reset(root);
    row(root, "Позывной", gameData.state.user.username);
    row(
      root,
      "Роль",
      tr(gameData.state.user.role === "admin" ? "АДМИНИСТРАТОР" : "ИГРОК"),
    );
    row(root, "Корпорация", gameData.profile.company || tr("Без компании"));
    row(root, "Кораблей", gameData.fleet.ships.length);
    root.append(
      b("История финансов", () => Modules.open("finance")),
      b("Настройки", () => Modules.open("settings")),
    );
  }
  async function renderUsers() {
    const result = await api(
        "admin/users?page=" + userPage + "&q=" + encodeURIComponent(userQuery),
      ),
      root = roots.users,
      focused = document.activeElement?.id === "user-search",
      caret = document.activeElement?.selectionStart;
    reset(root);
    userPage = result.page;
    const search = field(
      root,
      "Поиск по никнейму или ID",
      "user-search",
      userQuery,
      "search",
    );
    let timer;
    search.oninput = () => {
      userQuery = search.value;
      clearTimeout(timer);
      timer = setTimeout(() => {
        userPage = 1;
        renderUsers();
      }, 250);
    };
    search.onkeydown = (e) => {
      if (e.key === "Enter") {
        clearTimeout(timer);
        userQuery = search.value;
        userPage = 1;
        renderUsers();
      }
    };
    const paging = n("div", undefined, "pagination sticky-toolbar");
    paging.append(
      b(
        "‹",
        () => {
          userPage--;
          renderUsers();
        },
        userPage <= 1,
      ),
      n("span", `${userPage} / ${result.pages} · ${num(result.total)}`),
      b(
        "›",
        () => {
          userPage++;
          renderUsers();
        },
        userPage >= result.pages,
      ),
    );
    root.append(
      paging,
      UI.table(
        ["ID", "Игрок", "Роль", "Баланс", "Уровень игрока", "Действие"],
        result.items.map((u) => [
          u.id,
          u.username,
          tr(u.role) + (u.bot ? " · BOT" : ""),
          money(u.credits),
          u.level,
          b("Редактировать", () => editUser(u)),
        ]),
      ),
    );
    if (focused) {
      search.focus();
      search.setSelectionRange(caret, caret);
    }
  }
  function editUser(u) {
    const id = "user-" + u.id,
      root = Modules.create(id, "Редактирование пользователя", 550, 640);
    reset(root);
    const form = n("form");
    form.append(n("h3", tr("Основные сведения")));
    const name = field(form, "Позывной", "edit-name-" + u.id, u.username);
    name.disabled = u.bot;
    form.append(n("h3", tr("Игровые показатели")));
    const values = n("div", undefined, "settings-fields"),
      credits = field(
        values,
        "Кредиты",
        "edit-credits-" + u.id,
        u.credits,
        "number",
      ),
      level = field(
        values,
        "Уровень игрока",
        "edit-level-" + u.id,
        u.level,
        "number",
      ),
      xp = field(values, "Опыт", "edit-xp-" + u.id, u.xp, "number");
    form.append(values, n("h3", tr("Доступ")));
    const role = choice(
      form,
      "Роль",
      [
        ["player", "ИГРОК"],
        ["admin", "АДМИНИСТРАТОР"],
      ],
      u.role,
    );
    role.disabled = u.bot;
    const pass = field(
      form,
      "Новый пароль · пусто — без изменений",
      "edit-password-" + u.id,
      "",
      "password",
    );
    pass.disabled = u.bot;
    pass.autocomplete = "new-password";
    const dirty = UI.dirty(
      form,
      async () => {
        const ok = await act("admin/user", {
          id: u.id,
          username: name.value,
          credits: Number(credits.value),
          level: Number(level.value),
          xp: Number(xp.value),
          role: role.value,
          password: pass.value,
        });
        if (ok) {
          Object.assign(u, {
            username: name.value,
            credits: Number(credits.value),
            level: Number(level.value),
            xp: Number(xp.value),
            role: role.value,
          });
          pass.value = "";
          renderUsers();
          editUser(u);
        }
        return ok;
      },
      () => editUser(u),
    );
    role.onchange = dirty;
    form.addEventListener("input", (e) =>
      e.target.classList.add("field-dirty"),
    );
    root.append(form);
    Modules.open(id);
  }
  async function renderGameplay() {
    const result = await api("admin/gameplay"),
      root = roots.gameplay;
    reset(root);
    const form = n("form"),
      fields = n("div", undefined, "settings-fields"),
      g = result.settings,
      inputs = {};
    const specs = [
      [
        "tick_seconds",
        "Длительность цикла, секунд",
        "Реальные секунды между фазами. Влияет на добычу и производство.",
      ],
      [
        "date_step",
        "Импульсов даты за цикл",
        "Скорость звёздной даты. Не меняет реальную длительность перелётов.",
      ],
      [
        "quota_common",
        "Общий лимит объекта",
        "Общий объём добычи на объекте для всех игроков и ресурсов.",
      ],
      [
        "reset_seconds",
        "Период восстановления, часов",
        "Реальные часы восстановления. Не зависит от звёздной даты.",
      ],
      [
        "rare_min",
        "Минимум редкого ресурса",
        "Редкая квота входит в общий лимит объекта.",
      ],
      [
        "rare_max",
        "Максимум редкого ресурса",
        "Редкая квота входит в общий лимит объекта.",
      ],
      [
        "scan_seconds",
        "Сканирование объекта, секунд",
        "Время сканирования одного объекта.",
      ],
      [
        "company_cost",
        "Создание корпорации, CR",
        "Стоимость регистрации новой корпорации.",
      ],
      [
        "rescue_cost",
        "Эвакуация, CR",
        "Стоимость буксировки к ближайшему доступному порту.",
      ],
      [
        "reservation_hours",
        "Хранение брони, часов",
        "Полный возврат при отмене или истечении срока.",
      ],
    ];
    for (const [key, label, help] of specs) {
      const i = field(
        fields,
        label,
        "setting-" + key,
        key === "reset_seconds" ? g[key] / 3600 : g[key],
        "number",
        help,
      );
      i.required = true;
      if (key === "reset_seconds") i.step = "any";
      inputs[key] = i;
    }
    form.append(
      fields,
      n(
        "p",
        tr(
          "Новые лимиты и период применяются после завершения текущего периода. По умолчанию восстановление — 5 часов.",
        ),
        "muted",
      ),
    );
    const values = () =>
      Object.fromEntries(
        Object.entries(inputs).map(([key, i]) => [
          key,
          key === "reset_seconds"
            ? Math.round(Number(i.value) * 3600)
            : Number(i.value),
        ]),
      );
    UI.dirty(
      form,
      () => act("admin/gameplay", values()),
      () => renderGameplay(),
    );
    root.append(
      form,
      b("Восстановить квоты сейчас", () =>
        UI.confirm(
          "Восстановление квот",
          tr(
            "Обнулить использованную квоту всех объектов? Незавершённые задания сохранятся.",
          ),
          () => act("admin/gameplay", { ...values(), reset_now: true }),
        ),
      ),
    );
    const log = await api("admin/log");
    const details = n("details");
    details.append(n("summary", tr("Журнал администратора")));
    for (const event of log.slice(0, 20))
      details.append(
        n(
          "p",
          fmtTime(event.at) +
            " · " +
            tr(event.kind) +
            " · " +
            (event.data.actor_name || event.data.actor),
        ),
      );
    root.append(details);
  }
  const eventNames = {
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
    return (
      item.message ||
      tr(eventNames[item.kind] || item.kind) +
        (data.star
          ? " · " +
            (window.MapData?.stars.find((s) => s.id === data.star)?.name ||
              "#" + data.star)
          : "") +
        (data.resource
          ? " · " + tr(res(data.resource)?.name || data.resource)
          : "") +
        (data.quantity ? " × " + data.quantity : "")
    );
  }
  async function loadNotifications() {
    if (!signedIn) return;
    const epoch = authEpoch,
      items = await api("notifications");
    if (epoch !== authEpoch) return;
    gameData.notifications = items;
    notificationBadge();
    if (Modules.isOpen("notifications")) renderNotifications();
  }
  function notificationBadge() {
    const info = gameData.notifications || { count: 0, items: [] },
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
    const items = gameData.notifications?.items || [];
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
        const details = n("details");
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
        const detail = n("details");
        detail.append(
          n("summary", eventText(item)),
          n("p", fmtTime(item.created_at), "muted"),
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
  const objectArticles = [
    [
      "star",
      "Звезда",
      "Звезда",
      "Масса и светимость звезды определяют условия в системе. Спектральный класс описывает температуру её поверхности.",
    ],
    [
      "garden",
      "Садовый мир",
      "Садовый мир",
      "Мир с умеренной температурой и потенциальными источниками биосырья. Условия конкретной планеты определяются разведкой.",
    ],
    [
      "rocky",
      "Каменистая",
      "Каменистая",
      "Твёрдый мир с силикатной корой. Состав месторождений различается между планетами.",
    ],
    [
      "icy",
      "Ледяная",
      "Ледяная",
      "Холодный мир с запасами льда и замёрзших летучих веществ.",
    ],
    [
      "gas",
      "Газовый гигант",
      "Газовый гигант",
      "Крупный мир с протяжённой газовой оболочкой; источник летучих компонентов.",
    ],
    [
      "volcanic",
      "Вулканическая",
      "Вулканическая",
      "Горячая планета с активной поверхностью и минеральными залежами.",
    ],
    [
      "belt",
      "Пояс астероидов",
      "Пояс астероидов",
      "Область малых каменистых тел. Общая квота добычи разделяется всеми игроками.",
    ],
    [
      "ice",
      "Ледяное поле",
      "Ледяное поле",
      "Скопление ледяных тел, используемых для добычи воды и летучих веществ.",
    ],
    [
      "station",
      "Орбитальная станция",
      "Орбитальная станция",
      "Обслуживает корабли и торговлю. Ассортимент, услуги и принадлежность различаются.",
    ],
    [
      "black-hole",
      "Чёрная дыра",
      "Чёрная дыра",
      "Центральный объект Эдема и узел проекта межгалактического перехода.",
    ],
  ];
  function renderGuide() {
    const root = roots.guide;
    reset(root);
    const layout = n("div", undefined, "guide-layout"),
      left = n("aside"),
      article = n("article", undefined, "guide-article"),
      search = field(left, "Поиск", "guide-search", guideQuery, "search");
    const all = [
      ...d().resources.map((r) => ({
        id: r.id,
        name: r.name,
        code: r.code,
        desc: r.description,
        resource: r,
        category: "Ресурсы",
        subcategory: r.kind,
      })),
      ...objectArticles.map(([id, name, kind, desc]) => ({
        id,
        name,
        kind,
        desc,
        category: "Космические объекты",
        subcategory:
          kind === "Орбитальная станция"
            ? "Станции"
            : kind === "Звезда"
              ? "Звёзды"
              : kind === "Чёрная дыра"
                ? "Особые объекты"
                : kind === "Пояс астероидов" || kind === "Ледяное поле"
                  ? "Месторождения"
                  : "Планеты",
      })),
    ];
    const drawList = () => {
      left.querySelector(".guide-list")?.remove();
      const list = n("div", undefined, "guide-list"),
        q = search.value.toLowerCase();
      const matched = all.filter((item) =>
        [
          tr(item.name),
          item.code || "",
          tr(item.desc),
          tr(item.category),
          tr(item.subcategory),
        ].some((value) => value.toLowerCase().includes(q)),
      );
      for (const category of ["Ресурсы", "Космические объекты"]) {
        const entries = matched.filter((item) => item.category === category);
        if (!entries.length) continue;
        const group = n("details", undefined, "guide-category");
        group.open = true;
        group.append(n("summary", tr(category) + " · " + entries.length));
        for (const subcategory of new Set(
          entries.map((item) => item.subcategory),
        )) {
          const section = n("section");
          section.append(n("h4", tr(subcategory)));
          for (const item of entries.filter(
            (item) => item.subcategory === subcategory,
          )) {
            const button = b("", () => {
                guideID = item.id;
                renderArticle();
                drawList();
              }),
              label = tr(item.name),
              at = q ? label.toLowerCase().indexOf(q) : -1;
            if (at >= 0) {
              button.append(
                document.createTextNode(label.slice(0, at)),
                n("mark", label.slice(at, at + q.length)),
                document.createTextNode(label.slice(at + q.length)),
              );
            } else button.textContent = label;
            button.classList.toggle("selected", guideID === item.id);
            if (item.code) button.append(n("small", item.code));
            section.append(button);
          }
          group.append(section);
        }
        list.append(group);
      }
      if (!matched.length) UI.empty(list, "Ничего не найдено");
      left.append(list);
    };
    const renderArticle = () => {
      article.replaceChildren();
      const item = all.find((v) => v.id === guideID) || all[0];
      article.append(n("h2", tr(item.name)));
      if (item.resource) {
        article.append(
          n("div", item.code, "guide-code"),
          n("p", tr(item.desc)),
        );
        row(article, "Источники", tr(item.resource.sources));
        row(article, "Редкость", tr(item.resource.rarity));
        row(article, "Применение", item.resource.uses.map(tr).join(" · "));
        article.append(n("h3", tr("Связанные материалы")));
        for (const r of d()
          .resources.filter(
            (r) =>
              r.id !== item.id &&
              r.uses.some((tag) => item.resource.uses.includes(tag)),
          )
          .slice(0, 5))
          article.append(
            b(r.name, () => {
              guideID = r.id;
              renderArticle();
              drawList();
            }),
          );
      } else {
        const canvas = n("canvas");
        canvas.width = 650;
        canvas.height = 300;
        canvas.className = "object-model";
        Visuals.object(canvas.getContext("2d"), 325, 150, 95, {
          kind: item.kind,
        });
        article.append(
          canvas,
          n("p", tr(item.desc)),
          n(
            "p",
            tr(
              "Параметры и месторождения конкретных объектов открываются после исследования.",
            ),
            "muted",
          ),
        );
      }
    };
    search.oninput = () => {
      guideQuery = search.value;
      drawList();
    };
    layout.append(left, article);
    root.append(layout);
    drawList();
    renderArticle();
  }
  function renderTasks() {
    const root = roots.tasks;
    reset(root);
    for (const s of gameData.fleet.ships) {
      const card = n("section", undefined, "task-card");
      card.append(n("h3", tr(s.name) + " #" + s.id));
      if (s.destination)
        card.append(
          n("p", `${s.star} → ${s.destination} · ${fmtTime(s.arrival)}`),
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
          n("p", tr("Маршрут") + ": " + s.waypoints.join(" → ")),
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
      if (!fly(s) && !research && !w().mining?.some((j) => j.ship_id === s.id))
        card.append(n("p", tr("Ожидает задания"), "muted"));
      root.append(card);
    }
    for (const delivery of (w().deliveries || []).filter(
      (j) =>
        (j.owner === gameData.state.user.id ||
          j.carrier === gameData.state.user.id) &&
        ["bot", "carrying", "open"].includes(j.status),
    ))
      root.append(
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
  function renderFinance() {
    const root = roots.finance;
    reset(root);
    root.append(
      UI.table(
        ["Дата", "Операция", "Изменение", "Баланс"],
        (w().ledger || []).map((item) => [
          fmtTime(item.created_at),
          tr(item.reason),
          money(item.amount),
          money(item.balance),
        ]),
      ),
    );
    if (!w().ledger?.length) UI.empty(root, "Операций пока нет");
  }
  async function renderEvents() {
    const root = roots.events,
      events = await api("events");
    reset(root);
    root.append(
      UI.table(
        ["Дата", "Событие"],
        events.map((event) => [fmtTime(event.at), eventText(event)]),
      ),
    );
  }
  function renderPortal() {
    const root = roots.portal;
    reset(root);
    const portal = w().portal?.[0];
    if (!portal) return;
    root.append(
      n("h2", tr("Портал Эдема")),
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
    } else
      root.append(
        n("p", tr("Портал активирован")),
        b("Перейти в другую галактику", async () => {
          if (await command({ action: "portal_travel" })) {
            window.MapData = await api("galaxy");
            window.dispatchEvent(new Event("universechange"));
          }
        }),
      );
    root.append(
      n(
        "p",
        tr(
          d().ship.star > 1000
            ? "Обратный переход доступен из NX-0001."
            : "Переход доступен из систем у центральной чёрной дыры.",
        ),
        "muted",
      ),
      n(
        "p",
        (w().core_systems || [])
          .map(
            (id) => window.MapData?.stars.find((s) => s.id === id)?.name || id,
          )
          .join(" · "),
      ),
    );
  }
  async function renderBenchmark() {
    const root = roots.benchmark,
      focused =
        root.contains(document.activeElement) &&
        document.activeElement.matches("input");
    const runs = await api("admin/benchmark");
    if (focused) return;
    reset(root);
    const fields = n("div", undefined, "settings-fields"),
      players = field(
        fields,
        "Виртуальные игроки",
        "benchmark-players",
        10,
        "number",
      ),
      seconds = field(
        fields,
        "Длительность, секунд",
        "benchmark-seconds",
        10,
        "number",
      ),
      scenario = choice(
        fields,
        "Сценарий",
        [
          ["mixed", "Смешанный"],
          ["quota", "Последняя квота"],
          ["stock", "Последний товар"],
          ["reservations", "Бронирование и получение"],
        ],
        "mixed",
      );
    players.min = 1;
    players.max = 500;
    seconds.min = 5;
    seconds.max = 300;
    root.append(
      fields,
      b(
        "Начать тест",
        () =>
          act("admin/benchmark", {
            action: "start",
            players: Number(players.value),
            seconds: Number(seconds.value),
            scenario: scenario.value,
          }),
        runs.some((r) => r.status === "running"),
      ),
      n(
        "p",
        tr(
          "Виртуальные игроки используют HTTP API и PostgreSQL в одноразовой тестовой схеме той же базы.",
        ),
        "muted",
      ),
    );
    for (const run of runs.filter((r) => r.status === "running")) {
      root.append(
        UI.bar(
          Math.min(
            run.seconds,
            (Date.now() - new Date(run.started_at).getTime()) / 1000,
          ),
          run.seconds,
          "Секунды",
        ),
        n(
          "p",
          num(run.players) +
            " · " +
            num(run.requests) +
            " " +
            tr("Запросы") +
            " · " +
            num(run.errors) +
            " " +
            tr("Ошибки"),
        ),
        b("Остановить тест", () =>
          act("admin/benchmark", { action: "stop", id: run.id }),
        ),
      );
    }
    root.append(
      UI.table(
        [
          "Запуск",
          "Игроки",
          "Сценарий",
          "Статус",
          "Запросы",
          "Ошибки",
          "p95, ms",
        ],
        runs.map((run) => [
          b("#" + run.id, () => {
            benchmarkSelected = run.id;
            renderBenchmark();
          }),
          run.players,
          tr(run.scenario),
          tr(run.status),
          num(run.requests),
          num(run.errors),
          Number(run.metrics.p95_ms || 0).toFixed(1),
        ]),
      ),
    );
    const selected =
      runs.find((r) => r.id === benchmarkSelected) ||
      runs.find((r) => r.status !== "running");
    if (selected) {
      root.append(
        n("h3", tr("Результат") + " #" + selected.id),
        n(
          "p",
          selected.players +
            " · " +
            selected.seconds +
            " s · " +
            tr(selected.scenario) +
            " · v" +
            selected.metrics.version +
            " · " +
            fmtTime(selected.started_at),
          "muted",
        ),
      );
      const previous = runs.find(
        (r) =>
          r.id < selected.id &&
          r.scenario === selected.scenario &&
          r.players === selected.players &&
          r.seconds === selected.seconds,
      );
      root.append(
        UI.table(
          ["Показатель", "Текущий запуск", "Предыдущий запуск"],
          [
            "rps",
            "p50_ms",
            "p95_ms",
            "p99_ms",
            "max_ms",
            "integrity_violations",
            "expected_conflicts",
            "pool_max",
            "db_acquire_ms",
            "db_wait_count",
          ].map((key) => [
            tr(key),
            typeof selected.metrics[key] === "number"
              ? Number(selected.metrics[key]).toFixed(2)
              : "—",
            previous?.metrics[key] !== undefined
              ? Number(previous.metrics[key]).toFixed(2)
              : "—",
          ]),
        ),
      );
      for (const key of ["rps", "p95_ms", "errors"]) {
        const row = n("div", undefined, "benchmark-chart-row");
        row.append(n("span", tr(key)));
        const max = Math.max(
          1,
          ...runs.map((r) =>
            key === "errors" ? r.errors : r.metrics[key] || 0,
          ),
        );
        for (const run of runs.slice(0, 8).reverse()) {
          const bar = n("i"),
            value = key === "errors" ? run.errors : run.metrics[key] || 0;
          bar.style.height = Math.max(2, (value / max) * 65) + "px";
          bar.dataset.helpRaw = "#" + run.id + " · " + value;
          row.append(bar);
        }
        root.append(row);
      }
      if (selected.metrics.failure)
        root.append(n("p", selected.metrics.failure, "error"));
    }
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
    events: renderEvents,
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
    "events",
    "guide",
    "corporations",
  ]);
  function renderVisible(force = false) {
    if (!signedIn || !d()) return;
    for (const [id, render] of Object.entries(renderers)) {
      if (!Modules.isOpen(id) || (!force && onDemand.has(id))) continue;
      const root = roots[id];
      if (
        root.querySelector('[data-dirty="true"]') ||
        (!force &&
          root.contains(document.activeElement) &&
          document.activeElement.matches("input,select,.picker-button")) ||
        document.querySelector(".picker-popover")
      )
        continue;
      const scroll = root.scrollTop,
        epoch = authEpoch,
        inputs = [...root.querySelectorAll("input[id]")].map((el) => ({
          id: el.id,
          value: el.value,
          checked: el.checked,
        })),
        picks = [...root.querySelectorAll("[data-field].picker-button")].map(
          (el) => ({ key: el.dataset.field, value: el.value }),
        );
      Promise.resolve(render())
        .then(() => {
          if (epoch !== authEpoch) return;
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
              if (el) el.setValue(value.value);
            }
          }
        })
        .catch((error) => notify(error.message, "error"));
    }
  }
  window.addEventListener("moduleopen", async (e) => {
    if (!signedIn) return;
    try {
      await ensureMap();
      await renderers[e.detail]?.();
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
  window.addEventListener("signedout", () => {
    noticeCursor = null;
    notifySelected.clear();
    createCorp = false;
    selectedCorp = null;
    userQuery = "";
    userPage = 1;
    requestID++;
  });
  window.addEventListener("stationmarket", () => {
    marketTab = "local";
  });
  setInterval(() => {
    document.querySelectorAll("[data-reset-at]").forEach((el) => {
      const sec = Math.max(
        0,
        Math.ceil(
          (new Date(el.dataset.resetAt).getTime() -
            Date.now() -
            (gameData.serverOffset || 0)) /
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
                (gameData.serverOffset || 0) -
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
})();
