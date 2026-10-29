"use strict";
(() => {
  const tr = (s) => I18n.t(s),
    n = UI.node;
  const folds = new Map();
  const saveFold = (d) => {
    folds.set(d.dataset.fold, d.open);
    if (d.dataset.foldStorage) {
      try {
        localStorage.setItem(d.dataset.foldStorage, JSON.stringify(d.open));
      } catch {}
    }
  };
  UI.rememberFolds = (root) =>
    root.querySelectorAll("details[data-fold]").forEach(saveFold);
  UI.fold = (key, opened = false, className, persistent = false) => {
    const details = n("details", undefined, className);
    details.dataset.fold = key;
    if (persistent) {
      details.dataset.foldStorage = `fold:${gameData.state.user.id}:${key}`;
      if (!folds.has(key)) {
        try {
          const saved = JSON.parse(
            localStorage.getItem(details.dataset.foldStorage),
          );
          if (typeof saved === "boolean") folds.set(key, saved);
        } catch {}
      }
    }
    details.open = folds.has(key) ? folds.get(key) : opened;
    details.addEventListener("toggle", () => {
      if (details.isConnected) saveFold(details);
    });
    return details;
  };
  window.addEventListener("signedout", () => folds.clear());
  UI.serverStatus = (info) => {
    const target = document.getElementById("server-status");
    target.dataset.help = "Сервер";
    target.tabIndex = 0;
    target.dataset.helpRaw =
      I18n.t("Сервер") +
      ": " +
      I18n.t(info.name || "Иггдрасиль") +
      "\n" +
      I18n.t("Игроки онлайн") +
      ": " +
      info.players +
      "\n" +
      I18n.t("Боты") +
      ": " +
      info.bots;
  };
  I18n.ready
    .then(() => fetch("/api/server-status"))
    .then((r) => (r.ok ? r.json() : Promise.reject()))
    .then(UI.serverStatus)
    .catch(() => {});
  UI.objectModel = (site, star) => {
    const types = {
      Ледяная: "icy",
      Каменистая: "rocky",
      "Садовый мир": "garden",
      "Газовый гигант": "gas",
      Пустынная: "desert",
      Лавовая: "lava",
      Вулканическая: "lava",
      "Орбитальная станция": "station",
      "Пояс астероидов": "asteroids",
      "Ледяное поле": "ice-field",
      "Чёрная дыра": "black-hole",
    };
    let type = types[site.kind] || "rocky";
    if (site.kind === "Звезда")
      type =
        "star-" +
        (["O", "B", "A", "F", "G", "K", "M"].includes(star?.class)
          ? star.class.toLowerCase()
          : "g");
    return "object-" + type + ".svg";
  };
  UI.objectHologram = (site, star) => {
    return UI.hologram(
      UI.objectModel(site, star),
      tr(site.kind),
      "object-hologram",
    );
  };
  UI.objectTag = (kind) => {
    const colors = {
      "Орбитальная станция": "#7cc8d2",
      "Садовый мир": "#8bcf9e",
      Ледяная: "#8ec6ee",
      Каменистая: "#b0b1b3",
      "Газовый гигант": "#d9b889",
      Пустынная: "#d7bb83",
      Лавовая: "#e58c72",
      Вулканическая: "#e58c72",
      "Пояс астероидов": "#bfb29a",
      "Ледяное поле": "#9fd7eb",
      Звезда: "#efbd68",
      "Чёрная дыра": "#b59ad4",
    };
    const tag = n("span", tr(kind), "object-kind-tag");
    tag.dataset.kind = kind;
    tag.style.color = colors[kind] || "#b4c4d0";
    return tag;
  };
  UI.shipName = (s) => {
    const name =
      s.spec?.name ||
      gameData.workbench?.ship_classes?.find((c) => c.id === s.class)?.name;
    return s.name === name ? tr(s.name) : s.name;
  };
  UI.paintFlag = (badge, company) => {
    badge.replaceChildren();
    badge.style.color = company.color;
    const flag = n("span", undefined, "flag-mark"),
      design = company.flag_design;
    if (design?.background)
      flag.style.background = `linear-gradient(135deg,${design.background} 35%,${design.stripe} 36%,${design.stripe} 48%,${design.background} 49%)`;
    flag.append(n("span", company.flag, "flag-symbol"));
    badge.append(flag, n("span", "[" + company.tag + "]"));
  };
  UI.button = (label, run, disabled = false, reason = "") => {
    const b = n("button", tr(label));
    b.type = "button";
    if (typeof label === "string" && label.length > 1)
      b.classList.add("terminal-action");
    b.disabled = !!disabled;
    if (reason) b.dataset.help = reason;
    b.onclick = async (e) => {
      b.disabled = true;
      b.classList.add("is-busy");
      try {
        await run?.(e);
      } catch (error) {
        notify(error.message, "error");
      } finally {
        b.classList.remove("is-busy");
        b.disabled = !!disabled;
      }
    };
    return b;
  };
  UI.field = (root, label, id, value, type = "text", help = "") => {
    const l = n("label"),
      caption = n("span", tr(label)),
      i = n("input");
    l.append(caption, i);
    i.id = id;
    i.type = type;
    i.value = value ?? "";
    if (help) l.dataset.help = help;
    if (type === "number") {
      i.min = 0;
      i.step = 1;
    }
    root.append(l);
    return i;
  };
  UI.row = (root, label, value) => {
    const r = n("div", undefined, "row");
    r.append(
      n("span", tr(label)),
      value instanceof Node ? value : n("b", String(value ?? "—")),
    );
    root.append(r);
    return r;
  };
  UI.bar = (value, max, label) => {
    const wrap = n("div", undefined, "meter"),
      bar = n("progress");
    bar.max = max;
    bar.value = value;
    bar.setAttribute("aria-label", tr(label));
    wrap.append(
      n("span", tr(label)),
      n("b", `${Number(value.toFixed?.(1) ?? value)} / ${max}`),
      bar,
    );
    return wrap;
  };
  UI.tabs = (root, choices, value, change) => {
    const tabs = n("div", undefined, "tabs");
    tabs.setAttribute("role", "tablist");
    for (const [id, label] of choices) {
      const b = UI.button(label, () => change(id));
      b.classList.toggle("active", value === id);
      b.setAttribute("role", "tab");
      b.setAttribute("aria-selected", String(value === id));
      tabs.append(b);
    }
    root.append(tabs);
  };
  UI.table = (headers, rows) => {
    const wrap = n("div", undefined, "table-wrap"),
      table = n("table"),
      head = n("thead"),
      trh = n("tr");
    headers.forEach((h) => trh.append(n("th", tr(h))));
    head.append(trh);
    const body = n("tbody");
    for (const row of rows) {
      const trr = n("tr");
      for (const value of row) {
        const td = n("td");
        td.append(
          value instanceof Node
            ? value
            : document.createTextNode(String(value ?? "—")),
        );
        trr.append(td);
      }
      body.append(trr);
    }
    table.append(head, body);
    wrap.append(table);
    return wrap;
  };
  UI.empty = (root, text) => root.append(n("p", tr(text), "empty-state"));
  let current;
  const pickerRegistry = new WeakMap();
  window.addEventListener("languagechange", () => {
    for (const button of document.querySelectorAll(".picker-button")) {
      const entry = pickerRegistry.get(button);
      if (!entry) continue;
      for (const option of entry.opts)
        option.label = option.raw ? option.source : I18n.t(option.source);
      entry.update();
    }
  });
  UI.picker = (options, value, change, config = {}) => {
    const opts = options.map((o) =>
        Array.isArray(o)
          ? {
              id: String(o[0]),
              label: tr(o[1]),
              source: o[1],
              raw: !!config.raw,
            }
          : { ...o, id: String(o.id), source: o.label, raw: true },
      ),
      button = UI.button("", () => {});
    button.classList.add("picker-button");
    if (config.system) button.classList.add("system-picker");
    button.setAttribute("role", "combobox");
    button.setAttribute("aria-expanded", "false");
    button.setAttribute("aria-haspopup", "listbox");
    button.value = String(value ?? opts[0]?.id ?? "");
    const update = () => {
      for (const option of opts)
        if (option.station) {
          const port = window.gameData?.workbench?.stations?.find(
              (p) =>
                p.star === option.station.star &&
                p.object_id === option.station.id,
            ),
            name = port?.name;
          if (name) {
            if (option.stationName && option.label.includes(option.stationName))
              option.label = option.label.replace(option.stationName, name);
            option.stationName = name;
            option.source = option.label;
          }
        }
      const selected = opts.find((o) => o.id === button.value);
      button.replaceChildren();
      button.classList.toggle("picker-has-code", !!selected?.code);
      if (selected?.code) button.append(n("span", selected.code, "system-id"));
      button.append(
        selected?.station
          ? UI.stationLabel(
              selected.station.star,
              selected.station.id,
              selected.label,
            )
          : n(
              "span",
              selected?.label || tr("Выберите значение"),
              "picker-label",
            ),
        n("span", "⌄", "picker-chevron"),
      );
    };
    update();
    pickerRegistry.set(button, { opts, update });
    button.setValue = (value) => {
      button.value = String(value);
      update();
    };
    button.onclick = () => {
      if (current?.button === button) {
        current.close();
        return;
      }
      current?.close();
      const pop = n("div", undefined, "picker-popover"),
        search = n("input"),
        list = n("div", undefined, "picker-list"),
        footer = n("div", undefined, "pagination");
      if (config.system) pop.classList.add("system-popover");
      let page = 1,
        focusIndex = -1;
      search.type = "search";
      search.placeholder = tr(
        config.system ? "Название или ID системы" : "Поиск",
      );
      search.setAttribute("aria-label", search.placeholder);
      const favorites = n("div", undefined, "picker-favorites"),
        columns = n("div", undefined, "picker-columns");
      if (config.favorites) {
        const all = n("div");
        all.append(
          n(
            "div",
            tr(config.station ? "Все станции" : "Все системы"),
            "picker-column-title",
          ),
          list,
        );
        columns.append(all, favorites);
        pop.append(search, columns, footer);
      } else pop.append(search, list, footer);
      document.body.append(pop);
      button.setAttribute("aria-expanded", "true");
      const close = () => {
        pop.remove();
        button.setAttribute("aria-expanded", "false");
        document.removeEventListener("pointerdown", outside);
        current = null;
      };
      current = { close, button };
      const outside = (e) => {
        if (!pop.contains(e.target) && !button.contains(e.target)) close();
      };
      setTimeout(() => document.addEventListener("pointerdown", outside), 0);
      const render = () => {
        const q = search.value.toLowerCase().trim(),
          filtered = opts.filter((o) =>
            (o.label + " " + (o.code || "") + " " + o.id)
              .toLowerCase()
              .includes(q),
          ),
          size = config.pageSize || 10,
          pages = Math.max(1, Math.ceil(filtered.length / size));
        page = Math.min(page, pages);
        list.replaceChildren();
        list.setAttribute("role", "listbox");
        footer.replaceChildren();
        const optionRow = (o, host) => {
          const b = UI.button("", () => {
            button.value = o.id;
            update();
            close();
            button.focus();
            change?.(button.value);
            button.onchange?.({ target: button });
          });
          b.classList.add("picker-option");
          b.classList.toggle("picker-has-code", !!o.code);
          b.setAttribute("role", "option");
          b.setAttribute("aria-selected", String(o.id === button.value));
          if (o.code) b.append(n("span", o.code, "system-id"));
          b.append(
            o.station
              ? UI.stationLabel(o.station.star, o.station.id, o.label)
              : n("span", o.label, "picker-label"),
          );
          if (o.extra) b.append(n("small", o.extra));
          if (config.favorites) {
            const row = n("div", undefined, "picker-entry"),
              fav = UI.button(
                config.favorites.has(o.id) ? "★" : "☆",
                async () => {
                  await config.favorites.toggle(o.id);
                  render();
                },
              );
            fav.className = "favorite-toggle";
            fav.dataset.help = "Избранное";
            fav.setAttribute("aria-label", tr("Избранное") + ": " + o.label);
            row.append(b, fav);
            host.append(row);
          } else host.append(b);
        };
        for (const o of filtered.slice((page - 1) * size, page * size))
          optionRow(o, list);
        if (config.favorites) {
          favorites.replaceChildren(
            n("div", tr("Избранное"), "picker-column-title"),
          );
          const saved = filtered.filter((o) => config.favorites.has(o.id));
          for (const o of saved) optionRow(o, favorites);
          if (!saved.length) UI.empty(favorites, "Нет избранных объектов");
        }
        if (!filtered.length) UI.empty(list, "Ничего не найдено");
        footer.append(
          UI.button(
            "‹",
            () => {
              page--;
              render();
            },
            page === 1,
          ),
          n(
            "span",
            `${filtered.length ? (page - 1) * size + 1 : 0}–${Math.min(page * size, filtered.length)} / ${filtered.length}`,
          ),
          UI.button(
            "›",
            () => {
              page++;
              render();
            },
            page === pages,
          ),
        );
        const r = button.getBoundingClientRect();
        pop.style.width =
          Math.min(
            Math.max(
              r.width,
              config.favorites ? 620 : config.system ? 350 : 260,
            ),
            innerWidth - 16,
          ) + "px";
        pop.style.left =
          Math.max(8, Math.min(r.left, innerWidth - pop.offsetWidth - 8)) +
          "px";
        pop.style.top =
          Math.max(
            65,
            Math.min(r.bottom + 5, innerHeight - pop.offsetHeight - 40),
          ) + "px";
      };
      search.oninput = () => {
        page = 1;
        focusIndex = -1;
        render();
      };
      pop.onkeydown = (e) => {
        if (e.key === "Escape") {
          e.stopPropagation();
          close();
          button.focus();
        }
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
          e.preventDefault();
          const buttons = [...list.querySelectorAll("button")];
          focusIndex = Math.max(
            0,
            Math.min(
              buttons.length - 1,
              focusIndex + (e.key === "ArrowDown" ? 1 : -1),
            ),
          );
          buttons[focusIndex]?.focus();
        }
        if (e.key === "Enter" && e.target === search) {
          e.preventDefault();
          list.querySelector("button")?.click();
        }
      };
      render();
      search.focus();
    };
    return button;
  };
  UI.choice = (root, label, options, value, change) => {
    const l = n("label");
    l.append(n("span", tr(label)));
    const picker = UI.picker(options, value, change);
    picker.dataset.field = label;
    l.append(picker);
    root.append(l);
    return picker;
  };
  UI.bookmarks = (station) => {
    const target = (id) =>
      station
        ? {
            star: Number(String(id).split("|")[0]),
            object: String(id).split("|")[1],
          }
        : { star: Number(id), object: "" };
    const get = (id) => {
      const t = target(id);
      return gameData.workbench?.notes?.find(
        (n) => n.star === t.star && n.object_id === t.object,
      );
    };
    return {
      has: (id) => !!get(id)?.bookmark,
      toggle: async (id) => {
        const t = target(id),
          old = get(id);
        await Game.command({
          action: "note",
          star: t.star,
          target: t.object,
          note: old?.note || "",
          bookmark: !old?.bookmark,
        });
      },
    };
  };
  window.addEventListener("gamedata", () => {
    for (const button of document.querySelectorAll(".picker-button")) {
      const entry = pickerRegistry.get(button);
      if (entry?.opts.some((option) => option.station)) entry.update();
    }
  });
  UI.stationPicker = (root, ports, value, change) => {
    const picker = UI.picker(
      ports.map((p) => ({
        id: p.star + "|" + (p.station || p.object_id),
        code: String(p.star),
        label: p.name + (p.specialization ? " · " + tr(p.specialization) : ""),
        station: { star: p.star, id: p.station || p.object_id },
      })),
      value,
      change,
      { system: true, station: true, favorites: UI.bookmarks(true) },
    );
    root.append(picker);
    return picker;
  };
  UI.systems = (value, change) =>
    UI.picker(
      (window.MapData?.stars || [])
        .filter((s) => !s.deleted)
        .map((s) => ({
          id: s.id,
          code: String(s.id),
          label: s.name,
          extra:
            (gameData.workbench?.known.some((v) => v.star === s.id)
              ? "✓ "
              : "") +
            (gameData.fleet?.ships.some((ship) => ship.star === s.id)
              ? "◇"
              : ""),
        })),
      value,
      change,
      { system: true, favorites: UI.bookmarks(false) },
    );
  UI.dirty = (form, save, cancel) => {
    const bar = n("div", undefined, "save-bar");
    bar.hidden = true;
    const status = n("span", tr("Есть несохранённые изменения")),
      yes = UI.button("Сохранить", async () => {
        yes.disabled = true;
        status.textContent = tr("Сохранение…");
        try {
          const ok = await save();
          if (ok !== false) {
            bar.hidden = true;
            form.dataset.dirty = "false";
            form
              .querySelectorAll(".field-dirty")
              .forEach((e) => e.classList.remove("field-dirty"));
          }
        } finally {
          yes.disabled = false;
          status.textContent = tr("Есть несохранённые изменения");
        }
      }),
      no = UI.button("Отменить", () => {
        cancel();
        bar.hidden = true;
        form.dataset.dirty = "false";
        form
          .querySelectorAll(".field-dirty")
          .forEach((e) => e.classList.remove("field-dirty"));
      });
    bar.append(status, yes, no);
    form.append(bar);
    const dirty = () => {
      form.dataset.dirty = "true";
      bar.hidden = false;
    };
    form.addEventListener("input", dirty);
    form.addEventListener("change", dirty);
    form.onsubmit = (e) => {
      e.preventDefault();
      if (!bar.hidden) yes.click();
    };
    return dirty;
  };
})();
(() => {
  const paths = {
    galaxy: "M12 3v18M3 12h18M5 5l14 14M5 19L19 5",
    ship: "M3 17L21 3l-6 18-4-8-8 4z",
    research: "M4 20l9-9m-6-7c5-2 10 1 13 6l-4 3c-1-4-3-6-7-6z",
    market: "M3 7h16l-4-4m6 14H5l4 4",
    corporations: "M12 2l9 5v10l-9 5-9-5V7zM8 9h8v7H8z",
    tasks: "M8 6h13M8 12h13M8 18h13M3 6h1M3 12h1M3 18h1",
    studies:
      "M12 2v4M12 18v4M2 12h4M18 12h4M7 5l2 3M17 5l-2 3M7 19l2-3M17 19l-2-3M7 12a5 5 0 1 0 10 0a5 5 0 1 0-10 0",
    ideas: "M12 3a6 6 0 0 1 4 10l-1 2H9l-1-2a6 6 0 0 1 4-10zM9 18h6M10 21h4",
    guide: "M3 4h7l2 2 2-2h7v15h-7l-2 2-2-2H3zM12 6v15",
    portal:
      "M12 4c5 0 9 4 9 8s-4 8-9 8-9-4-9-8 4-8 9-8zM8 12c0-3 2-5 4-5s4 2 4 5-2 5-4 5-4-2-4-5z",
  };
  const svg = (path) => {
    const s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    s.setAttribute("viewBox", "0 0 24 24");
    s.setAttribute("aria-hidden", "true");
    const p = document.createElementNS(s.namespaceURI, "path");
    p.setAttribute("d", path);
    s.append(p);
    return s;
  };
  for (const b of document.querySelectorAll("#sidebar [data-open]")) {
    const icon = b.querySelector(".nav-icon");
    if (icon && paths[b.dataset.open])
      icon.replaceChildren(svg(paths[b.dataset.open]));
    b.dataset.help = b.dataset.nav || b.textContent.trim();
  }
  const bell = document.getElementById("notifications-button"),
    count = document.getElementById("notification-count");
  bell.replaceChildren(svg("M6 8a6 6 0 0 1 12 0v6l3 3H3l3-3V8M9 21h6"));
  bell.append(count);
  const pass = document.querySelector("#auth-form input[name=password]"),
    wrap = UI.node("div", undefined, "password-wrap"),
    show = UI.button("◉", () => {
      pass.type = pass.type === "password" ? "text" : "password";
      show.setAttribute("aria-pressed", String(pass.type === "text"));
    });
  show.className = "password-toggle";
  show.setAttribute("aria-label", I18n.t("Показать пароль"));
  show.setAttribute("aria-pressed", "false");
  pass.replaceWith(wrap);
  wrap.append(pass, show);
  I18n.ready.then(() => {
    for (const select of document.querySelectorAll("#window-settings select")) {
      const picker = UI.picker(
        [...select.options].map((o) => [o.value, o.textContent]),
        select.value,
        (value) => {
          select.value = value;
          select.onchange?.();
        },
      );
      select.hidden = true;
      select.after(picker);
    }
  });
})();
