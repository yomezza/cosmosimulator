"use strict";
(() => {
  const n = UI.node,
    tr = (s) => I18n.t(s),
    b = UI.button;
  const root = Modules.create("logistics", "Логистика", 1120, 720);
  root.classList.add("logistics-content");
  let data = null,
    request = 0,
    open = new Set(),
    query = "",
    scope = "all",
    drag = null,
    suppressClick = 0,
    pending = false,
    formOpen = false,
    operation = 0;
  const uuid = () =>
    [...crypto.getRandomValues(new Uint32Array(4))]
      .map((v) => v.toString(16).padStart(8, "0"))
      .join("");
  const resources = () => gameData.expedition?.resources || [],
    resource = (id) => resources().find((r) => r.id === id),
    ports = () => gameData.workbench?.stations || [];
  const key = (p) => p.star + "|" + (p.station || p.object_id),
    stock = (kind, p) =>
      ((kind === "company" ? data?.corporate : data?.personal) || []).find(
        (s) => key(s) === key(p),
      )?.cargo || {};
  const built = (p) => (data?.built || []).some((s) => key(s) === key(p));
  const ship = () => gameData.expedition?.ship;
  const shipAt = (s, p) =>
    s &&
    !s.destination &&
    !s.local_target &&
    s.position_mode === "docked" &&
    s.star === p.star &&
    s.object_id === (p.station || p.object_id);
  const same = (a, b) =>
    a.kind === b.kind &&
    (a.kind === "ship"
      ? a.ship === b.ship
      : a.star === b.star &&
        a.station === b.station &&
        (a.kind !== "company" || a.company === b.company));
  function label(loc) {
    if (loc.kind === "ship") {
      const s = gameData.fleet.ships.find((s) => s.id === loc.ship);
      return tr("Трюм") + " · " + (s ? UI.shipName(s) : "");
    }
    const p = ports().find((p) => key(p) === loc.star + "|" + loc.station);
    return (
      (p?.name || window.MapData?.stars[loc.star - 1]?.name || tr("Станция")) +
      " · " +
      tr(loc.kind === "company" ? "Корпоративный склад" : "Личный склад")
    );
  }
  function cleanDrag() {
    drag = null;
    document.body.classList.remove("cargo-dragging");
    document
      .querySelectorAll(".cargo-drop-ready")
      .forEach((e) => e.classList.remove("cargo-drop-ready"));
    suppressClick = performance.now() + 200;
  }
  const apiError = (e) => notify(e.message, "error");
  async function refresh() {
    if (!signedIn || !Modules.isOpen("logistics")) return;
    const id = ++request,
      epoch = authEpoch;
    const result = await api("logistics");
    if (
      id !== request ||
      epoch !== authEpoch ||
      !signedIn ||
      !Modules.isOpen("logistics")
    )
      return;
    data = result;
    if (!drag && !formOpen) draw();
  }
  function local(from, to) {
    const fromShip =
        from.kind === "ship"
          ? gameData.fleet.ships.find((s) => s.id === from.ship)
          : null,
      toShip =
        to.kind === "ship"
          ? gameData.fleet.ships.find((s) => s.id === to.ship)
          : null,
      carrier = fromShip || toShip || ship(),
      a = fromShip
        ? { star: fromShip.star, station: fromShip.object_id }
        : from,
      c = toShip ? { star: toShip.star, station: toShip.object_id } : to;
    return (
      carrier &&
      shipAt(carrier, a) &&
      a.star === c.star &&
      a.station === c.station &&
      (!fromShip || shipAt(fromShip, c)) &&
      (!toShip || shipAt(toShip, a))
    );
  }
  function transfer(from, to, id, q) {
    if (same(from, to) || pending) return;
    const carrier =
      from.kind === "ship"
        ? gameData.fleet.ships.find((s) => s.id === from.ship)
        : to.kind === "ship"
          ? gameData.fleet.ships.find((s) => s.id === to.ship)
          : ship();
    if (!carrier) return;
    const mode = local(from, to) ? "direct" : "queue",
      r = resource(id),
      holder = Modules.create(
        "cargo-transfer",
        "Перемещение ресурса",
        560,
        450,
      );
    formOpen = true;
    holder.replaceChildren();
    holder.append(
      n("h3", tr(r?.name || id)),
      n("p", label(from)),
      n("p", "→ " + label(to), "transfer-target"),
      n(
        "p",
        tr(
          mode === "direct"
            ? "Перенос на текущей станции"
            : "Будет создана задача: корабль перевезёт ресурс по существующему маршруту.",
        ),
        "muted",
      ),
    );
    let max = Math.min(q, 100000);
    if (to.kind === "ship") {
      const target = gameData.fleet.ships.find((s) => s.id === to.ship),
        reserved = (gameData.workbench.mining || [])
          .filter((j) => j.ship_id === target.id)
          .reduce((a, j) => a + j.quantity, 0);
      max = Math.min(
        max,
        target.spec.cargo -
          Object.values(target.cargo).reduce((a, v) => a + v, 0) -
          reserved,
      );
    }
    const quantity = UI.field(
      holder,
      "Количество",
      "cargo-transfer-quantity",
      Math.max(1, max),
      "number",
    );
    quantity.min = 1;
    quantity.max = Math.max(0, max);
    quantity.step = 1;
    const requestID = uuid(),
      submit = b(
        mode === "direct" ? "Переместить" : "Создать доставку",
        async () => {
          if (!quantity.reportValidity() || pending) return;
          const serial = ++operation,
            epoch = authEpoch;
          pending = true;
          cancel.disabled = true;
          quantity.disabled = true;
          try {
            await api("logistics", {
              request: requestID,
              mode,
              from,
              to,
              ship: carrier.id,
              resource: id,
              quantity: Number(quantity.value),
            });
            if (serial !== operation || epoch !== authEpoch || !signedIn)
              return;
            Modules.close("cargo-transfer");
            notify(
              tr(mode === "direct" ? "Ресурс перемещён" : "Задача добавлена"),
            );
            await window.refresh(true);
            await refresh();
          } catch (e) {
            if (serial === operation && epoch === authEpoch && signedIn)
              apiError(e);
          } finally {
            if (serial === operation) {
              pending = false;
              cancel.disabled = false;
              quantity.disabled = false;
            }
          }
        },
        max < 1,
        max < 1 ? "Трюм заполнен" : "",
      ),
      cancel = b("Отмена", () => Modules.close("cargo-transfer"));
    holder.append(submit, cancel);
    Modules.open("cargo-transfer");
  }
  function draggable(tile, from, id, q, enabled = true) {
    tile.draggable = enabled;
    tile.dataset.cargoResource = id;
    if (!enabled) return;
    tile.classList.add("cargo-draggable");
    const prior = tile.onclick;
    tile.onclick = (e) => {
      if (performance.now() < suppressClick) return;
      return prior?.call(tile, e);
    };
    tile.ondragstart = (e) => {
      if (pending || formOpen) {
        e.preventDefault();
        return;
      }
      const token = uuid();
      drag = { from, id, q, token };
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("application/x-cosmos-cargo", token);
      tile.classList.add("cargo-drag-source");
      document.body.classList.add("cargo-dragging");
    };
    tile.ondragend = () => {
      tile.classList.remove("cargo-drag-source");
      cleanDrag();
    };
  }
  function dropzone(element, target, allowed = () => true) {
    let timer;
    const get = () => (typeof target === "function" ? target() : target);
    element.classList.add("cargo-dropzone");
    element.tabIndex = element.tabIndex < 0 ? 0 : element.tabIndex;
    element.ondragover = (e) => {
      if (!drag || !allowed() || same(drag.from, get())) return;
      e.preventDefault();
      e.stopPropagation();
      e.dataTransfer.dropEffect = "move";
      element.classList.add("cargo-drop-ready");
      if (element.tagName === "DETAILS" && !element.open && !timer)
        timer = setTimeout(() => {
          element.open = true;
          open.add(element.dataset.station);
        }, 450);
    };
    element.ondragleave = (e) => {
      if (element.contains(e.relatedTarget)) return;
      clearTimeout(timer);
      timer = null;
      element.classList.remove("cargo-drop-ready");
    };
    element.ondrop = (e) => {
      e.preventDefault();
      e.stopPropagation();
      clearTimeout(timer);
      timer = null;
      const item = drag,
        target = get();
      if (!item || !allowed() || same(item.from, target)) return;
      const token = e.dataTransfer.getData("application/x-cosmos-cargo");
      if (token !== item.token) return;
      cleanDrag();
      transfer(item.from, target, item.id, item.q);
    };
  }
  function tile(from, id, q, canMove) {
    const r = resource(id),
      tile = b("", (e) => {
        const rect = tile.getBoundingClientRect();
        UI.menu(
          [
            { heading: r?.name || id },
            { label: "Информация", action: () => UI.resource(id) },
            {
              label: "Переместить ресурс",
              disabled: !canMove,
              reason: "Недостаточно корпоративных полномочий",
              action: () => chooseDestination(from, id, q),
            },
          ],
          e.clientX || rect.x,
          e.clientY || rect.bottom,
        );
      });
    tile.className = "cargo-tile" + (r?.rarity === "Редкий" ? " rare" : "");
    tile.dataset.resource = id;
    tile.append(
      n("small", tr(r?.kind || "Ресурс")),
      n("b", r?.code || id),
      n("span", Number(q).toLocaleString(I18n.language)),
    );
    draggable(tile, from, id, q, canMove);
    return tile;
  }
  function grid(from, cargo, canMove) {
    const grid = n("div", undefined, "cargo-grid");
    for (const [id, q] of Object.entries(cargo || {})) {
      if (q > 0) grid.append(tile(from, id, q, canMove));
    }
    if (!grid.children.length)
      grid.append(
        n(
          "p",
          tr("Здесь пока пусто. Перетащите ресурс для переноса."),
          "cargo-drop-hint",
        ),
      );
    return grid;
  }
  async function chooseDestination(from, id, q) {
    const epoch = authEpoch;
    try {
      const info = await api("logistics");
      if (epoch !== authEpoch || !signedIn) return;
      data = info;
    } catch (e) {
      if (epoch === authEpoch && signedIn) apiError(e);
      return;
    }
    const holder = Modules.create(
        "cargo-destination",
        "Место назначения",
        620,
        420,
      ),
      options = [];
    if (ship())
      options.push({
        id: "ship",
        label: tr("Трюм") + " · " + UI.shipName(ship()),
        location: { kind: "ship", ship: ship().id },
      });
    for (const p of ports()) {
      options.push({
        id: "personal:" + key(p),
        code: String(p.star),
        label: p.name + " · " + tr("Личный склад"),
        location: { kind: "personal", star: p.star, station: p.object_id },
      });
      if (data?.can_deposit && built(p))
        options.push({
          id: "company:" + key(p),
          code: String(p.star),
          label: p.name + " · " + tr("Корпоративный склад"),
          location: {
            kind: "company",
            star: p.star,
            station: p.object_id,
            company: data.company,
          },
        });
    }
    const available = options.filter((o) => !same(from, o.location));
    holder.replaceChildren(n("h3", tr(resource(id)?.name || id)));
    if (!available.length) {
      UI.empty(holder, "Нет доступных складов");
      Modules.open("cargo-destination");
      return;
    }
    let selected = available[0].id;
    holder.append(
      UI.picker(available, selected, (v) => (selected = v), { system: true }),
      b("Продолжить", () => {
        Modules.close("cargo-destination");
        transfer(
          from,
          available.find((o) => o.id === selected).location,
          id,
          q,
        );
      }),
    );
    Modules.open("cargo-destination");
  }
  function draw() {
    if (!data || drag || formOpen) return;
    const focus = document.activeElement?.id,
      caret = document.activeElement?.selectionStart,
      scroll = root.querySelector(".storage-tree")?.scrollTop || 0;
    root.replaceChildren();
    const top = n("div", undefined, "logistics-toolbar"),
      search = UI.field(
        top,
        "Поиск станций и ресурсов",
        "logistics-search",
        query,
        "search",
      );
    search.oninput = () => {
      query = search.value;
      draw();
    };
    UI.choice(
      top,
      "Показывать",
      [
        ["all", "Все станции"],
        ["stock", "С запасами"],
        ["system", "Текущая система"],
      ],
      scope,
      (v) => {
        scope = v;
        draw();
      },
    );
    top.append(b("Планировщик задач", () => Modules.open("tasks")));
    root.append(top);
    const layout = n("div", undefined, "logistics-layout"),
      hold = n("aside", undefined, "logistics-hold"),
      main = n("section", undefined, "storage-tree"),
      s = ship();
    layout.append(hold, main);
    root.append(layout);
    if (s) {
      hold.append(
        n("h3", UI.shipName(s)),
        n(
          "p",
          (window.MapData?.stars[s.star - 1]?.name || "") +
            " · " +
            tr(s.position_mode),
          "muted",
        ),
      );
      const reserved = (gameData.workbench.mining || [])
        .filter((j) => j.ship_id === s.id)
        .reduce((a, j) => a + j.quantity, 0);
      hold.append(
        UI.bar(
          Object.values(s.cargo).reduce((a, v) => a + v, 0) + reserved,
          s.spec.cargo,
          "Трюм",
        ),
        grid({ kind: "ship", ship: s.id }, s.cargo, true),
      );
      dropzone(hold, { kind: "ship", ship: s.id });
      hold.append(
        n(
          "p",
          tr("Перетащите плитку ресурса на нужный склад или в трюм."),
          "muted",
        ),
      );
    }
    const q = query.trim().toLowerCase();
    const filtered = ports()
      .filter((p) => {
        const inventories = [stock("personal", p), stock("company", p)],
          has = inventories.some((c) => Object.values(c).some((v) => v > 0));
        return (
          (scope !== "stock" || has) &&
          (scope !== "system" || p.star === s?.star) &&
          (!q ||
            [
              p.name,
              window.MapData?.stars[p.star - 1]?.name,
              p.owner_name,
              ...inventories.flatMap((c) =>
                Object.keys(c)
                  .filter((id) => c[id] > 0)
                  .flatMap((id) => [
                    tr(resource(id)?.name || id),
                    resource(id)?.code || id,
                  ]),
              ),
            ].some((v) =>
              String(v || "")
                .toLowerCase()
                .includes(q),
            ))
        );
      })
      .sort(
        (a, b) =>
          Number(b.star === s?.star) - Number(a.star === s?.star) ||
          a.name.localeCompare(b.name),
      );
    main.append(
      n("p", tr("Доступные станции") + ": " + filtered.length, "storage-count"),
    );
    for (const p of filtered) {
      const details = n("details", undefined, "storage-station"),
        summary = n("summary"),
        name = n("span", undefined, "storage-station-name"),
        count = [stock("personal", p), stock("company", p)]
          .flatMap((c) => Object.values(c))
          .reduce((a, v) => a + v, 0);
      details.dataset.station = key(p);
      details.open = open.has(key(p));
      name.append(
        n("b", p.name),
        n("small", window.MapData?.stars[p.star - 1]?.name || ""),
      );
      summary.append(
        n("span", "◉", "station-symbol"),
        name,
        n(
          "span",
          tr(shipAt(s, p) ? "На станции" : "Удалённый склад"),
          "storage-location",
        ),
        n("b", Number(count).toLocaleString(I18n.language), "storage-total"),
      );
      details.append(summary);
      const contents = n("div", undefined, "storage-contents");
      details.append(contents);
      const render = () => {
        if (contents.childElementCount) return;
        for (const kind of ["personal", "company"]) {
          if (kind === "company" && !data.company) continue;
          const enabled = kind === "personal" || built(p),
            section = n("section", undefined, "storage-inventory"),
            loc = {
              kind,
              star: p.star,
              station: p.object_id,
              ...(kind === "company" ? { company: data.company } : {}),
            };
          section.dataset.inventory = kind;
          section.append(
            n(
              "h4",
              tr(kind === "personal" ? "Личный склад" : "Корпоративный склад") +
                (kind === "company"
                  ? " [" + (gameData.profile.company_tag || "") + "]"
                  : ""),
            ),
          );
          if (enabled) {
            section.append(
              grid(
                loc,
                stock(kind, p),
                kind === "personal" || data.can_withdraw,
              ),
            );
            dropzone(
              section,
              loc,
              () => kind === "personal" || data.can_deposit,
            );
          } else {
            section.append(
              n("p", tr("Корпоративный склад ещё не построен"), "muted"),
              grid(loc, stock(kind, p), false),
            );
            section.ondragover = (e) => {
              e.preventDefault();
              e.stopPropagation();
              e.dataTransfer.dropEffect = "none";
            };
            section.ondrop = (e) => {
              e.preventDefault();
              e.stopPropagation();
            };
          }
          contents.append(section);
        }
      };
      details.ontoggle = () => {
        if (details.open) {
          open.add(key(p));
          render();
        } else open.delete(key(p));
      };
      if (details.open) render();
      dropzone(details, {
        kind: "personal",
        star: p.star,
        station: p.object_id,
      });
      main.append(details);
    }
    if (!filtered.length) UI.empty(main, "Ничего не найдено");
    const history = n("details", undefined, "logistics-history");
    history.append(n("summary", tr("История перемещений")));
    for (const entry of data.history || []) {
      const r = entry.result;
      history.append(
        n(
          "p",
          tr(resource(r.resource)?.name || r.resource) +
            " × " +
            r.quantity +
            " · " +
            label(r.from) +
            " → " +
            label(r.to) +
            " · " +
            tr(
              r.mode === "direct"
                ? "Перемещено"
                : entry.task_status || "queued",
            ),
        ),
      );
    }
    root.append(history);
    main.scrollTop = scroll;
    if (focus === "logistics-search") {
      search.focus();
      if (caret !== null) search.setSelectionRange(caret, caret);
    }
  }
  window.Logistics = {
    get dragging() {
      return !!drag;
    },
    draggable,
    dropzone,
    chooseDestination,
    refresh,
  };
  window.addEventListener("moduleopen", (e) => {
    if (e.detail === "logistics") {
      if (!window.MapData)
        api("galaxy")
          .then((g) => {
            window.MapData = g;
            return refresh();
          })
          .catch(apiError);
      else refresh().catch(apiError);
    }
  });
  window.addEventListener("gamedata", () => {
    if (Modules.isOpen("logistics") && !drag) refresh().catch(apiError);
  });
  window.addEventListener("languagechange", () => {
    if (data && Modules.isOpen("logistics")) draw();
  });
  window.addEventListener("moduleclose", (e) => {
    if (e.detail === "cargo-transfer") {
      formOpen = false;
      if (Modules.isOpen("logistics")) refresh().catch(apiError);
    }
  });
  window.addEventListener("signedout", () => {
    request++;
    operation++;
    data = null;
    open.clear();
    query = "";
    scope = "all";
    formOpen = false;
    pending = false;
    cleanDrag();
    root.replaceChildren();
    for (const id of ["logistics", "cargo-transfer", "cargo-destination"])
      Modules.close(id);
  });
})();
