"use strict";
(() => {
  const n = UI.node,
    tr = (s) => I18n.t(s),
    btn = UI.button;
  let current = null,
    epoch = 0,
    polling = false,
    selected = 1;
  const actions = new Map();
  const statusName = (s) =>
    ({
      funding: "Финансирование",
      building: "Строительство",
      complete: "Колонизация завершена",
      cancelled: "Отменён",
      expired: "Просрочен",
    })[s] || s;
  const asset = (id) =>
    id === "credits"
      ? "CR"
      : tr(gameData.expedition.resources.find((r) => r.id === id)?.name || id);
  async function send(input) {
    const session = authEpoch;
    const key = JSON.stringify(input);
    let request = actions.get(key);
    if (!request) {
      request = Array.from(crypto.getRandomValues(new Uint32Array(4)), (v) =>
        v.toString(16),
      ).join("-");
      actions.set(key, request);
    }
    try {
      const result = await api("planet/action", { ...input, request });
      if (!signedIn || session !== authEpoch) return null;
      actions.delete(key);
      notify(
        tr(
          result.pending_refunds
            ? "Возврат ожидает получения в разделе Колонии"
            : "Действие выполнено",
        ),
        result.pending_refunds ? "warning" : "success",
      );
      await refresh(true);
      await update();
      return result;
    } catch (e) {
      notify(e.message, "error");
      return null;
    }
  }
  function confirm(title, message, input, done) {
    UI.confirm(title, message, async () => {
      const result = await send(input);
      if (result) await done?.(result);
      return !!result;
    });
  }
  function close(returnToSystem = true) {
    if (!current) return;
    const old = current;
    current = null;
    epoch++;
    old.resize?.disconnect();
    if (returnToSystem)
      window.dispatchEvent(
        new CustomEvent("returntoplanetsystem", { detail: old.data.star }),
      );
    old.host.classList.add("planet-leaving");
    setTimeout(() => {
      old.host.remove();
      if (!current || current.root !== old.root)
        old.root.classList.remove("planet-open");
      window.dispatchEvent(
        new CustomEvent("moduleresize", { detail: "galaxy" }),
      );
    }, 160);
  }
  function hex(s) {
    return Array.from({ length: 6 }, (_, i) => {
      const a = (Math.PI / 3) * i;
      return { x: s.center.x + Math.sin(a), y: s.center.y + Math.cos(a) };
    });
  }
  function color(s, claim) {
    if (claim) {
      if (claim.status === "funding") return "#eac85e";
      if (claim.status === "building") return "#eea45f";
      if (claim.owner_user === gameData.state.user.id) return "#7ad7a7";
      if (claim.owner_company === current.state.company) return "#baa0ef";
      return "#e09091";
    }
    return s.land < current.state.settings.min_land || !current.state.habitable
      ? "#607588"
      : "#b9cad0";
  }
  function draw() {
    const c = current;
    if (!c) return;
    const { canvas, g, camera } = c;
    const w = canvas.clientWidth,
      h = canvas.clientHeight;
    if (!w || !h) return;
    const ratio = devicePixelRatio || 1;
    canvas.width = Math.round(w * ratio);
    canvas.height = Math.round(h * ratio);
    const ctx = canvas.getContext("2d");
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.fillStyle = "#0d1923";
    ctx.fillRect(0, 0, w, h);
    const base = Math.min((w - 30) / g.width, (h - 30) / g.height);
    c.scale = base * camera.zoom;
    c.ox = w / 2 - (g.width * c.scale) / 2 + camera.x;
    c.oy = h / 2 - (g.height * c.scale) / 2 + camera.y;
    ctx.translate(c.ox, c.oy);
    ctx.scale(c.scale, c.scale);
    // One coherent geographic raster, clipped to the exact union of sector polygons.
    ctx.beginPath();
    for (const s of g.sectors) {
      const points = hex(s);
      ctx.moveTo(points[0].x, points[0].y);
      for (const p of points.slice(1)) ctx.lineTo(p.x, p.y);
      ctx.closePath();
    }
    ctx.save();
    ctx.clip();
    ctx.imageSmoothingEnabled = true;
    ctx.drawImage(c.terrain, 0, 0, g.width, g.height);
    ctx.strokeStyle = "#72bccb";
    ctx.lineWidth = 0.04;
    ctx.lineCap = "round";
    for (const river of g.rivers) {
      ctx.beginPath();
      river.forEach((p, i) =>
        i ? ctx.lineTo(p.x, p.y) : ctx.moveTo(p.x, p.y),
      );
      ctx.stroke();
    }
    ctx.restore();
    const claims = new Map(c.state.claims.map((cl) => [cl.sector, cl]));
    for (const s of g.sectors) {
      const points = hex(s),
        claim = claims.get(s.number);
      ctx.beginPath();
      ctx.moveTo(points[0].x, points[0].y);
      for (const p of points.slice(1)) ctx.lineTo(p.x, p.y);
      ctx.closePath();
      const line = color(s, claim);
      ctx.strokeStyle = line;
      ctx.lineWidth = (s.number === selected ? 2.5 : 1) / c.scale;
      ctx.globalAlpha = claim?.status
        ? 0.16
        : s.number === selected
          ? 0.18
          : 0.03;
      ctx.fillStyle = line;
      ctx.fill();
      ctx.globalAlpha = 1;
      ctx.stroke();
      if (camera.zoom > 1.4 || g.sectors.length <= 100) {
        ctx.fillStyle = "#e5eef1";
        ctx.font = `${10 / c.scale}px monospace`;
        ctx.textAlign = "center";
        ctx.fillText(s.number, s.center.x, s.center.y);
      }
    }
  }
  function terrain(g, kind) {
    const canvas = document.createElement("canvas");
    canvas.width = (g.grid_w - 1) * 4 + 1;
    canvas.height = (g.grid_h - 1) * 4 + 1;
    const ctx = canvas.getContext("2d"),
      image = ctx.createImageData(canvas.width, canvas.height);
    for (let y = 0; y < canvas.height; y++)
      for (let x = 0; x < canvas.width; x++) {
        const gx = x / 4,
          gy = y / 4,
          ix = Math.floor(gx),
          iy = Math.floor(gy),
          fx = gx - ix,
          fy = gy - iy;
        const ids = [
          iy * g.grid_w + ix,
          iy * g.grid_w + Math.min(ix + 1, g.grid_w - 1),
          Math.min(iy + 1, g.grid_h - 1) * g.grid_w + ix,
          Math.min(iy + 1, g.grid_h - 1) * g.grid_w +
            Math.min(ix + 1, g.grid_w - 1),
        ];
        const weights = [
          (1 - fx) * (1 - fy),
          fx * (1 - fy),
          (1 - fx) * fy,
          fx * fy,
        ];
        let h = 0,
          fresh = 0;
        ids.forEach((id, i) => {
          h += g.heights[id] * weights[i];
          if (g.water[id] >= 2) fresh += weights[i];
        });
        const sea =
            g.sea_level ??
            (kind === "Садовый мир" || kind === "Ледяная" ? 460 : -1),
          water = h < sea ? 1 : fresh > 0.5 ? 2 : 0;
        let rgb;
        if (water && kind === "Ледяная") rgb = [156, 192, 210];
        else if (water === 1)
          rgb = [22, 57 + Math.min(25, h / 20), 78 + Math.min(30, h / 15)];
        else if (water === 2) rgb = [48, 111, 133];
        else if (h < 485 && sea > 0) rgb = [151, 142, 102];
        else if (h > 700) rgb = [151, 157, 147];
        else if (kind === "Вулканическая") rgb = [105 + h / 20, 65, 50];
        else if (kind === "Каменистая")
          rgb = [75 + h / 12, 77 + h / 13, 80 + h / 15];
        else
          rgb = [61 + (h - 460) / 9, 91 + (h - 460) / 10, 65 + (h - 460) / 12];
        const i = (y * canvas.width + x) * 4;
        for (let c = 0; c < 3; c++) image.data[i + c] = rgb[c];
        image.data[i + 3] = 255;
      }
    ctx.putImageData(image, 0, 0);
    return canvas;
  }
  function controls(c) {
    let pointers = new Map(),
      dragged = false,
      down = null,
      pinch = 0;
    const bounds = () => c.canvas.getBoundingClientRect();
    const point = (e) => {
      const r = bounds();
      return { x: e.clientX - r.left, y: e.clientY - r.top };
    };
    const constrain = () => {
      const w = c.canvas.clientWidth,
        h = c.canvas.clientHeight;
      c.camera.x = Math.max(
        (-w * c.camera.zoom) / 2,
        Math.min((w * c.camera.zoom) / 2, c.camera.x),
      );
      c.camera.y = Math.max(
        (-h * c.camera.zoom) / 2,
        Math.min((h * c.camera.zoom) / 2, c.camera.y),
      );
    };
    const zoom = (factor, x, y) => {
      const old = c.camera.zoom;
      c.camera.zoom = Math.max(0.7, Math.min(8, old * factor));
      const ratio = c.camera.zoom / old;
      c.camera.x =
        (c.camera.x + c.canvas.clientWidth / 2 - x) * ratio -
        (c.canvas.clientWidth / 2 - x);
      c.camera.y =
        (c.camera.y + c.canvas.clientHeight / 2 - y) * ratio -
        (c.canvas.clientHeight / 2 - y);
      constrain();
      draw();
    };
    c.canvas.onwheel = (e) => {
      e.preventDefault();
      const p = point(e);
      zoom(Math.exp(-e.deltaY * 0.0015), p.x, p.y);
    };
    c.canvas.onpointerdown = (e) => {
      if (e.button !== 0) return;
      c.canvas.setPointerCapture(e.pointerId);
      pointers.set(e.pointerId, point(e));
      down = point(e);
      dragged = false;
      if (pointers.size === 2) {
        const [a, b] = [...pointers.values()];
        pinch = Math.hypot(a.x - b.x, a.y - b.y);
      }
    };
    c.canvas.onpointermove = (e) => {
      if (!pointers.has(e.pointerId)) return;
      const old = pointers.get(e.pointerId),
        p = point(e);
      pointers.set(e.pointerId, p);
      if (pointers.size === 2) {
        const [a, b] = [...pointers.values()],
          distance = Math.hypot(a.x - b.x, a.y - b.y);
        if (pinch) zoom(distance / pinch, (a.x + b.x) / 2, (a.y + b.y) / 2);
        pinch = distance;
        dragged = true;
        return;
      }
      if (down && Math.hypot(p.x - down.x, p.y - down.y) > 4) dragged = true;
      if (dragged) {
        c.camera.x += p.x - old.x;
        c.camera.y += p.y - old.y;
        constrain();
        draw();
      }
    };
    const finish = (e) => {
      if (!pointers.has(e.pointerId)) return;
      const p = point(e);
      pointers.delete(e.pointerId);
      if (!dragged && down) {
        const x = (p.x - c.ox) / c.scale,
          y = (p.y - c.oy) / c.scale;
        const sector = c.g.sectors.find(
          (s) =>
            Math.abs(x - s.center.x) <= 0.867 &&
            Math.abs(y - s.center.y) <=
              1 - Math.abs(x - s.center.x) / Math.sqrt(3),
        );
        if (sector) {
          selected = sector.number;
          card();
          draw();
        }
      }
      if (!pointers.size) {
        down = null;
        pinch = 0;
      }
    };
    c.canvas.onpointerup = finish;
    c.canvas.onpointercancel = () => {
      pointers.clear();
      down = null;
      dragged = true;
    };
  }
  function card() {
    const c = current;
    if (!c) return;
    const s = c.g.sectors.find((s) => s.number === selected) || c.g.sectors[0],
      root = c.card;
    root.replaceChildren(
      n("p", c.data.name, "eyebrow"),
      n("h3", tr("Сектор") + " " + s.number),
    );
    UI.row(root, "Тип", tr(s.type));
    UI.row(
      root,
      "Площадь",
      Math.round(s.area).toLocaleString(I18n.language) + " km²",
    );
    UI.row(
      root,
      "Суша",
      s.land +
        "% · " +
        Math.round((s.area * s.land) / 100).toLocaleString(I18n.language) +
        " km²",
    );
    UI.row(
      root,
      "Вода",
      100 -
        s.land +
        "% · " +
        Math.round((s.area * (100 - s.land)) / 100).toLocaleString(
          I18n.language,
        ) +
        " km²",
    );
    UI.row(root, "Температура", c.g.temperature + " °C");
    root.append(
      n(
        "small",
        tr(
          "Средняя равновесная оценка; локальный климат пока не моделируется.",
        ),
        "muted",
      ),
    );
    UI.row(root, "Природные условия", tr(s.biome));
    root.append(n("h4", tr("Предварительные ресурсы")));
    for (const [id, estimate] of Object.entries(s.estimates))
      UI.row(root, asset(id), tr(estimate));
    root.append(
      n(
        "small",
        tr("Точные объёмы требуют будущей геологической разведки."),
        "muted",
      ),
    );
    const claim = c.state.claims.find((p) => p.sector === s.number);
    UI.row(root, "Владелец", claim?.owner || tr("Свободная территория"));
    UI.row(
      root,
      "Статус",
      tr(claim ? statusName(claim.status) : "Свободная территория"),
    );
    if (claim) {
      root.append(btn("Проект колонизации", () => project(claim.id)));
      return;
    }
    let reason = !c.state.habitable
      ? "Планета непригодна для колонизации"
      : s.land < c.state.settings.min_land
        ? "Недостаточная доля суши"
        : c.state.level < c.state.settings.min_level
          ? "Для колонизации недостаточно стажа"
          : "";
    root.append(
      n(
        "p",
        reason
          ? tr(reason) +
              (reason.includes("стажа")
                ? " · " + c.state.settings.min_level
                : reason.includes("суши")
                  ? " · " + c.state.settings.min_land + "%"
                  : "")
          : tr("Сектор доступен для колонизации"),
        reason ? "muted" : "signal",
      ),
    );
    for (const corporate of [false, true]) {
      const blocked =
        reason ||
        (corporate
          ? !c.state.can_corporate
            ? "Недостаточно корпоративных полномочий"
            : ""
          : c.state.personal_count >= c.state.settings.personal_limit
            ? "Достигнут лимит личных секторов"
            : "");
      root.append(
        btn(
          corporate ? "Корпоративный проект" : "Личный проект",
          () =>
            confirm(
              "Создание колонии",
              c.data.name +
                " · " +
                tr("Сектор") +
                " " +
                s.number +
                " · " +
                tr(corporate ? "Корпоративный проект" : "Личный проект") +
                "\n" +
                Object.entries(c.state.settings.cost)
                  .map(([id, q]) => asset(id) + ": " + q)
                  .join(" · ") +
                "\n" +
                tr("Срок финансирования, дней") +
                ": " +
                c.state.settings.funding_days +
                " · " +
                tr("Длительность строительства, импульсов") +
                ": " +
                c.state.settings.build_ticks +
                " · " +
                tr("Возврат при отмене, %") +
                ": " +
                c.state.settings.refund_percent,
              {
                action: "create",
                surface: c.data.id,
                sector: s.number,
                corporate,
              },
              (r) => project(r.project),
            ),
          !!blocked,
          blocked,
        ),
      );
    }
  }
  async function update() {
    if (!current || polling) return;
    const c = current;
    polling = true;
    try {
      const state = await api("planet/state?surface=" + c.data.id);
      if (current !== c) return;
      c.state = state;
      if (!c.card.contains(document.activeElement)) card();
      draw();
    } catch (e) {
      if (current === c) notify(e.message, "error");
    } finally {
      polling = false;
    }
  }
  async function open(star, planet) {
    const request = ++epoch;
    Modules.open("galaxy");
    const data = await api(`planet/surface?star=${star}&planet=${planet}`);
    if (request !== epoch || !signedIn) return;
    const state = await api("planet/state?surface=" + data.id);
    if (request !== epoch || !signedIn) return;
    const root = document.querySelector("#window-galaxy .module-content");
    current?.resize?.disconnect();
    root.querySelector(".planet-ui")?.remove();
    root.classList.add("planet-open");
    const host = n("section", undefined, "planet-ui"),
      tools = n("div", undefined, "planet-toolbar"),
      layout = n("div", undefined, "planet-workspace"),
      wrap = n("div", undefined, "planet-canvas-wrap"),
      canvas = n("canvas"),
      side = n("aside", undefined, "planet-sector-card");
    canvas.id = "planet-map";
    canvas.setAttribute("aria-label", tr("Поверхность планеты"));
    canvas.tabIndex = 0;
    const c = {
      root,
      host,
      canvas,
      card: side,
      data,
      state,
      g: data.geography,
      camera: { zoom: 1, x: 0, y: 0 },
      terrain: terrain(data.geography, data.kind),
    };
    current = c;
    selected = 1;
    tools.append(
      btn("К карте системы", () => close()),
      n("strong", data.name),
      btn("Вместить карту", () => {
        c.camera = { zoom: 1, x: 0, y: 0 };
        draw();
      }),
      btn("Колонии", () => manager()),
    );
    const legend = n("div", undefined, "planet-legend");
    for (const [label, color] of [
      ["Свободная территория", "#b9cad0"],
      ["Недоступно", "#607588"],
      ["Финансирование", "#eac85e"],
      ["Строительство", "#eea45f"],
      ["Личная колония", "#7ad7a7"],
      ["Корпоративная колония", "#baa0ef"],
      ["Чужая колония", "#e09091"],
    ]) {
      const item = n("span", tr(label));
      item.style.borderLeft = "3px solid " + color;
      legend.append(item);
    }
    wrap.append(canvas);
    layout.append(wrap, side);
    host.append(tools, layout, legend);
    root.append(host);
    controls(c);
    c.resize = new ResizeObserver(draw);
    c.resize.observe(wrap);
    card();
    requestAnimationFrame(draw);
  }
  const projectRoots = new Map(),
    contributionCache = new Map();
  async function project(
    id,
    before = 0,
    background = false,
    contributionsBefore = 0,
  ) {
    const epochAt = epoch,
      data = await api(
        "planet/project?id=" +
          id +
          (before ? "&before=" + before : "") +
          (contributionsBefore
            ? "&contributions_before=" + contributionsBefore
            : ""),
      );
    if (
      !signedIn ||
      epochAt !== epoch ||
      (background && !Modules.isOpen("colony-project-" + id))
    )
      return;
    const p = data.project,
      key = "colony-project-" + id,
      root = Modules.create(key, "Проект колонизации", 760, 750);
    projectRoots.set(id, root);
    if (!data.can_contribute && gameData.state.user.role !== "admin") {
      const cached = contributionCache.get(id);
      if (cached) {
        for (const [key, item] of cached.items)
          if (item.actor !== gameData.state.user.id) cached.items.delete(key);
        if (cached.events)
          for (const [key, item] of cached.events)
            if (item.actor !== gameData.state.user.id)
              cached.events.delete(key);
      }
    }
    if (contributionsBefore) {
      const cache = contributionCache.get(id) || { items: new Map() };
      data.contributions.forEach((item) => cache.items.set(item.id, item));
      cache.more = data.contributions.length === 50;
      contributionCache.set(id, cache);
      const host = root.querySelector(".colony-contributions");
      if (host) {
        host.replaceChildren();
        appendContributions(
          host,
          [...cache.items.values()].sort((a, b) => b.id - a.id),
          p,
          id,
          cache.more,
        );
      }
      return;
    }
    if (before) {
      const history = root.querySelector(".colony-history");
      appendEvents(history, data.events, id, true);
      return;
    }
    if (
      root.contains(document.activeElement) &&
      document.activeElement.matches("input,select")
    )
      return;
    const scroll = root.scrollTop;
    root.replaceChildren(
      n("h3", data.name + " · " + tr("Сектор") + " " + p.sector),
    );
    UI.row(root, "Владелец", data.owner);
    UI.row(root, "Статус", tr(statusName(p.status)));
    UI.row(root, "Инициатор", data.initiator_name || String(p.initiator));
    if (p.status === "funding") {
      const remaining = Math.max(
        0,
        (new Date(p.expires_at) - new Date(data.server_time)) / 3600000,
      );
      root.append(
        n(
          "p",
          tr("Осталось на финансирование, часов") + ": " + remaining.toFixed(1),
        ),
        n(
          "p",
          tr(
            "Материалы передаются с личного склада станции при стыковке в системе планеты. Кредиты списываются с личного счёта.",
          ),
          "muted",
        ),
      );
    }
    if (p.status === "funding")
      root.append(
        n(
          "p",
          tr(
            "Полное финансирование автоматически начинает строительство. После этого возврат вкладов и отмена недоступны.",
          ),
          "muted",
        ),
      );
    const rows = Object.entries(p.required).map(([id, required]) => [
      asset(id),
      String(required),
      p.received ? String(p.received[id] || 0) : "—",
      p.received ? String(required - (p.received[id] || 0)) : "—",
    ]);
    root.append(
      UI.table(["Материал", "Требуется", "Внесено", "Осталось"], rows),
    );
    if (p.status === "building" || p.status === "complete")
      root.append(
        UI.bar(p.progress, p.build_ticks, "Импульсы строительства"),
        n("p", tr("Осталось импульсов") + ": " + (p.build_ticks - p.progress)),
      );
    if (p.status === "funding" && data.can_contribute) {
      const form = n("form"),
        resource = n("select");
      for (const [id, q] of Object.entries(p.required)) {
        if (q <= (p.received[id] || 0)) continue;
        const option = n("option", asset(id));
        option.value = id;
        resource.append(option);
      }
      const label = n("label");
      label.append(n("span", tr("Материал")), resource);
      form.append(label);
      const amount = UI.field(
        form,
        "Количество",
        "colony-amount-" + id,
        1,
        "number",
      );
      amount.min = 1;
      amount.step = 1;
      form.append(
        btn("Внести вклад", () =>
          confirm(
            "Подтверждение взноса",
            asset(resource.value) + " × " + amount.value,
            {
              action: "contribute",
              id,
              resource: resource.value,
              amount: Number(amount.value),
            },
            () => project(id),
          ),
        ),
      );
      form.onsubmit = (e) => e.preventDefault();
      root.append(form);
    }
    const tools = n("div", undefined, "inline-controls");
    if (p.status === "funding" && data.can_manage)
      tools.append(
        btn("Отменить проект", () =>
          confirm(
            "Отмена проекта",
            tr("При отмене возвращается") +
              " " +
              p.refund_percent +
              "% · " +
              tr(
                "Ядро возвращается полностью. Возвраты направляются вкладчикам на исходные личные склады и счета.",
              ),
            { action: "cancel", id },
            () => project(id),
          ),
        ),
      );
    if (p.status === "funding" && gameData.state.user.role === "admin")
      tools.append(
        btn("Пополнить ресурсы", () =>
          confirm(
            "Административное пополнение",
            tr("Выдать недостающие материалы и начать обычное строительство?"),
            { action: "admin_fill", id },
            () => project(id),
          ),
        ),
      );
    if (p.status === "complete")
      tools.append(
        btn(
          "Высадиться",
          () => send({ action: "land", id }).then((r) => r && landing(r, id)),
          !!data.land_reason,
          data.land_reason,
        ),
      );
    tools.append(
      btn("На поверхности", () =>
        open(data.star, data.planet).then(() => {
          selected = p.sector;
          card();
          draw();
        }),
      ),
    );
    root.append(tools, n("h4", tr("История взносов")));
    const contributionsRoot = n("div", undefined, "colony-contributions");
    const cache = contributionCache.get(id) || {
      items: new Map(),
      more: data.contributions.length === 50,
    };
    data.contributions.forEach((item) => cache.items.set(item.id, item));
    contributionCache.set(id, cache);
    appendContributions(
      contributionsRoot,
      [...cache.items.values()].sort((a, b) => b.id - a.id),
      p,
      id,
      cache.more,
    );
    root.append(contributionsRoot);
    const history = n("div", undefined, "colony-history");
    appendEvents(history, data.events, id);
    root.append(history);
    root.scrollTop = scroll;
    if (!background) Modules.open(key);
  }
  function appendContributions(root, items, p, id, more) {
    for (const item of items) {
      const returned =
        item.returned || ["cancelled", "expired"].includes(p.status);
      const row = n("div", undefined, "colony-contribution");
      row.append(
        n(
          "span",
          item.username +
            " · " +
            asset(item.resource) +
            " × " +
            item.amount +
            " · " +
            new Date(item.created_at).toLocaleString(I18n.language) +
            (item.admin_grant
              ? " · " + tr("Выдача администратора")
              : returned
                ? " · " + tr("Возвращён")
                : ""),
        ),
      );
      if (
        p.status === "funding" &&
        item.actor === gameData.state.user.id &&
        !item.admin_grant &&
        !returned
      )
        row.append(
          btn("Вернуть вклад", () =>
            confirm(
              "Возврат вклада",
              asset(item.resource) + " × " + item.amount + " · 100%",
              { action: "withdraw", id, contribution: item.id },
              () => project(id),
            ),
          ),
        );
      root.append(row);
    }
    if (more && items.length)
      root.append(
        btn("Ранее", () => project(id, 0, false, items[items.length - 1].id)),
      );
  }
  function appendEvents(root, events, id, older = false) {
    const cache = contributionCache.get(id) || {
      items: new Map(),
      more: false,
    };
    cache.events ||= new Map();
    if (cache.eventMore === undefined || older)
      cache.eventMore = events.length === 30;
    events.forEach((e) => cache.events.set(e.id, e));
    contributionCache.set(id, cache);
    const all = [...cache.events.values()].sort((a, b) => b.id - a.id);
    root.replaceChildren();
    for (const e of all)
      root.append(
        n(
          "p",
          new Date(e.created_at).toLocaleString(I18n.language) +
            " · " +
            tr(
              {
                create: "Проект создан",
                contribute: "Взнос",
                refund: "Возврат вклада",
                building: "Строительство",
                complete: "Колонизация завершена",
                cancelled: "Отменён",
                expired: "Просрочен",
                admin_fill: "Выдача администратора",
              }[e.kind] || e.kind,
            ) +
            " · " +
            (e.username || tr("Система")) +
            " · " +
            (e.data?.resource
              ? asset(e.data.resource) + " × " + e.data.amount
              : e.kind === "admin_fill"
                ? Object.entries(e.data || {})
                    .map(([id, q]) => asset(id) + " × " + q)
                    .join(" · ")
                : ""),
          "muted",
        ),
      );
    if (cache.eventMore && all.length) {
      const more = btn("Ранее", () => project(id, all[all.length - 1].id));
      more.classList.add("colony-more");
      root.append(more);
    }
  }
  function landing(data, id) {
    const root = Modules.create(
      "colony-location",
      "Планетарная локация",
      650,
      500,
    );
    root.replaceChildren(
      n("h2", data.planet + " · " + tr("Сектор") + " " + data.sector),
      n("p", tr("Высадка разрешена. Колония готова к освоению.")),
      n(
        "p",
        tr(
          "Движение персонажа и подробная поверхность появятся в следующей очереди.",
        ),
        "muted",
      ),
      btn("Вернуться в космос", async () => {
        if (await send({ action: "leave", id }))
          Modules.close("colony-location");
      }),
    );
    Modules.open("colony-location");
  }
  async function manager(background = false) {
    const session = authEpoch;
    const data = await api("planet/projects");
    if (
      !signedIn ||
      session !== authEpoch ||
      (background && !Modules.isOpen("colonies"))
    )
      return;
    const root = Modules.create("colonies", "Колонии", 850, 650);
    root.replaceChildren(n("h3", tr("Колонии и проекты")));
    for (const p of data.projects) {
      const card = n("section", undefined, "colony-list-item");
      card.append(
        n("b", p.name + " · " + tr("Сектор") + " " + p.sector),
        n("p", p.owner + " · " + tr(statusName(p.status))),
        btn("Открыть проект", () => project(p.id)),
        btn("Поверхность планеты", () => open(p.star, p.planet)),
      );
      root.append(card);
    }
    if (!data.projects.length) UI.empty(root, "Планетарных проектов пока нет");
    for (const refund of data.refunds)
      root.append(
        btn(
          tr("Получить возврат") +
            " · " +
            asset(refund.resource) +
            " × " +
            refund.amount,
          () =>
            send({ action: "claim_refund", id: refund.id }).then(() =>
              manager(),
            ),
        ),
      );
    if (!background) Modules.open("colonies");
  }
  async function settings(root) {
    const session = authEpoch;
    const data = await api("admin/planet/settings"),
      form = n("form"),
      fields = {};
    if (
      !signedIn ||
      session !== authEpoch ||
      gameData.state.user.role !== "admin"
    )
      return false;
    form.onsubmit = (e) => e.preventDefault();
    form.append(
      n("h3", tr("Планетарная колонизация")),
      n(
        "p",
        tr(
          "Изменения применяются к новым проектам и ещё не созданным поверхностям.",
        ),
        "muted",
      ),
    );
    for (const [key, label] of [
      ["min_level", "Минимальный стаж колонизации"],
      ["personal_limit", "Лимит личных секторов"],
      ["min_land", "Минимальная доля суши, %"],
      ["funding_days", "Срок финансирования, дней"],
      ["build_ticks", "Длительность строительства, импульсов"],
      ["refund_percent", "Возврат при отмене, %"],
    ])
      fields[key] = UI.field(
        form,
        label,
        "planet-setting-" + key,
        data[key],
        "number",
      );
    const counts = data.counts.map((value, i) =>
      UI.field(
        form,
        ["Маленькая", "Средняя", "Большая", "Гигантская"][i],
        "planet-count-" + i,
        value,
        "number",
      ),
    );
    const costs = {};
    for (const [id, value] of Object.entries(data.cost))
      costs[id] = UI.field(
        form,
        asset(id),
        "planet-cost-" + id,
        value,
        "number",
      );
    form.append(
      btn("Сохранить настройки колонизации", async () => {
        try {
          await api("admin/planet/settings", {
            ...Object.fromEntries(
              Object.entries(fields).map(([k, v]) => [k, Number(v.value)]),
            ),
            counts: counts.map((v) => Number(v.value)),
            cost: Object.fromEntries(
              Object.entries(costs).map(([k, v]) => [k, Number(v.value)]),
            ),
          });
          notify(tr("Действие выполнено"));
        } catch (e) {
          notify(e.message, "error");
        }
      }),
    );
    root.append(form);
    return true;
  }
  window.PlanetUI = { open, project, manager, settings, dismiss: close };
  window.addEventListener("moduleresize", (e) => {
    if (e.detail === "galaxy") draw();
  });
  window.addEventListener("gamedata", () => {
    if (Modules.isOpen("colonies")) manager(true).catch(() => {});
    if (current && Modules.isOpen("galaxy")) update();
    for (const [id, root] of projectRoots) {
      if (
        Modules.isOpen("colony-project-" + id) &&
        !root.contains(document.activeElement)
      )
        project(id, 0, true).catch(() => {});
    }
  });
  window.addEventListener("languagechange", () => {
    if (current) {
      const { data } = current;
      open(data.star, data.planet);
    }
    for (const [id] of projectRoots) {
      if (Modules.isOpen("colony-project-" + id))
        project(id, 0, true).catch(() => {});
    }
  });
  window.addEventListener("signedout", () => {
    epoch++;
    close(false);
    actions.clear();
    projectRoots.clear();
    contributionCache.clear();
    document
      .querySelectorAll('[id^="window-colony"],#window-colonies')
      .forEach((w) => {
        w.close();
        w.querySelector(".module-content")?.replaceChildren();
      });
  });
})();
