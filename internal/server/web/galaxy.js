"use strict";
(() => {
  const root = document.querySelector("#window-galaxy .module-content"),
    tr = (s) => I18n.t(s),
    e = UI.node;
  root.classList.add("galaxy-content");
  const top = e("div", undefined, "map-toolbar"),
    back = UI.button("К галактике", () => {});
  back.hidden = true;
  let galaxyPicker = UI.button("Edem", () => {});
  galaxyPicker.hidden = true;
  let system = UI.button("Поиск системы", () => {});
  system.classList.add("system-picker");
  system.id = "map-system-id";
  system.value = 1;
  const inspect = e("button"),
    reset = UI.button("⌖", () => {}),
    minus = UI.button("−", () => {}),
    plus = UI.button("+", () => {}),
    label = e("span", undefined, "map-breadcrumb"),
    toggle = UI.button("☷", () => {
      body.classList.toggle("inspector-hidden");
      draw();
    });
  toggle.dataset.help = "Показать или скрыть панель";
  label.id = "map-breadcrumb";
  top.append(back, galaxyPicker, system, label, toggle);
  const filters = e("div", undefined, "map-filters"),
    flags = {
      ships: true,
      ports: true,
      planets: true,
      belts: true,
      labels: true,
      territories: true,
    };
  for (const [id, text] of [
    ["ships", "Мои корабли"],
    ["ports", "Порты"],
    ["planets", "Планеты"],
    ["belts", "Месторождения"],
    ["labels", "Подписи"],
    ["territories", "Владения корпораций"],
  ]) {
    const l = e("label"),
      i = e("input"),
      span = e("span", tr(text));
    l.dataset.filter = id;
    i.type = "checkbox";
    i.checked = true;
    i.onchange = () => {
      flags[id] = i.checked;
      draw();
    };
    l.append(i, span);
    filters.append(l);
  }
  const body = e("div", undefined, "map-workspace"),
    view = e("div", undefined, "map-canvas-wrap"),
    canvas = e("canvas"),
    side = e("aside", undefined, "map-inspector"),
    tools = e("div", undefined, "map-tools"),
    zoomLabel = e("span", "1×", "zoom-label"),
    legend = e("details", undefined, "map-legend");
  canvas.id = "galaxy-map";
  canvas.setAttribute("aria-label", "Edem interactive map");
  tools.append(minus, zoomLabel, plus, reset);
  reset.dataset.help = "Вместить карту";
  legend.append(
    e("summary", tr("Легенда")),
    e(
      "p",
      tr(
        "Янтарный — маршрут · голубой — свои корабли · цвет изученной звезды — её спектральный класс · полупрозрачные области — владения корпораций",
      ),
    ),
  );
  view.append(canvas, tools, legend);
  body.append(view, side);
  root.append(top, body);
  const ctx = canvas.getContext("2d");
  const summary = e("div", undefined, "map-system-summary");
  summary.id = "map-system-summary";
  summary.hidden = true;
  summary.setAttribute("role", "tooltip");
  document.body.append(summary);
  let territorySource,
    territories = [],
    hoverID = null,
    hoverTimer,
    hoverGeneration = 0;
  const summaryCache = new Map();
  let knownStars = new Set();
  function hideSummary() {
    hoverID = null;
    clearTimeout(hoverTimer);
    hoverGeneration++;
    summary.hidden = true;
  }
  canvas.onpointerleave = hideSummary;
  function buildTerritories() {
    if (territorySource === galaxy) return;
    territorySource = galaxy;
    territories = [];
    for (const definition of galaxy.galaxies || [])
      for (const area of definition.territories || []) {
        const center = galaxy.stars[area.anchor - 1],
          owner = galaxy.founders?.find((c) => c.founder_key === area.founder);
        if (center && owner)
          territories.push({
            galaxy: definition.id,
            center,
            radius: area.radius,
            color: owner.color,
          });
      }
  }
  function drawTerritories() {
    if (!flags.territories) return;
    ctx.save();
    for (const area of territories) {
      if (area.galaxy !== viewedGalaxy) continue;
      const p = galaxyPoint(area.center),
        edge = galaxyPoint({
          x: area.center.x + area.radius,
          y: area.center.y,
        }),
        radius = Math.abs(edge.x - p.x);
      ctx.beginPath();
      ctx.arc(p.x, p.y, radius, 0, Math.PI * 2);
      ctx.fillStyle = area.color;
      ctx.globalAlpha = 0.065;
      ctx.fill();
      ctx.strokeStyle = area.color;
      ctx.globalAlpha = 0.22;
      ctx.lineWidth = 1;
      ctx.stroke();
    }
    ctx.restore();
  }
  function hoverSummary(event) {
    const bounds = canvas.getBoundingClientRect(),
      x = event.clientX - bounds.x,
      y = event.clientY - bounds.y;
    let closest = null,
      distance = 9;
    for (const star of galaxy.stars) {
      if (star.galaxy !== viewedGalaxy) continue;
      const p = galaxyPoint(star),
        d = Math.hypot(x - p.x, y - p.y);
      if (d < distance) {
        distance = d;
        closest = star;
      }
    }
    if (!closest) {
      hideSummary();
      return;
    }
    summary.style.left =
      Math.max(8, Math.min(event.clientX + 16, innerWidth - 310)) + "px";
    summary.style.top =
      Math.max(8, Math.min(event.clientY + 16, innerHeight - 240)) + "px";
    if (hoverID === closest.id) return;
    hideSummary();
    hoverID = closest.id;
    const generation = hoverGeneration,
      epoch = authEpoch,
      star = closest;
    hoverTimer = setTimeout(async () => {
      if (hoverID !== star.id) return;
      const owner = galaxy.founders?.find(
          (c) => c.founder_key === star.founder,
        ),
        region = galaxy.regions?.find((r) => r.id === star.region);
      summary.replaceChildren(
        e("b", star.name),
        e(
          "small",
          tr(definition()?.name || "") + " · " + tr(region?.name || ""),
        ),
        e(
          "p",
          `${tr("Класс звезды")}: ${star.class} · ${star.mass} M☉ · ${star.luminosity} L☉`,
        ),
      );
      const badge = e(
        "span",
        owner
          ? `${owner.flag} [${owner.tag}] ${owner.name}`
          : tr("Нейтральная система"),
        "map-owner",
      );
      if (owner) badge.style.color = owner.color;
      summary.append(badge);
      const known = Game.known(star.id),
        progress =
          gameData.workbench?.research?.find((r) => r.star === star.id)
            ?.scanned || 0;
      summary.append(
        e(
          "p",
          tr(
            known
              ? "Система исследована"
              : progress
                ? "Сканирование раскрывает объекты по очереди."
                : "Объекты скрыты. Исследуйте систему, чтобы открыть их.",
          ),
          "muted",
        ),
      );
      summary.hidden = false;
      if (!known && !progress) return;
      const cacheKey = `${star.id}/${known}/${progress}/${galaxy.revision}`;
      try {
        let data = summaryCache.get(cacheKey);
        if (!data) {
          data = await api("system?id=" + star.id);
          summaryCache.set(cacheKey, data);
        }
        if (
          generation !== hoverGeneration ||
          epoch !== authEpoch ||
          hoverID !== star.id ||
          !signedIn
        )
          return;
        const text = [
          `${tr("Планеты")}: ${data.sites.filter((s) => s.details).length}`,
          `${tr("Порты")}: ${data.sites.filter((s) => s.kind === "Орбитальная станция").length}`,
          `${tr("Месторождения")}: ${data.sites.filter((s) => s.planet >= 101).length}`,
        ];
        summary.append(e("p", text.join(" · ")));
      } catch (error) {
        if (generation === hoverGeneration)
          summary.append(e("p", tr("Сводка временно недоступна"), "muted"));
      }
    }, 180);
  }
  let galaxy,
    local,
    viewedGalaxy = "edem",
    mode = "galaxy",
    camera = {
      galaxy: { zoom: 1, x: 0, y: 0 },
      system: { zoom: 1, x: 0, y: 0 },
    },
    pointer,
    route = [],
    selected = 1,
    selectedSite = null,
    sites = [],
    shipHits = [],
    request = 0,
    loading = false,
    pendingFind = null,
    pendingStar = null,
    hoveredSite = null,
    panelTab = "route",
    routeStops = [];
  const starGalaxy = (id) => galaxy?.stars[id - 1]?.galaxy || "edem";
  const definition = () => galaxy?.galaxies?.find((g) => g.id === viewedGalaxy);
  const galaxyTitle = () =>
    tr(definition()?.name || "Edem") +
    " · " +
    (galaxy?.stars.filter((s) => s.galaxy === viewedGalaxy).length || 0);
  function updateGalaxyPicker() {
    if (!galaxy) return;
    const entries = (galaxy.galaxies || []).filter((g) => g.opened);
    if (!entries.some((g) => g.id === viewedGalaxy))
      viewedGalaxy = entries[0]?.id || "edem";
    const picker = UI.picker(
      entries.map((g) => [g.id, tr(g.name)]),
      viewedGalaxy,
      (value) => {
        request++;
        viewedGalaxy = value;
        selected =
          fleet().find((ship) => starGalaxy(ship.star) === viewedGalaxy)
            ?.star ||
          galaxy.stars.find((s) => s.galaxy === viewedGalaxy)?.id ||
          1;
        mode = "galaxy";
        local = null;
        back.hidden = true;
        route = [];
        camera.galaxy = { zoom: 1, x: 0, y: 0 };
        updateSystemPicker();
        updateGalaxyPicker();
        routePanel();
        draw();
      },
      { raw: true },
    );
    picker.id = "map-galaxy";
    picker.classList.add("galaxy-picker");
    picker.hidden = entries.length < 2;
    galaxyPicker.replaceWith(picker);
    galaxyPicker = picker;
    if (mode === "galaxy") label.textContent = galaxyTitle();
    buildTerritories();
  }
  function updateSystemPicker() {
    if (!galaxy) return;
    const picker = UI.systems(selected, (id) => {
      request++;
      selected = Number(id);
      viewedGalaxy = starGalaxy(selected);
      updateGalaxyPicker();
      const star = galaxy.stars[selected - 1];
      mode = "galaxy";
      back.hidden = true;
      camera.galaxy = { zoom: 3, x: 0, y: 0 };
      const p = galaxyPoint(star);
      camera.galaxy.x = canvas.clientWidth / 2 - p.x;
      camera.galaxy.y = canvas.clientHeight / 2 - p.y;
      routePanel();
      draw();
      window.dispatchEvent(
        new CustomEvent("galaxyselection", { detail: selected }),
      );
    });
    picker.id = "map-system-id";
    picker.classList.add("system-picker");
    system.replaceWith(picker);
    system = picker;
  }
  const now = () => Date.now() + (gameData.serverOffset || 0),
    cam = () => camera[mode],
    fleet = () => gameData.fleet?.ships || [];
  function action(text, fn) {
    const b = e("button", tr(text));
    b.onclick = fn;
    return b;
  }
  function hint(text, message) {
    const n = e("span", text, "help");
    n.tabIndex = 0;
    n.dataset.help = message;
    return n;
  }
  function galaxyPoint(s) {
    const c = camera.galaxy,
      scale =
        (Math.min(canvas.clientWidth, canvas.clientHeight) /
          ((definition()?.radius || 1250) * 2)) *
        c.zoom;
    return {
      x: canvas.clientWidth / 2 + c.x + s.x * scale,
      y: canvas.clientHeight / 2 + c.y + s.y * scale,
    };
  }
  function localPoint(p) {
    const c = camera.system,
      w = canvas.clientWidth,
      h = canvas.clientHeight;
    return {
      x: w / 2 + c.x + (p.x - w / 2) * c.zoom,
      y: h / 2 + c.y + (p.y - h / 2) * c.zoom,
    };
  }
  function objectBase(id) {
    const s = sites.find((s) => s.id === id);
    if (s) return { x: s.baseX, y: s.baseY };
    return { x: canvas.clientWidth * 0.12, y: canvas.clientHeight * 0.82 };
  }
  function localShip(ship) {
    if (ship.destination) {
      return localPoint(objectBase("entry"));
    }
    const base = objectBase(ship.object_id);
    let to = base,
      angle = 0;
    if (ship.local_target) {
      const target = objectBase(ship.local_target),
        start = new Date(ship.local_started).getTime(),
        end = new Date(ship.local_arrival).getTime(),
        f = Math.max(
          0,
          Math.min(1, (now() - start) / Math.max(1, end - start)),
        );
      to = {
        x: base.x + (target.x - base.x) * f,
        y: base.y + (target.y - base.y) * f,
      };
      angle = Math.atan2(target.y - base.y, target.x - base.x);
    } else if (ship.position_mode === "orbit") {
      const phase = now() / 16000;
      to = {
        x: base.x + Math.cos(phase) * 20,
        y: base.y + Math.sin(phase) * 14,
      };
      angle = phase + Math.PI / 2;
    } else to = { x: base.x + 16, y: base.y + 11 };
    return { ...localPoint(to), angle };
  }
  function galaxyShip(ship) {
    const path = ship.path?.length ? ship.path : [ship.star];
    if (!ship.destination || path.length < 2)
      return galaxyPoint(galaxy.stars[ship.star - 1]);
    const start = new Date(ship.departure).getTime(),
      end = new Date(ship.arrival).getTime(),
      phase = Math.max(
        0,
        Math.min(
          path.length - 1,
          ((now() - start) / Math.max(1, end - start)) * (path.length - 1),
        ),
      ),
      i = Math.min(path.length - 2, Math.floor(phase)),
      t = phase - i;
    const a = galaxyPoint(galaxy.stars[path[i] - 1]),
      b = galaxyPoint(galaxy.stars[path[i + 1] - 1]);
    return {
      x: a.x + (b.x - a.x) * t,
      y: a.y + (b.y - a.y) * t,
      angle: Math.atan2(b.y - a.y, b.x - a.x),
    };
  }
  function draw() {
    if (mode !== "galaxy") hideSummary();
    zoomLabel.textContent = cam().zoom.toFixed(1) + "×";
    if (
      !galaxy ||
      !Modules.isOpen("galaxy") ||
      fleet().some((ship) => ship.star > galaxy.stars.length)
    )
      return;
    const ratio = devicePixelRatio || 1,
      w = canvas.clientWidth,
      h = canvas.clientHeight;
    if (!w || !h) return;
    if (
      canvas.width !== Math.round(w * ratio) ||
      canvas.height !== Math.round(h * ratio)
    ) {
      canvas.width = w * ratio;
      canvas.height = h * ratio;
    }
    ctx.lineWidth = 1;
    ctx.setLineDash([]);
    ctx.shadowBlur = 0;
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.clearRect(0, 0, w, h);
    shipHits = [];
    if (mode === "system" && local) {
      sites = [];
      const cx = w / 2,
        cy = h / 2,
        max = Math.max(1, ...local.sites.map((s) => Math.log(1 + s.orbit)));
      const center = localPoint({ x: cx, y: cy });
      ctx.fillStyle =
        {
          O: "#96baff",
          B: "#bdd5ff",
          A: "#edf1ff",
          F: "#fff0c4",
          G: "#ffdb78",
          K: "#f5ad65",
          M: "#ee8277",
        }[local.star.class] || "#efad58";
      ctx.shadowColor = ctx.fillStyle;
      ctx.shadowBlur = 18;
      ctx.beginPath();
      ctx.arc(center.x, center.y, 12 * Math.sqrt(cam().zoom), 0, Math.PI * 2);
      ctx.fill();
      ctx.shadowBlur = 0;
      ctx.font = "11px monospace";
      ctx.fillText(local.star.name, center.x + 18, center.y);
      for (let i = 0; i < local.sites.length; i++) {
        const s = local.sites[i],
          r =
            35 +
            (Math.log(1 + s.orbit) / max) *
              Math.max(20, Math.min(w, h) / 2 - 75),
          angle = i * 2.4 + local.star.id * 0.2,
          baseX = cx + Math.cos(angle) * r,
          baseY = cy + Math.sin(angle) * r * 0.72,
          p = localPoint({ x: baseX, y: baseY });
        const visible =
          s.kind === "Орбитальная станция"
            ? flags.ports
            : s.planet >= 101
              ? flags.belts
              : flags.planets;
        const radius =
          (s.kind === "Газовый гигант"
            ? 13
            : s.planet >= 101
              ? 13
              : s.kind === "Орбитальная станция"
                ? 10
                : 8) * Math.min(2.5, Math.sqrt(cam().zoom));
        sites.push({ ...s, x: p.x, y: p.y, baseX, baseY, radius, visible });
        if (!visible) continue;
        ctx.strokeStyle = "#28394b";
        ctx.setLineDash(s.planet >= 101 ? [2, 5] : []);
        ctx.beginPath();
        ctx.ellipse(
          center.x,
          center.y,
          r * cam().zoom,
          r * 0.72 * cam().zoom,
          0,
          0,
          Math.PI * 2,
        );
        ctx.stroke();
        ctx.setLineDash([]);
        Visuals.object(ctx, p.x, p.y, radius, s);
        if (s.id === selectedSite || s.id === hoveredSite) {
          ctx.strokeStyle = "#efad58";
          ctx.lineWidth = 1.5;
          ctx.beginPath();
          ctx.arc(p.x, p.y, radius + 5, 0, Math.PI * 2);
          ctx.stroke();
        }
        if (flags.labels) {
          ctx.fillStyle = "#c7d4df";
          ctx.font = "10px monospace";
          ctx.fillText(s.name, p.x + radius + 5, p.y - 5);
        }
      }
      const entry = localPoint(objectBase("entry"));
      ctx.strokeStyle = "#55839b";
      ctx.beginPath();
      ctx.arc(entry.x, entry.y, 9, 0, Math.PI * 2);
      ctx.stroke();
      if (flags.labels) {
        ctx.fillStyle = "#79a4bc";
        ctx.fillText(tr("Точка гиперперехода"), entry.x + 14, entry.y);
      }
      if (flags.ships)
        for (const ship of fleet()) {
          const inSystem =
            ship.star === local.star.id ||
            (ship.destination === local.star.id &&
              new Date(ship.arrival).getTime() <= now());
          if (!inSystem) continue;
          const p = localShip(ship);
          if (ship.local_target) {
            const a = localPoint(objectBase(ship.object_id)),
              b = localPoint(objectBase(ship.local_target));
            ctx.strokeStyle = "#7fdce388";
            ctx.setLineDash([3, 5]);
            ctx.beginPath();
            ctx.moveTo(a.x, a.y);
            ctx.lineTo(b.x, b.y);
            ctx.stroke();
            ctx.setLineDash([]);
          }
          Visuals.ship(ctx, p.x, p.y, 10, p.angle || 0);
          ctx.fillStyle = "#a8e6f3";
          ctx.fillText(
            tr(ship.name) +
              (ship.destination ? " · " + tr("Гиперпереход") : ""),
            p.x + 12,
            p.y - 13,
          );
          shipHits.push({ ship, x: p.x, y: p.y });
        }
      return;
    }
    drawTerritories();
    ctx.strokeStyle = "#273749";
    ctx.lineWidth = 0.6;
    ctx.beginPath();
    for (const lane of galaxy.lanes) {
      if (starGalaxy(lane.from) !== viewedGalaxy) continue;
      const a = galaxyPoint(galaxy.stars[lane.from - 1]),
        b = galaxyPoint(galaxy.stars[lane.to - 1]);
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
    }
    ctx.stroke();
    ctx.strokeStyle = "#efad58";
    ctx.lineWidth = 2;
    ctx.beginPath();
    for (let i = 1; i < route.length; i++) {
      const a = galaxyPoint(galaxy.stars[route[i - 1] - 1]),
        b = galaxyPoint(galaxy.stars[route[i] - 1]);
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
    }
    ctx.stroke();
    const colors = {
      O: "#96baff",
      B: "#bdd5ff",
      A: "#edf1ff",
      F: "#fff0c4",
      G: "#ffdb78",
      K: "#f5ad65",
      M: "#ee8277",
    };
    for (const s of galaxy.stars) {
      if (s.galaxy !== viewedGalaxy) continue;
      const p = galaxyPoint(s);
      ctx.fillStyle = knownStars.has(s.id)
        ? colors[s.class] || "#ffffff"
        : "#6d8296";
      ctx.beginPath();
      ctx.arc(
        p.x,
        p.y,
        Math.min(3.2, 1.15 * Math.sqrt(cam().zoom)),
        0,
        Math.PI * 2,
      );
      ctx.fill();
      if (s.id === selected) {
        ctx.strokeStyle = "#efad58";
        ctx.beginPath();
        ctx.arc(p.x, p.y, 8, 0, Math.PI * 2);
        ctx.stroke();
      }
      if (flags.labels && cam().zoom > 3) {
        ctx.font = "10px monospace";
        ctx.fillText(s.name, p.x + 5, p.y - 5);
      }
    }
    if (flags.labels) {
      ctx.font = "11px monospace";
      for (const region of galaxy.regions || []) {
        const stars = galaxy.stars.filter(
          (s) => s.region === region.id && s.galaxy === viewedGalaxy,
        );
        if (!stars.length) continue;
        const p = galaxyPoint({
          x: stars.reduce((n, s) => n + s.x, 0) / stars.length,
          y: stars.reduce((n, s) => n + s.y, 0) / stars.length,
        });
        ctx.fillStyle = "#dce5ed";
        ctx.fillText(tr(region.name), p.x, p.y);
      }
    }
    if (viewedGalaxy === "edem" && galaxy.black_hole) {
      const p = galaxyPoint(galaxy.black_hole);
      ctx.strokeStyle = "#70618c";
      ctx.lineWidth = 1;
      for (const id of galaxy.black_hole.systems) {
        const q = galaxyPoint(galaxy.stars[id - 1]);
        ctx.beginPath();
        ctx.moveTo(p.x, p.y);
        ctx.lineTo(q.x, q.y);
        ctx.stroke();
      }
      ctx.strokeStyle = "#c8a0ed";
      ctx.lineWidth = 3;
      ctx.beginPath();
      ctx.ellipse(p.x, p.y, 17, 7, 0.4, 0, Math.PI * 2);
      ctx.stroke();
      ctx.fillStyle = "#02030a";
      ctx.beginPath();
      ctx.arc(p.x, p.y, 8, 0, Math.PI * 2);
      ctx.fill();
    }
    if (flags.ships) {
      const groups = new Map();
      for (const ship of fleet()) {
        const p = galaxyShip(ship),
          key = ship.destination ? "flight-" + ship.id : "star-" + ship.star,
          group = groups.get(key) || [];
        group.push({ ship, p });
        groups.set(key, group);
      }
      for (const group of groups.values()) {
        const { ship, p } = group[0];
        if (starGalaxy(ship.star) !== viewedGalaxy) continue;
        ctx.strokeStyle = "#9de5f2";
        ctx.beginPath();
        ctx.arc(p.x, p.y, 15, 0, Math.PI * 2);
        ctx.stroke();
        Visuals.ship(ctx, p.x, p.y, 10, p.angle || 0);
        ctx.fillStyle = "#b8edf5";
        ctx.font = "11px monospace";
        ctx.fillText(
          group.length > 1
            ? `${group.length} ${tr("Кораблей")}`
            : tr(ship.name),
          p.x + 19,
          p.y - 10,
        );
        for (const item of group)
          shipHits.push({ ship: item.ship, x: p.x, y: p.y });
      }
    }
  }
  function fleetPanel() {
    const section = e("section");
    section.append(e("h3", tr("Мой флот")));
    for (const ship of fleet()) {
      const card = e("div", undefined, "fleet-card");
      card.append(
        e("b", tr(ship.name) + " #" + ship.id),
        e(
          "p",
          ship.destination
            ? `${tr("В пути")} · ${ship.star} → ${ship.destination}`
            : galaxy.stars[ship.star - 1].name,
        ),
        UI.button("Найти на карте", () => findShip(ship.id)),
      );
      if (ship.id !== gameData.expedition.ship.id)
        card.append(
          UI.button("Выбрать", () =>
            Game.command({ action: "select_ship", ship: ship.id }),
          ),
        );
      section.append(card);
    }
    side.append(section);
  }
  function panelTabs() {
    UI.tabs(
      side,
      [
        ["system", "Система"],
        ["route", "Маршрут"],
        ["fleet", "Флот"],
      ],
      panelTab,
      (value) => {
        panelTab = value;
        mode === "system" ? systemPanel() : routePanel();
      },
    );
  }
  function routePanel() {
    side.replaceChildren();
    panelTabs();
    if (panelTab === "fleet") {
      fleetPanel();
      return;
    }
    if (panelTab === "system") {
      const star = galaxy.stars[selected - 1];
      side.append(
        e("h3", star.name),
        e(
          "p",
          tr(
            Game.known(selected)
              ? "Система исследована"
              : "Неизвестная система",
          ),
        ),
        UI.button("Открыть систему", () => openSystem(selected), false),
        UI.button(
          "Исследовать",
          () => Game.explore(selected),
          Game.known(selected),
          "Система уже исследована",
        ),
      );
      const knowledge = gameData.workbench.known?.find(
        (item) => item.star === selected,
      );
      if (knowledge)
        side.append(
          e(
            "p",
            tr("Получено") +
              ": " +
              new Date(knowledge.received_at).toLocaleString(I18n.language) +
              " · " +
              tr(knowledge.source),
            "muted",
          ),
        );
      const note = gameData.workbench.notes?.find(
        (v) => v.star === selected && !v.object_id,
      );
      const text = UI.field(
        side,
        "Личная заметка",
        "system-note",
        note?.note || "",
      );
      side.append(
        UI.button("Сохранить заметку", () =>
          Game.command({
            action: "note",
            star: selected,
            note: text.value,
            bookmark: note?.bookmark || false,
          }),
        ),
        UI.button(note?.bookmark ? "Убрать закладку" : "В закладки", () =>
          Game.command({
            action: "note",
            star: selected,
            note: text.value,
            bookmark: !note?.bookmark,
          }),
        ),
      );
      return;
    }
    side.append(e("h3", tr("Планировщик маршрута")));
    const from = gameData.expedition?.ship.star || 1;
    UI.row(side, "Отправление", galaxy.stars[from - 1].name);
    const target = UI.systems(selected, (value) => {
      selected = Number(value);
      routePanel();
      draw();
    });
    target.id = "route-to";
    side.append(target);
    const result = e("p");
    result.id = "route-result";
    const details = e("details");
    details.append(e("summary", tr("Промежуточные точки")));
    details.append(
      UI.button("Добавить точку", () => {
        routeStops.push(selected);
        routePanel();
      }),
    );
    for (const [id, to] of routeStops.entries()) {
      const line = e("div", undefined, "inline-controls");
      line.append(
        e("span", galaxy.stars[to - 1]?.name || to),
        UI.button("×", () => {
          routeStops.splice(id, 1);
          routePanel();
        }),
      );
      details.append(line);
    }
    side.append(details);
    const plan = () => {
      const points = [...routeStops, selected];
      let current = from,
        full = [from];
      for (const to of points) {
        if (to === current) continue;
        const part = localPath(current, to);
        if (!part) return null;
        full.push(...part.slice(1));
        current = to;
      }
      return full;
    };
    const path = plan(),
      jumps = path ? path.length - 1 : 0;
    result.textContent = path
      ? `${jumps} ${tr("переходов")} · ${jumps * (gameData.expedition.ship.spec?.jump_seconds || 5)} s · ${jumps * (gameData.expedition.ship.spec?.jump_fuel || 2)} ${tr("Топливо")}`
      : tr("Нет пути: системы не связаны гиперкоридорами.");
    side.append(result);
    if (
      path &&
      jumps * (gameData.expedition.ship.spec?.jump_fuel || 2) * 2 >
        gameData.expedition.ship.fuel
    )
      side.append(
        e("p", tr("Топлива на обратный путь может не хватить"), "error"),
      );
    side.append(
      UI.button(
        "Показать маршрут",
        () => {
          route = path || [];
          draw();
        },
        !path,
      ),
      UI.button(
        "Отправиться",
        () =>
          Game.command({
            action: "route",
            waypoints: [...routeStops, selected],
          }),
        !path ||
          !jumps ||
          jumps * (gameData.expedition.ship.spec?.jump_fuel || 2) >
            gameData.expedition.ship.fuel,
        "Недостаточно топлива или нет пути",
      ),
    );
    const way = e("details");
    way.append(
      e("summary", tr("Системы маршрута")),
      e("p", path?.map((id) => galaxy.stars[id - 1].name).join(" → ") || "—"),
    );
    side.append(way);
    const bookmarks = e("details");
    bookmarks.append(e("summary", tr("Закладки")));
    for (const item of gameData.workbench.notes?.filter((n) => n.bookmark) ||
      [])
      bookmarks.append(
        UI.button(galaxy.stars[item.star - 1]?.name || item.star, () => {
          selected = item.star;
          updateSystemPicker();
          routePanel();
          draw();
        }),
      );
    side.append(bookmarks);
    const filter = e("details");
    filters
      .querySelectorAll("label")
      .forEach(
        (l) =>
          (l.hidden =
            mode === "galaxy" &&
            !["ships", "labels"].includes(l.dataset.filter)),
      );
    filter.append(e("summary", tr("Отображение")), filters);
    side.append(filter);
  }
  function localPath(from, to) {
    if (from === to) return [from];
    const adj = new Map();
    for (const lane of galaxy.lanes) {
      for (const [a, b] of [
        [lane.from, lane.to],
        [lane.to, lane.from],
      ]) {
        if (!adj.has(a)) adj.set(a, []);
        adj.get(a).push(b);
      }
    }
    const prev = new Map([[from, null]]),
      q = [from];
    for (let i = 0; i < q.length; i++) {
      const id = q[i];
      for (const next of adj.get(id) || []) {
        if (prev.has(next)) continue;
        prev.set(next, id);
        if (next === to) {
          const path = [];
          let v = to;
          while (v !== null) {
            path.unshift(v);
            v = prev.get(v);
          }
          return path;
        }
        q.push(next);
      }
    }
    return null;
  }
  function siteActions(site, x, y) {
    selectedSite = site.id;
    systemPanel();
    draw();
    const ship = gameData.expedition?.ship,
      current = ship?.star === local.star.id,
      moving = !!ship?.destination || !!ship?.local_target,
      blocked = !current || moving || !!gameData.expedition?.job,
      items = [
        { heading: site.name },
        { label: "Информация", action: () => UI.object(site, local.star) },
        {
          label: "Приблизиться",
          disabled: blocked,
          reason: current
            ? "Дождитесь завершения действия"
            : "Прибудьте в систему",
          action: () =>
            performExpedition({
              action: "move",
              target: site.id,
              mode: "approach",
            }),
        },
      ];
    if (site.details)
      items.push({
        label: "Перейти на орбиту",
        disabled: blocked,
        action: () =>
          performExpedition({ action: "move", target: site.id, mode: "orbit" }),
      });
    if (site.kind === "Орбитальная станция")
      items.push(
        {
          label: "Стыковаться",
          disabled: blocked,
          action: () =>
            performExpedition({
              action: "move",
              target: site.id,
              mode: "docked",
            }),
        },
        {
          label: "Рынок",
          action: () => {
            window.dispatchEvent(
              new CustomEvent("stationmarket", { detail: local.star.id }),
            );
            Modules.open("market");
          },
        },
      );
    if (site.resources?.length)
      items.push({
        label: "Разведка и добыча",
        action: () => Modules.open("research"),
      });
    UI.menu(items, x, y);
  }
  function systemPanel() {
    side.replaceChildren();
    panelTabs();
    if (panelTab === "fleet") {
      fleetPanel();
      return;
    }
    if (panelTab === "route") {
      routePanel();
      return;
    }
    side.append(e("h3", local.star.name), e("p", tr(local.region)));
    const stellar = e("p");
    stellar.append(
      hint(
        local.star.class,
        "Спектральный класс звезды: O, B, A, F, G, K, M — от горячих к холодным. Солнце относится к G.",
      ),
      document.createTextNode(" · "),
      hint(
        local.star.mass + " M☉",
        "M☉ — масса Солнца. Значение 1 означает массу, равную солнечной.",
      ),
      document.createTextNode(" · "),
      hint(
        local.star.luminosity + " L☉",
        "L☉ — светимость Солнца: полная мощность излучения, а не видимая яркость на небе.",
      ),
    );
    side.append(stellar);
    if (!local.surveyed)
      side.append(
        e(
          "p",
          tr(
            local.sites.length
              ? "Сканирование раскрывает объекты по очереди."
              : "Объекты скрыты. Исследуйте систему, чтобы открыть их.",
          ),
          "muted",
        ),
      );
    const chosen = local.sites.find((s) => s.id === selectedSite);
    if (chosen) {
      const model = e("canvas");
      model.width = 400;
      model.height = 200;
      model.className = "inspector-model";
      Visuals.object(model.getContext("2d"), 200, 100, 65, chosen);
      side.append(
        model,
        e("h3", chosen.name),
        UI.button("Все объекты", () => {
          selectedSite = null;
          systemPanel();
          draw();
        }),
      );
    }
    for (const [kind, name] of [
      ["Орбитальная станция", "Порты"],
      ["planet", "Планеты"],
      ["belt", "Месторождения"],
    ]) {
      const group = e("details");
      group.open = true;
      const objects = local.sites.filter((s) =>
        kind === "planet"
          ? !!s.details
          : kind === "belt"
            ? s.resources?.length && !s.details
            : s.kind === kind,
      );
      group.append(e("summary", tr(name) + " · " + objects.length));
      for (const site of objects) {
        const button = UI.button("", (ev) => {
          const r = button.getBoundingClientRect();
          siteActions(site, r.x, r.bottom);
        });
        button.className = "object-row";
        button.dataset.site = site.id;
        const icon = e("canvas");
        icon.width = 50;
        icon.height = 50;
        Visuals.object(icon.getContext("2d"), 25, 25, 15, site);
        button.append(icon, e("span", site.name));
        button.classList.toggle("selected", selectedSite === site.id);
        button.onpointerenter = () => {
          hoveredSite = site.id;
          draw();
        };
        button.onpointerleave = () => {
          hoveredSite = null;
          draw();
        };
        group.append(button);
      }
      side.append(group);
    }
    const display = e("details");
    display.append(e("summary", tr("Отображение")), filters);
    filters.querySelectorAll("label").forEach((l) => (l.hidden = false));
    side.append(display);
  }
  async function openSystem(id, notifySelection = true) {
    if (!Number.isInteger(id) || id < 1 || id > (galaxy?.stars.length || 0)) {
      notify("Некорректная система");
      return;
    }
    const key = ++request;
    try {
      const d = await api("system?id=" + id);
      if (key !== request || !signedIn) return;
      if (local?.star.id !== id) {
        selectedSite = null;
        camera.system = { zoom: 1, x: 0, y: 0 };
      }
      local = d;
      viewedGalaxy = starGalaxy(id);
      updateGalaxyPicker();
      mode = "system";
      panelTab = "system";
      selected = id;
      updateSystemPicker();
      back.hidden = false;
      label.textContent = d.star.name + " · " + tr(d.region);
      if (notifySelection)
        window.dispatchEvent(
          new CustomEvent("galaxyselection", { detail: id }),
        );
      systemPanel();
      draw();
    } catch (error) {
      notify(error.message);
    }
  }
  async function load() {
    if (galaxy) {
      if (window.MapData && window.MapData !== galaxy) {
        galaxy = window.MapData;
        updateSystemPicker();
      }
      if (mode === "galaxy") routePanel();
      draw();
      if (pendingFind) findShip(pendingFind);
      if (pendingStar) showStar(pendingStar);
      return;
    }
    if (loading) return;
    loading = true;
    try {
      galaxy = window.MapData || (await api("galaxy"));
      window.MapData = galaxy;
      selected = gameData.expedition.ship.star;
      viewedGalaxy = starGalaxy(selected);
      updateSystemPicker();
      updateGalaxyPicker();
      label.textContent = galaxyTitle();
      routePanel();
      draw();
      const focusRequested = pendingFind || pendingStar;
      if (pendingFind) findShip(pendingFind);
      if (pendingStar) showStar(pendingStar);
      if (!focusRequested) findShip(gameData.expedition.ship.id);
    } catch (error) {
      notify(error.message);
    } finally {
      loading = false;
    }
  }
  function showStar(id) {
    pendingStar = id;
    if (!galaxy) return;
    const star = galaxy.stars.find((s) => s.id === id);
    if (!star) return;
    request++;
    pendingStar = null;
    pendingFind = null;
    selected = id;
    viewedGalaxy = starGalaxy(id);
    mode = "galaxy";
    panelTab = "route";
    back.hidden = true;
    camera.galaxy = { zoom: 3, x: 0, y: 0 };
    const point = galaxyPoint(star);
    camera.galaxy.x = canvas.clientWidth / 2 - point.x;
    camera.galaxy.y = canvas.clientHeight / 2 - point.y;
    updateSystemPicker();
    updateGalaxyPicker();
    routePanel();
    draw();
  }
  window.addEventListener("showstar", (event) =>
    showStar(Number(event.detail)),
  );
  function findShip(id) {
    pendingFind = id;
    if (!galaxy) return;
    const ship = fleet().find((s) => s.id === id);
    if (!ship) return;
    request++;
    pendingFind = null;
    pendingStar = null;
    selected = ship.star;
    viewedGalaxy = starGalaxy(ship.star);
    updateSystemPicker();
    updateGalaxyPicker();
    mode = "galaxy";
    back.hidden = true;
    camera.galaxy.zoom = 2;
    camera.galaxy.x = 0;
    camera.galaxy.y = 0;
    const p = galaxyShip(ship);
    camera.galaxy.x = canvas.clientWidth / 2 - p.x;
    camera.galaxy.y = canvas.clientHeight / 2 - p.y;
    label.textContent = galaxyTitle();
    routePanel();
    draw();
  }
  function zoomAt(
    factor,
    x = canvas.clientWidth / 2,
    y = canvas.clientHeight / 2,
  ) {
    const c = cam(),
      next = Math.max(0.5, Math.min(18, c.zoom * factor)),
      scale = next / c.zoom;
    c.x =
      x - canvas.clientWidth / 2 - (x - canvas.clientWidth / 2 - c.x) * scale;
    c.y =
      y - canvas.clientHeight / 2 - (y - canvas.clientHeight / 2 - c.y) * scale;
    c.zoom = next;
    draw();
  }
  inspect.onclick = () => openSystem(Number(system.value));
  back.onclick = () => {
    request++;
    mode = "galaxy";
    back.hidden = true;
    label.textContent = galaxyTitle();
    routePanel();
    draw();
  };
  reset.onclick = () => {
    camera[mode] = { zoom: 1, x: 0, y: 0 };
    draw();
  };
  minus.onclick = () => zoomAt(0.8);
  plus.onclick = () => zoomAt(1.25);
  canvas.addEventListener(
    "wheel",
    (event) => {
      event.preventDefault();
      event.stopPropagation();
      const r = canvas.getBoundingClientRect();
      zoomAt(
        Math.exp(-event.deltaY * 0.001),
        event.clientX - r.x,
        event.clientY - r.y,
      );
    },
    { passive: false },
  );
  canvas.onpointerdown = (event) => {
    if (event.button !== 0) return;
    hideSummary();
    pointer = {
      x: event.clientX,
      y: event.clientY,
      startX: event.clientX,
      startY: event.clientY,
      moved: false,
    };
    canvas.setPointerCapture(event.pointerId);
  };
  canvas.onpointermove = (event) => {
    if (!pointer) {
      if (mode === "galaxy") hoverSummary(event);
      if (mode === "system") {
        const r = canvas.getBoundingClientRect(),
          hit = sites.find(
            (s) =>
              s.visible &&
              Math.hypot(event.clientX - r.x - s.x, event.clientY - r.y - s.y) <
                s.radius + 7,
          );
        if (hoveredSite !== hit?.id) {
          hoveredSite = hit?.id;
          side
            .querySelectorAll("[data-site]")
            .forEach((b) =>
              b.classList.toggle(
                "selected",
                b.dataset.site === hoveredSite ||
                  b.dataset.site === selectedSite,
              ),
            );
          draw();
        }
      }
      return;
    }
    pointer.moved ||=
      Math.hypot(
        event.clientX - pointer.startX,
        event.clientY - pointer.startY,
      ) > 5;
    cam().x += event.clientX - pointer.x;
    cam().y += event.clientY - pointer.y;
    pointer.x = event.clientX;
    pointer.y = event.clientY;
    draw();
  };
  canvas.onpointercancel = () => (pointer = null);
  canvas.onlostpointercapture = () => (pointer = null);
  function clickMap(event) {
    if (!galaxy) return;
    hideSummary();
    const r = canvas.getBoundingClientRect(),
      x = event.clientX - r.x,
      y = event.clientY - r.y,
      hit = shipHits.find((s) => Math.hypot(x - s.x, y - s.y) < 13);
    if (hit) {
      UI.menu(
        [
          { heading: hit.ship.name },
          { label: "Корабль", action: () => Modules.open("ship") },
          { label: "К системе", action: () => openSystem(hit.ship.star) },
        ],
        event.clientX,
        event.clientY,
      );
      return;
    }
    if (mode === "system") {
      const center = localPoint({
        x: canvas.clientWidth / 2,
        y: canvas.clientHeight / 2,
      });
      if (Math.hypot(x - center.x, y - center.y) < 20) {
        selectedSite = null;
        systemPanel();
        draw();
        return;
      }
      const candidates = sites
        .filter((s) => s.visible && Math.hypot(x - s.x, y - s.y) < s.radius + 8)
        .sort(
          (a, b) => Math.hypot(x - a.x, y - a.y) - Math.hypot(x - b.x, y - b.y),
        );
      if (candidates.length > 1) {
        UI.menu(
          candidates.map((s) => ({
            label: s.name + " · " + tr(s.kind),
            action: () => siteActions(s, event.clientX, event.clientY),
          })),
          event.clientX,
          event.clientY,
        );
      } else if (candidates[0])
        siteActions(candidates[0], event.clientX, event.clientY);
      return;
    }
    let closest,
      distance = 9;
    for (const star of galaxy.stars) {
      if (star.galaxy !== viewedGalaxy) continue;
      const p = galaxyPoint(star),
        d = Math.hypot(x - p.x, y - p.y);
      if (d < distance) {
        distance = d;
        closest = star;
      }
    }
    if (closest) {
      selected = closest.id;
      updateSystemPicker();
      draw();
      const star = closest;
      UI.menu(
        [
          { heading: star.name },
          {
            label: "Построить маршрут",
            action: () => {
              panelTab = "route";
              routePanel();
            },
          },
          {
            label: "Открыть систему",
            action: () => openSystem(star.id),
          },
          {
            label: "Исследовать",
            disabled: Game.known(star.id),
            reason: "Система уже исследована",
            action: () => Game.explore(star.id),
          },
          ...(gameData.state?.user.role === "admin"
            ? [
                {
                  label: "Телепортироваться",
                  action: () => Game.teleport(star.id),
                },
              ]
            : []),
        ],
        event.clientX,
        event.clientY,
      );
    } else if (viewedGalaxy === "edem" && galaxy.black_hole) {
      const p = galaxyPoint(galaxy.black_hole);
      if (Math.hypot(x - p.x, y - p.y) < 22) Modules.open("portal");
    }
  }
  canvas.onpointerup = (event) => {
    if (!pointer) return;
    const moved = pointer.moved;
    pointer = null;
    if (!moved) clickMap(event);
  };
  canvas.oncontextmenu = (event) => {
    event.preventDefault();
    clickMap(event);
  };
  window.addEventListener("universechange", () => {
    request++;
    hideSummary();
    summaryCache.clear();
    galaxy = window.MapData;
    selected = gameData.expedition.ship.star;
    viewedGalaxy = starGalaxy(selected);
    updateGalaxyPicker();
    label.textContent = galaxyTitle();
    back.hidden = true;
    mode = "galaxy";
    local = null;
    route = [];
    camera.galaxy = { zoom: 1, x: 0, y: 0 };
    updateSystemPicker();
    routePanel();
    draw();
  });
  window.addEventListener("galaxyselection", (ev) => {
    if (!galaxy) return;
    selected = ev.detail;
    updateSystemPicker();
    if (mode === "galaxy") routePanel();
    draw();
  });
  window.addEventListener("findship", (event) => {
    pendingFind = event.detail;
    if (Modules.isOpen("galaxy")) findShip(event.detail);
  });
  window.addEventListener("moduleopen", (event) => {
    if (event.detail === "galaxy") load();
  });
  window.addEventListener("moduleclose", (event) => {
    if (event.detail === "galaxy") hideSummary();
  });
  window.addEventListener("moduleresize", (event) => {
    if (event.detail === "galaxy") draw();
  });
  window.addEventListener("languagechange", () => {
    if (mode === "system" && local) openSystem(local.star.id, false);
    else if (galaxy) routePanel();
    draw();
  });
  window.addEventListener("gamedata", () => {
    knownStars = new Set(
      (gameData.workbench?.known || []).map((item) => item.star),
    );
    if (galaxy && gameData.workbench?.universe_revision > galaxy.revision) {
      if (!loading) {
        loading = true;
        api("galaxy")
          .then((data) => {
            if (!signedIn) return;
            window.MapData = data;
            window.dispatchEvent(new Event("universechange"));
          })
          .catch((error) => notify(error.message, "error"))
          .finally(() => (loading = false));
      }
      return;
    }

    if (
      mode === "galaxy" &&
      galaxy &&
      !side.contains(document.activeElement) &&
      !document.querySelector(".picker-popover")
    )
      routePanel();
    if (
      mode === "system" &&
      local &&
      Modules.isOpen("galaxy") &&
      !side.contains(document.activeElement) &&
      !document.querySelector(".picker-popover")
    )
      openSystem(local.star.id, false);
  });
  window.addEventListener("signedout", () => {
    hideSummary();
    summaryCache.clear();
    request++;
    pendingStar = null;
    local = null;
    mode = "galaxy";
    route = [];
    pendingFind = null;
    back.hidden = true;
    side.replaceChildren();
  });
  new ResizeObserver(draw).observe(canvas);
  setInterval(draw, 100);
})();
