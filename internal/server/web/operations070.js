"use strict";
(() => {
  const n = UI.node,
    tr = (s) => I18n.t(s),
    button = UI.button;
  const stationKey = (p) => p.star + "|" + (p.station || p.object_id);
  const parseStation = (value) => {
    const [star, station] = String(value).split("|");
    return { star: Number(star), station };
  };
  let request = 0;
  window.Scheduler = {
    async render(root) {
      UI.rememberFolds(root);
      let host = root.querySelector(".planner");
      if (!host || host.dataset.language !== I18n.language) {
        host?.remove();
        host = n("section", undefined, "planner");
        host.dataset.language = I18n.language;
        root.prepend(host);
        host.append(
          n("h3", tr("Планировщик задач")),
          n(
            "p",
            tr(
              "Задачи выполняются сервером при закрытом браузере. На каждом корабле — последовательно. Пауза не отменяет уже начатый полёт или добычу.",
            ),
            "muted",
          ),
        );
        const form = n("form"),
          body = n("div"),
          list = n("div", undefined, "planner-list");
        form.onsubmit = (e) => e.preventDefault();
        let shipID = gameData.expedition.ship.id,
          kind = "mine",
          source,
          dest,
          resource,
          quantity,
          site;
        UI.choice(
          form,
          "Корабль",
          gameData.fleet.ships.map((s) => [s.id, UI.shipName(s)]),
          shipID,
          (value) => {
            shipID = Number(value);
            build();
          },
        );
        UI.choice(
          form,
          "Тип задачи",
          [
            ["mine", "Добыча"],
            ["haul", "Перевозка между складами"],
          ],
          kind,
          (value) => {
            kind = value;
            build();
          },
        );
        const layout = n("div", undefined, "planner-workspace"),
          compose = n("section", undefined, "planner-compose"),
          side = n("aside", undefined, "planner-sidebar"),
          queueTitle = n(
            "h3",
            tr("Очередь и выполнение"),
            "planner-queue-title",
          ),
          history = n("details", undefined, "planner-history"),
          historyList = n("div", undefined, "planner-history-list");
        compose.append(form, body);
        history.append(
          n("summary", tr("История завершённых задач")),
          historyList,
        );
        side.append(queueTitle, list, history);
        layout.append(compose, side);
        host.append(layout);
        let historyRequest = 0;
        async function loadHistory() {
          const serial = ++historyRequest,
            epoch = authEpoch;
          try {
            const tasks = await api("planner?view=history");
            if (
              serial !== historyRequest ||
              epoch !== authEpoch ||
              !host.isConnected ||
              !history.open
            )
              return;
            historyList.replaceChildren();
            for (const task of tasks) {
              const res = gameData.expedition.resources.find(
                (r) => r.id === task.resource,
              );
              const line = n(
                "p",
                task.ship_name +
                  " · " +
                  tr(res?.name || task.resource) +
                  " · " +
                  task.progress +
                  " / " +
                  task.quantity +
                  " · " +
                  tr(task.status),
              );
              historyList.append(line);
            }
            if (!tasks.length)
              UI.empty(historyList, "Завершённых задач пока нет");
          } catch (e) {
            if (epoch === authEpoch && signedIn) notify(e.message, "error");
          }
        }
        history.ontoggle = () => {
          if (history.open) loadHistory();
        };
        let generation = 0;
        async function build() {
          const key = ++generation,
            epoch = authEpoch;
          body.replaceChildren();
          const inputs = n("form");
          inputs.onsubmit = (e) => e.preventDefault();
          body.append(inputs);
          const ship = gameData.fleet.ships.find((s) => s.id === shipID);
          if (!ship) return;
          const ports = gameData.workbench.stations || [];
          if (kind === "mine") {
            const data = await api("system?id=" + ship.star);
            if (key !== generation || epoch !== authEpoch || !host.isConnected)
              return;
            const sites = data.sites.filter((s) => s.resources?.length);
            if (!sites.length) {
              UI.empty(
                inputs,
                "Исследуйте объекты системы для автоматической добычи",
              );
              return;
            }
            site = sites[0];
            source = { star: ship.star, station: site.id };
            const resources = n("div");
            function chooseResources() {
              resources.replaceChildren();
              resource = site.resources[0];
              UI.choice(
                resources,
                "Ресурс",
                site.resources.map((id) => [
                  id,
                  gameData.expedition.resources.find((r) => r.id === id)
                    ?.name || id,
                ]),
                resource,
                (value) => (resource = value),
              );
            }
            UI.choice(
              inputs,
              "Объект добычи",
              sites.map((s) => [s.id, s.name]),
              site.id,
              (value) => {
                site = sites.find((s) => s.id === value);
                source.station = value;
                chooseResources();
              },
            );
            chooseResources();
            inputs.append(resources);
            dest = { star: ship.star, station: "" };
            UI.choice(
              inputs,
              "Выгрузка добытого",
              [
                ["", "Оставлять в трюме"],
                ...ports
                  .filter((p) => p.star === ship.star)
                  .map((p) => ({
                    id: p.object_id,
                    label: p.name,
                    station: { star: p.star, id: p.object_id },
                  })),
              ],
              "",
              (value) => (dest.station = value),
            );
          } else {
            if (!ports.length) {
              UI.empty(inputs, "Нет доступных станций");
              return;
            }
            source = parseStation(
              stationKey(
                ports.find(
                  (p) => p.star === ship.star && p.object_id === ship.object_id,
                ) || ports[0],
              ),
            );
            dest = parseStation(
              stationKey(
                ports.find(
                  (p) => stationKey(p) !== source.star + "|" + source.station,
                ) || ports[0],
              ),
            );
            for (const [label, value, change] of [
              ["Склад отправления", source, (v) => (source = parseStation(v))],
              ["Склад назначения", dest, (v) => (dest = parseStation(v))],
            ]) {
              const field = n("label");
              field.append(n("span", tr(label)));
              UI.stationPicker(
                field,
                ports,
                value.star + "|" + value.station,
                change,
              );
              inputs.append(field);
            }
            resource = gameData.expedition.resources[0].id;
            UI.choice(
              inputs,
              "Ресурс",
              gameData.expedition.resources.map((r) => [r.id, r.name]),
              resource,
              (value) => (resource = value),
            );
          }
          quantity = UI.field(
            inputs,
            "Количество",
            "planner-quantity",
            kind === "mine" ? 30 : 10,
            "number",
          );
          quantity.min = 1;
          quantity.max = 100000;
          quantity.step = 1;
          inputs.append(
            button("Добавить задачу", async () => {
              if (!inputs.reportValidity()) return;
              await api("planner", {
                action: "create",
                ship: shipID,
                kind,
                source_star: source.star,
                source_object: source.station,
                dest_star: dest.star,
                dest_station: dest.station,
                resource,
                quantity: Number(quantity.value),
              });
              notify(tr("Задача добавлена"));
              await refresh(true);
              Scheduler.render(root);
            }),
          );
        }
        build().catch((e) => notify(e.message, "error"));
      }
      const key = ++request,
        epoch = authEpoch,
        result = await api("planner");
      if (key !== request || epoch !== authEpoch || !host.isConnected) return;
      const list = host.querySelector(".planner-list");
      UI.rememberFolds(list);
      list.replaceChildren();
      const active = result
        .filter((t) => ["queued", "running", "paused"].includes(t.status))
        .sort(
          (a, b) =>
            Number(b.carried > 0 || b.pending > 0) -
              Number(a.carried > 0 || a.pending > 0) ||
            b.priority - a.priority ||
            a.id - b.id,
        );
      host.querySelector(".planner-queue-title").textContent =
        tr("Очередь и выполнение") + " · " + active.length;
      host.querySelector(".planner-queue-title").dataset.help =
        "Приоритет меняется в пределах корабля. Текущий цикл добычи или разгрузка уже принятого груза завершается перед переключением.";
      const groups = new Map(),
        counts = new Map();
      for (const task of active)
        counts.set(task.ship_id, (counts.get(task.ship_id) || 0) + 1);
      for (const ship of gameData.fleet.ships) {
        const group = UI.fold(
            `planner:ship:v2:${ship.id}`,
            false,
            "planner-ship",
            true,
          ),
          heading = n("summary", undefined, "planner-ship-heading"),
          tasks = n("div", undefined, "planner-ship-tasks");
        const model = UI.hologram(
          ship.spec.model,
          UI.shipName(ship),
          "planner-ship-model",
        );
        UI.updateHologram(
          model,
          ship.spec.model,
          UI.shipName(ship),
          !!ship.destination || !!ship.local_target,
        );
        const count = counts.get(ship.id) || 0,
          badge = n("span", tr("Задания") + ": " + count, "planner-ship-count");
        badge.dataset.help = "В очереди, выполняются или приостановлены";
        heading.append(model, n("h4", UI.shipName(ship)), badge);
        group.append(heading, tasks);
        UI.shipActivity(tasks, ship, count > 0);
        if (!count) UI.empty(tasks, "Нет запланированных задач");
        groups.set(ship.id, tasks);
        list.append(group);
      }
      for (const task of active) {
        const card = UI.fold(
            `planner:task:${task.id}`,
            true,
            "planner-item",
            true,
          ),
          taskHeading = n("summary"),
          resource = gameData.expedition.resources.find(
            (r) => r.id === task.resource,
          ),
          ship = gameData.fleet.ships.find((s) => s.id === task.ship_id);
        card.dataset.task = String(task.id);
        card.dataset.status = task.status;
        taskHeading.append(
          n(
            "b",
            (ship ? UI.shipName(ship) : task.ship_name) +
              " · " +
              tr(
                {
                  mine: "Добыча",
                  haul: "Перевозка между складами",
                  load: "Получение со склада",
                  unload: "Доставка на склад",
                }[task.kind] || task.kind,
              ),
          ),
        );
        card.append(
          taskHeading,
          n(
            "p",
            tr(resource?.name || task.resource) +
              " · " +
              task.progress +
              " / " +
              task.quantity +
              " · " +
              tr(task.status),
          ),
          UI.bar(task.progress, task.quantity, "Выполнено"),
        );
        if (task.error) card.append(n("p", tr(task.error), "planner-message"));
        if (["queued", "running", "paused"].includes(task.status)) {
          const tools = n("div", undefined, "inline-controls");
          for (const [action, label] of [
            [
              task.status === "paused" ? "resume" : "pause",
              task.status === "paused" ? "Продолжить" : "Приостановить",
            ],
            ["raise", "Выше в очереди"],
            ["lower", "Ниже в очереди"],
            ["activate", "Сделать активной"],
            ["cancel", "Отменить задачу"],
          ])
            tools.append(
              button(label, async () => {
                await api("planner", { action, id: task.id });
                await refresh(true);
                Scheduler.render(root);
              }),
            );
          card.append(tools);
        }
        groups.get(task.ship_id)?.append(card);
      }
    },
  };
  const grantRoot = Modules.create(
    "resourcegrant",
    "Выдача ресурсов",
    760,
    670,
  );
  let targetUser = 0,
    grantRequest = 0;
  window.AdminResources = {
    async render() {
      if (!signedIn || gameData.state?.user.role !== "admin") {
        Modules.close("resourcegrant");
        return;
      }
      const epoch = authEpoch,
        key = ++grantRequest;
      targetUser ||= gameData.state.user.id;
      const info = await api("admin/resources?user=" + targetUser);
      if (
        epoch !== authEpoch ||
        key !== grantRequest ||
        !Modules.isOpen("resourcegrant")
      )
        return;
      grantRoot.replaceChildren();
      const search = UI.field(
          grantRoot,
          "Поиск игрока",
          "grant-user-search",
          "",
          "search",
        ),
        users = n("div");
      grantRoot.append(users);
      let userSearchRequest = 0,
        page = 1;
      const drawUsers = async () => {
        const req = ++userSearchRequest;
        const result = await api(
          "admin/users?search=" +
            encodeURIComponent(search.value) +
            "&page=" +
            page,
        );
        if (
          req !== userSearchRequest ||
          epoch !== authEpoch ||
          key !== grantRequest
        )
          return;
        users.replaceChildren();
        for (const u of result.users || result.items || [])
          users.append(
            button(u.username, () => {
              targetUser = u.id;
              AdminResources.render();
            }),
          );
      };
      search.oninput = () => {
        page = 1;
        drawUsers().catch((e) => notify(e.message, "error"));
      };
      const userName = await api("admin/users?search=" + targetUser);
      if (epoch !== authEpoch || key !== grantRequest) return;
      const selected = (userName.users || userName.items || []).find(
        (u) => u.id === targetUser,
      );
      grantRoot.append(
        n(
          "p",
          tr("Игрок") +
            ": " +
            (selected?.username || gameData.state.user.username),
        ),
      );
      const form = n("form", undefined, "resource-grant-form");
      form.onsubmit = (e) => e.preventDefault();
      grantRoot.append(form);
      let target = "ship",
        ship = info.ships[0]?.id,
        company = info.companies[0]?.id,
        port = (gameData.workbench.stations || [])[0],
        resource = gameData.expedition.resources[0].id;
      const controls = n("div");
      UI.choice(
        form,
        "Место выдачи",
        [
          ["ship", "Трюм корабля"],
          ["warehouse", "Личный склад на станции"],
          ["company", "Склад корпорации"],
        ],
        target,
        (value) => {
          target = value;
          drawTargets();
        },
      );
      function drawTargets() {
        controls.replaceChildren();
        if (target === "ship")
          UI.choice(
            controls,
            "Корабль",
            info.ships.map((s) => [
              s.id,
              UI.shipName(s) +
                " · " +
                (window.MapData?.stars[s.star - 1]?.name || ""),
            ]),
            ship,
            (value) => (ship = Number(value)),
          );
        else {
          if (target === "company")
            UI.choice(
              controls,
              "Корпорация",
              info.companies.map((c) => [c.id, "[" + c.tag + "] " + c.name]),
              company,
              (value) => (company = Number(value)),
            );
          if (port) {
            const label = n("label");
            label.append(n("span", tr("Станция")));
            UI.stationPicker(
              label,
              gameData.workbench.stations,
              stationKey(port),
              (value) => (port = parseStation(value)),
            );
            controls.append(label);
          }
        }
      }
      drawTargets();
      form.append(controls);
      UI.choice(
        form,
        "Ресурс",
        gameData.expedition.resources.map((r) => [r.id, r.name]),
        resource,
        (value) => (resource = value),
      );
      const quantity = UI.field(
        form,
        "Количество",
        "grant-quantity",
        1,
        "number",
      );
      quantity.min = 1;
      quantity.max = 1000000;
      quantity.step = 1;
      form.append(
        button("Выдать ресурсы", async () => {
          if (!form.reportValidity()) return;
          await api("admin/resources", {
            user: targetUser,
            ship,
            company,
            target,
            star: port?.star,
            station: port?.station || port?.object_id,
            resource,
            quantity: Number(quantity.value),
          });
          notify(tr("Ресурсы выданы"));
          await refresh(true);
        }),
      );
    },
    open(user) {
      targetUser = user || gameData.state.user.id;
      Modules.open("resourcegrant");
    },
  };
  window.addEventListener("moduleopen", (e) => {
    if (e.detail === "resourcegrant")
      AdminResources.render().catch((e) => notify(e.message, "error"));
  });
  window.addEventListener("signedout", () => {
    request++;
    grantRequest++;
    targetUser = 0;
    grantRoot.replaceChildren();
    Modules.close("resourcegrant");
  });
})();
