"use strict";
(() => {
  const objects = new Map();
  const tr = (s) => I18n.t(s),
    node = (tag, text, cls) => {
      const e = document.createElement(tag);
      if (text !== undefined) e.textContent = text;
      if (cls) e.className = cls;
      return e;
    };
  const context = node("div", undefined, "context-menu");
  context.hidden = true;
  context.setAttribute("role", "menu");
  document.body.append(context);
  let anchor;
  window.UI = {
    node,
    menu(items, x, y) {
      context.replaceChildren();
      anchor = document.activeElement;
      for (const item of items) {
        if (item.heading) {
          context.append(node("p", tr(item.heading), "context-heading"));
          continue;
        }
        const b = node("button", tr(item.label));
        b.type = "button";
        b.setAttribute("role", "menuitem");
        b.disabled = !!item.disabled;
        if (item.reason) b.dataset.help = item.reason;
        b.onclick = () => {
          context.hidden = true;
          item.action?.();
        };
        context.append(b);
      }
      context.hidden = false;
      context.style.left =
        Math.max(8, Math.min(x, innerWidth - context.offsetWidth - 8)) + "px";
      context.style.top =
        Math.max(8, Math.min(y, innerHeight - context.offsetHeight - 8)) + "px";
      context.querySelector("button:not(:disabled)")?.focus();
    },
    resource(id) {
      const r = gameData.expedition?.resources.find((r) => r.id === id);
      if (!r) return;
      const root = Modules.create(
        "resource-" + id,
        "Информация о ресурсе",
        500,
        500,
      );
      root.replaceChildren(resourceCard(r));
      Modules.open("resource-" + id);
    },
    object(site, star) {
      const key = "object-" + star.id + "-" + site.id,
        root = Modules.create(key, "Информация об объекте", 600, 650);
      objects.set(key, { root, site, star });
      renderObject(root, site, star);
      Modules.open(key);
    },
    confirm(title, text, run) {
      const root = Modules.create("confirm", title, 470, 340);
      root.replaceChildren(node("p", text));
      const yes = node("button", tr("Подтвердить")),
        no = node("button", tr("Отмена"));
      yes.onclick = async () => {
        yes.disabled = true;
        try {
          if (await run()) Modules.close("confirm");
        } finally {
          yes.disabled = false;
        }
      };
      no.onclick = () => Modules.close("confirm");
      root.append(yes, no);
      Modules.open("confirm");
    },
  };
  function renderObject(container, site, star) {
    const scroll = container.scrollTop;
    let model = container.querySelector(":scope > .object-hologram"),
      root = container.querySelector(":scope > .object-information");
    if (!model) {
      container.replaceChildren();
      model = UI.objectHologram(site, star);
      root = node("div", undefined, "object-information");
      container.append(model, root);
    }
    const asset = UI.objectModel(site, star);
    if (model.getAttribute("src") !== asset) model.src = asset;
    root.replaceChildren(node("h2", site.name), UI.objectTag(site.kind));
    if (site.details) {
      const p = site.details;
      const params = node("p");
      for (const [value, message] of [
        [
          p.mass + " M⊕",
          "M⊕ — масса Земли. Значение 1 означает массу, равную земной.",
        ],
        [
          p.gravity + " g",
          "g — земная сила тяжести. 1 g примерно равно 9,81 м/с².",
        ],
        [
          p.temperature + " K",
          "K — кельвины. Для приблизительного перевода в °C вычтите 273. Температура равновесная, без парникового эффекта.",
        ],
        [
          p.au + " AU",
          "AU — астрономическая единица, среднее расстояние от Земли до Солнца: около 149,6 млн км.",
        ],
      ]) {
        const span = node("span", value, "help");
        span.dataset.help = message;
        span.tabIndex = 0;
        params.append(span, document.createTextNode(" · "));
      }
      root.append(params, node("p", tr(p.atmosphere)));
    }
    if (site.shared_quota) {
      const p = node(
          "p",
          tr("Общий запас") +
            `: ${site.shared_quota.remaining} / ${site.shared_quota.capacity}`,
        ),
        time = node("span");
      time.dataset.resetAt = site.shared_quota.reset_at;
      p.append(document.createTextNode(" · "), time);
      root.append(p);
    }
    root.append(
      node(
        "h3",
        tr(site.kind === "Орбитальная станция" ? "Услуги порта" : "Ресурсы"),
      ),
    );
    if (site.resources?.length)
      for (const id of site.resources) {
        const r = gameData.expedition.resources.find((r) => r.id === id),
          p = node("p", tr(r?.name || id));
        if (site.quotas?.[id])
          p.append(
            document.createTextNode(
              ` · ${site.quotas[id].remaining} / ${site.quotas[id].capacity}`,
            ),
          );
        root.append(p);
      }
    else
      root.append(
        node(
          "p",
          tr(
            site.kind === "Орбитальная станция"
              ? "У каждого порта свой ассортимент и склад. Для торговли и получения груза нужна стыковка с выбранной станцией."
              : "Месторождения откроются после разведки на месте.",
          ),
        ),
      );
    if (site.quality) {
      UI.row(root, "Качество", site.quality);
      UI.row(root, "Концентрация", site.concentration + "%");
      const help = node(
        "p",
        tr(
          "Качество Q1–Q5 и концентрация влияют на производительность специализированного добытчика.",
        ),
        "muted",
      );
      root.append(help);
    }
    if (site.kind === "Орбитальная станция") {
      const port = gameData.workbench?.stations.find(
        (p) => p.star === star.id && p.object_id === site.id,
      );
      if (port) {
        UI.row(root, "Специализация", tr(port.specialization));
        UI.row(
          root,
          "Владелец",
          port.owner_name
            ? port.owner_kind === "founder"
              ? tr(port.owner_name)
              : port.owner_name
            : tr("Нейтральная станция"),
        );
        UI.row(root, "Тариф", port.fee_percent + "%");
        UI.row(root, "Репутация", port.reputation);
        if (port.shipyard)
          root.append(
            UI.button("Открыть верфь", () =>
              Game.openShipyards(star.id, site.id),
            ),
          );
        root.append(
          node(
            "p",
            tr(
              "Репутация 20 отменяет тариф корпорации-основателя 02. Тариф округляется вверх до целого CR.",
            ),
            "muted",
          ),
        );
      }
    }
    const guide = node("button", tr("Руководство пилота"));
    guide.onclick = () =>
      Game.openGuide(
        site.details
          ? {
              Ледяная: "icy",
              "Садовый мир": "garden",
              "Газовый гигант": "gas",
              Вулканическая: "volcanic",
            }[site.kind] || "rocky"
          : site.kind === "Орбитальная станция"
            ? "station"
            : site.kind === "Ледяное поле"
              ? "ice"
              : "belt",
      );
    root.append(guide);
    const saved = gameData.workbench?.notes.find(
      (item) => item.star === star.id && item.object_id === site.id,
    );
    const note = UI.field(
      root,
      "Личная заметка",
      "object-note-" + star.id + "-" + site.id,
      saved?.note || "",
    );
    root.append(
      UI.button("Сохранить заметку", () =>
        Game.command({
          action: "note",
          star: star.id,
          target: site.id,
          note: note.value,
          bookmark: false,
        }),
      ),
    );
    container.scrollTop = scroll;
  }
  window.addEventListener("gamedata", async () => {
    const epoch = authEpoch,
      cache = new Map();
    for (const [key, entry] of objects) {
      if (window.MapData?.stars[entry.star.id - 1]?.deleted) {
        Modules.close(key);
        objects.delete(key);
        continue;
      }
      if (
        !Modules.isOpen(key) ||
        (entry.root.contains(document.activeElement) &&
          document.activeElement.matches("input,textarea"))
      )
        continue;
      if (!cache.has(entry.star.id))
        cache.set(entry.star.id, api("system?id=" + entry.star.id));
      try {
        const data = await cache.get(entry.star.id);
        if (epoch !== authEpoch) return;
        const site = data.sites.find((s) => s.id === entry.site.id);
        if (site) renderObject(entry.root, site, data.star);
      } catch {}
    }
  });
  window.addEventListener("signedout", () => {
    objects.clear();
    context.hidden = true;
    document.getElementById("tooltip").hidden = true;
  });
  function resourceCard(r) {
    const box = node("div", undefined, "resource-card");
    box.append(
      node("span", r.code, "resource-code"),
      node("h3", tr(r.name)),
      node("p", tr(r.description)),
      node("p", tr(r.sources), "muted"),
      node("p", tr(r.origin), "muted"),
    );
    if (r.id === "agni")
      box.append(
        node("p", tr("1 ед. водорода = до 5 единиц топлива"), "resource-fuel"),
      );
    const tags = node("div", undefined, "resource-tags");
    for (const text of [r.rarity, ...(r.uses || [])])
      tags.append(node("span", tr(text)));
    box.append(tags);
    return box;
  }
  const tooltip = document.getElementById("tooltip");
  function rich(target, x, y) {
    const r = gameData.expedition?.resources.find(
      (r) => r.id === target.dataset.resource,
    );
    if (!r) return;
    tooltip.replaceChildren(resourceCard(r));
    tooltip.hidden = false;
    tooltip.style.left = Math.max(8, Math.min(x + 12, innerWidth - 350)) + "px";
    tooltip.style.top =
      Math.max(8, Math.min(y + 12, innerHeight - tooltip.offsetHeight - 8)) +
      "px";
  }
  document.addEventListener("pointerover", (e) => {
    const target = e.target.closest("[data-resource]");
    if (target) rich(target, e.clientX, e.clientY);
  });
  document.addEventListener("pointerout", (e) => {
    if (e.target.closest("[data-resource]")) tooltip.hidden = true;
  });
  document.addEventListener("focusin", (e) => {
    const target = e.target.closest("[data-resource]");
    if (target) {
      const r = target.getBoundingClientRect();
      rich(target, r.x, r.bottom);
    }
  });
  document.addEventListener("pointerdown", (e) => {
    if (!context.contains(e.target)) context.hidden = true;
  });
  context.onkeydown = (e) => {
    if (e.key === "Escape") {
      context.hidden = true;
      anchor?.focus();
    }
    if (["ArrowDown", "ArrowUp"].includes(e.key)) {
      e.preventDefault();
      const buttons = [...context.querySelectorAll("button:not(:disabled)")],
        i = buttons.indexOf(document.activeElement);
      buttons[
        (i + (e.key === "ArrowDown" ? 1 : -1) + buttons.length) % buttons.length
      ]?.focus();
    }
  };
  window.Visuals = {
    object(ctx, x, y, r, site, scale = 1) {
      const kind = site.kind;
      if (kind === "Звезда" || kind === "Чёрная дыра") {
        ctx.save();
        ctx.fillStyle = kind === "Звезда" ? "#efbd68" : "#050710";
        ctx.shadowColor = kind === "Звезда" ? "#efad58" : "#a68bcf";
        ctx.shadowBlur = r * 0.35;
        ctx.beginPath();
        ctx.arc(x, y, r * 0.7, 0, Math.PI * 2);
        ctx.fill();
        ctx.strokeStyle = "#bba0e4";
        if (kind === "Чёрная дыра") {
          ctx.lineWidth = 5;
          ctx.beginPath();
          ctx.ellipse(x, y, r, r * 0.35, 0.3, 0, Math.PI * 2);
          ctx.stroke();
        }
        ctx.restore();
        return;
      }
      if (kind === "Орбитальная станция") {
        ctx.save();
        ctx.translate(x, y);
        ctx.rotate(0.4);
        ctx.strokeStyle = "#8ccfb8";
        ctx.lineWidth = Math.max(1, r * 0.12);
        ctx.strokeRect(-r, -r * 0.6, r * 2, r * 1.2);
        ctx.fillStyle = "#243f4d";
        ctx.fillRect(-r * 0.5, -r * 0.5, r, r);
        ctx.fillStyle = "#c5dadd";
        ctx.fillRect(-r * 1.4, -r * 0.1, r * 2.8, r * 0.2);
        ctx.restore();
        return;
      }
      if (kind === "Пояс астероидов" || kind === "Ледяное поле") {
        for (let i = 0; i < 5; i++) {
          const a = i * 2.4,
            px = x + Math.cos(a) * r * 0.65,
            py = y + Math.sin(a) * r * 0.6,
            size = r * (0.22 + i * 0.025);
          ctx.beginPath();
          for (let j = 0; j < 6; j++) {
            const theta = (j * Math.PI) / 3,
              rad = size * (j % 2 ? 0.75 : 1);
            const fx = px + Math.cos(theta) * rad,
              fy = py + Math.sin(theta) * rad;
            if (j === 0) ctx.moveTo(fx, fy);
            else ctx.lineTo(fx, fy);
          }
          ctx.closePath();
          ctx.fillStyle =
            kind === "Ледяное поле"
              ? ["#86cde8", "#c0ecfa"][i % 2]
              : ["#787c81", "#a29a8c", "#5d636c"][i % 3];
          ctx.fill();
          ctx.strokeStyle = kind === "Ледяное поле" ? "#d5f5ff" : "#c0b7a3";
          ctx.lineWidth = 1;
          ctx.stroke();
        }
        return;
      }
      const colors = {
        Ледяная: ["#b6edff", "#326bb1"],
        Каменистая: ["#b0b1b3", "#4b4f56"],
        "Садовый мир": ["#74ba86", "#225d7a"],
        Вулканическая: ["#ef9c4d", "#8a292b"],
        "Газовый гигант": ["#d1bb8b", "#72507e"],
      };
      const color = colors[kind] || ["#b8c4d0", "#485b70"];
      const gradient = ctx.createRadialGradient(
        x - r * 0.35,
        y - r * 0.4,
        r * 0.1,
        x,
        y,
        r,
      );
      gradient.addColorStop(0, color[0]);
      gradient.addColorStop(1, color[1]);
      ctx.fillStyle = gradient;
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      ctx.fill();
      ctx.save();
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      ctx.clip();
      if (kind === "Газовый гигант") {
        for (let i = -3; i <= 3; i++) {
          ctx.fillStyle = i % 2 ? "#d9bf9a66" : "#51406755";
          ctx.fillRect(x - r, y + i * r * 0.23, r * 2, r * 0.12);
        }
      } else if (kind === "Садовый мир") {
        ctx.fillStyle = "#8dbb6c";
        for (let i = 0; i < 6; i++) {
          ctx.beginPath();
          ctx.ellipse(
            x + Math.sin(i * 2) * r * 0.55,
            y + Math.cos(i * 1.7) * r * 0.6,
            r * 0.3,
            r * 0.14,
            i,
            0,
            Math.PI * 2,
          );
          ctx.fill();
        }
      } else if (kind === "Каменистая") {
        ctx.fillStyle = "#333b4455";
        for (let i = 0; i < 6; i++) {
          ctx.beginPath();
          ctx.arc(
            x + Math.sin(i * 2) * r * 0.6,
            y + Math.cos(i * 3) * r * 0.6,
            r * 0.12,
            0,
            Math.PI * 2,
          );
          ctx.fill();
        }
      }
      ctx.restore();
    },
    ship(ctx, x, y, size = 10, angle = 0) {
      ctx.save();
      ctx.translate(x, y);
      ctx.rotate(angle);
      ctx.fillStyle = "#9de5f2";
      ctx.strokeStyle = "#f4ffff";
      ctx.lineWidth = 1;
      ctx.shadowColor = "#63dcff";
      ctx.shadowBlur = 9;
      ctx.beginPath();
      ctx.moveTo(size, 0);
      ctx.lineTo(-size * 0.65, -size * 0.6);
      ctx.lineTo(-size * 0.35, 0);
      ctx.lineTo(-size * 0.65, size * 0.6);
      ctx.closePath();
      ctx.fill();
      ctx.stroke();
      ctx.restore();
    },
  };
})();
