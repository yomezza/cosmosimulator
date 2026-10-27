"use strict";
(() => {
  const windows = new Map(),
    links = new Map();
  let z = 40,
    resetting = false;
  const tr = (s) => I18n.t(s),
    read = (k, f) => {
      try {
        return JSON.parse(localStorage.getItem(k)) ?? f;
      } catch {
        return f;
      }
    };
  const node = (tag, text, cls) => {
    const e = document.createElement(tag);
    if (text !== undefined) e.textContent = text;
    if (cls) e.className = cls;
    return e;
  };
  function active(w) {
    for (const entry of windows.values())
      entry.dialog.classList.toggle("window-active", entry === w);
    w.dialog.style.zIndex = ++z;
    switcher();
  }
  function switcher() {
    const host = document.getElementById("window-switcher");
    if (!host) return;
    host.replaceChildren();
    for (const [id, w] of windows) {
      if (!w.dialog.open) continue;
      const b = node("button", w.title);
      b.classList.toggle(
        "active",
        w.dialog.classList.contains("window-active"),
      );
      b.onclick = () => {
        active(w);
        w.bar.focus();
      };
      host.append(b);
    }
  }
  function save(id, w) {
    if (resetting || !w.dialog.open || w.maximized) return;
    const r = w.dialog.getBoundingClientRect();
    try {
      localStorage.setItem(
        "module:" + id,
        JSON.stringify({
          x: r.x,
          y: r.y,
          w: r.width,
          h: r.height,
          pinned: w.pinned,
        }),
      );
    } catch {}
  }
  function clamp(w) {
    const d = w.dialog,
      r = d.getBoundingClientRect();
    d.style.left = Math.max(4, Math.min(r.x, innerWidth - r.width - 4)) + "px";
    d.style.top =
      Math.max(62, Math.min(r.y, innerHeight - r.height - 36)) + "px";
  }
  function linkedPeers(id) {
    const seen = new Set([id]),
      result = [],
      queue = [id];
    for (let i = 0; i < queue.length; i++) {
      for (const peerID of links.get(queue[i]) || []) {
        if (seen.has(peerID)) continue;
        seen.add(peerID);
        const peer = windows.get(peerID);
        if (!peer?.dialog.open || peer.pinned || peer.maximized) continue;
        queue.push(peerID);
        result.push([peerID, peer]);
      }
    }
    return result;
  }
  window.Modules = {
    create(id, title, width = 820, height = 600) {
      if (windows.has(id)) return windows.get(id).content;
      const d = node("dialog", undefined, "module-window"),
        bar = node("div", undefined, "module-titlebar"),
        h = node("h2", tr(title)),
        actions = node("div", undefined, "window-actions"),
        pin = node("button", "⌖"),
        max = node("button", "□"),
        close = node("button", "×"),
        content = node("div", undefined, "module-content"),
        small = node(
          "p",
          tr("Увеличьте окно, чтобы увидеть содержимое"),
          "window-small",
        );
      d.id = "window-" + id;
      h.id = "title-" + id;
      h.dataset.i18n = title;
      d.setAttribute("aria-labelledby", h.id);
      bar.tabIndex = 0;
      for (const [b, label] of [
        [pin, "Закрепить окно"],
        [max, "Развернуть окно"],
        [close, "Закрыть"],
      ]) {
        b.type = "button";
        b.dataset.help = label;
        b.dataset.label = label;
        b.setAttribute("aria-label", tr(label));
      }
      pin.setAttribute("aria-pressed", "false");
      actions.append(pin, max, close);
      bar.append(h, actions);
      d.append(bar, content, small);
      document.body.append(d);
      const w = {
        dialog: d,
        content,
        bar,
        h,
        title: tr(title),
        pinned: false,
        maximized: false,
        initialized: false,
        width,
        height,
      };
      windows.set(id, w);
      const show = () => {
        if (!d.open) {
          d.show();
          if (!w.initialized) {
            const l = read("module:" + id, {});
            d.style.width = Math.min(l.w || width, innerWidth - 12) + "px";
            d.style.height = Math.min(l.h || height, innerHeight - 106) + "px";
            d.style.left =
              (l.x ?? Math.max(6, (innerWidth - width) / 2)) + "px";
            d.style.top =
              (l.y ?? Math.max(64, (innerHeight - height) / 2)) + "px";
            w.pinned = !!l.pinned;
            d.classList.toggle("pinned", w.pinned);
            pin.setAttribute("aria-pressed", String(w.pinned));
            w.initialized = true;
          }
          clamp(w);
        }
        active(w);
        bar.focus();
        document
          .querySelectorAll(`#sidebar [data-open="${id}"]`)
          .forEach((b) => b.classList.add("module-open"));
        window.dispatchEvent(new CustomEvent("moduleopen", { detail: id }));
      };
      const hide = () => {
        save(id, w);
        d.close();
        document
          .querySelectorAll(`#sidebar [data-open="${id}"]`)
          .forEach((b) => b.classList.remove("module-open"));
        switcher();
        window.dispatchEvent(new CustomEvent("moduleclose", { detail: id }));
      };
      w.show = show;
      w.hide = hide;
      const maximize = () => {
        if (w.pinned) return;
        if (!w.maximized) {
          const r = d.getBoundingClientRect();
          w.restore = { x: r.x, y: r.y, w: r.width, h: r.height };
          d.style.left = "5px";
          d.style.top = "62px";
          d.style.width = innerWidth - 10 + "px";
          d.style.height = innerHeight - 101 + "px";
        } else {
          const r = w.restore;
          d.style.left = r.x + "px";
          d.style.top = r.y + "px";
          d.style.width = r.w + "px";
          d.style.height = r.h + "px";
          clamp(w);
        }
        w.maximized = !w.maximized;
        d.classList.toggle("maximized", w.maximized);
        max.textContent = w.maximized ? "▣" : "□";
        save(id, w);
      };
      pin.onclick = () => {
        w.pinned = !w.pinned;
        d.classList.toggle("pinned", w.pinned);
        pin.setAttribute("aria-pressed", String(w.pinned));
        pin.dataset.help = w.pinned ? "Открепить окно" : "Закрепить окно";
        save(id, w);
      };
      close.onclick = hide;
      max.onclick = maximize;
      bar.ondblclick = (e) => {
        if (!e.target.closest("button")) maximize();
      };
      let drag, resize;
      d.addEventListener("pointerdown", (e) => {
        active(w);
        if (w.pinned || w.maximized || e.button !== 0) return;
        const r = d.getBoundingClientRect(),
          dx = e.clientX - r.x,
          dy = e.clientY - r.y;
        const right = r.width - dx < 8,
          bottom = r.height - dy < 8,
          left = dx < 6;
        if (right || bottom || left) {
          resize = { x: e.clientX, y: e.clientY, r, right, bottom, left };
          d.setPointerCapture(e.pointerId);
          e.preventDefault();
        }
      });
      bar.onpointerdown = (e) => {
        if (
          w.pinned ||
          w.maximized ||
          e.target.closest("button") ||
          e.button !== 0
        )
          return;
        const r = d.getBoundingClientRect();
        drag = {
          x: e.clientX - r.x,
          y: e.clientY - r.y,
        };
        bar.setPointerCapture(e.pointerId);
      };
      bar.onpointermove = (e) => {
        if (!drag) return;
        const moving = [[id, w], ...linkedPeers(id)],
          rects = moving.map(([, peer]) => peer.dialog.getBoundingClientRect()),
          r = rects[0],
          left = Math.min(...rects.map((r) => r.left)),
          right = Math.max(...rects.map((r) => r.right)),
          top = Math.min(...rects.map((r) => r.top)),
          bottom = Math.max(...rects.map((r) => r.bottom)),
          dx = Math.max(
            4 - left,
            Math.min(e.clientX - drag.x - r.x, innerWidth - 4 - right),
          ),
          dy = Math.max(
            62 - top,
            Math.min(e.clientY - drag.y - r.y, innerHeight - 36 - bottom),
          );
        for (let i = 0; i < moving.length; i++) {
          const [peerID, peer] = moving[i];
          peer.dialog.style.left = rects[i].x + dx + "px";
          peer.dialog.style.top = rects[i].y + dy + "px";
          save(peerID, peer);
        }
      };
      d.addEventListener("pointermove", (e) => {
        if (!resize) return;
        const dx = e.clientX - resize.x,
          dy = e.clientY - resize.y,
          r = resize.r;
        if (resize.right) d.style.width = Math.max(260, r.width + dx) + "px";
        if (resize.left) {
          const width = Math.max(260, r.width - dx);
          d.style.width = width + "px";
          d.style.left = r.right - width + "px";
        }
        if (resize.bottom) d.style.height = Math.max(170, r.height + dy) + "px";
      });
      const finish = () => {
        drag = null;
        resize = null;
        save(id, w);
      };
      for (const event of [
        "pointerup",
        "pointercancel",
        "lostpointercapture",
      ]) {
        bar.addEventListener(event, finish);
        d.addEventListener(event, finish);
      }
      bar.onkeydown = (e) => {
        const delta = {
          ArrowLeft: [-20, 0],
          ArrowRight: [20, 0],
          ArrowUp: [0, -20],
          ArrowDown: [0, 20],
        }[e.key];
        if (!delta || w.pinned || w.maximized || e.target !== bar) return;
        e.preventDefault();
        d.style.left = d.offsetLeft + delta[0] + "px";
        d.style.top = d.offsetTop + delta[1] + "px";
        clamp(w);
        save(id, w);
      };
      d.addEventListener("keydown", (e) => {
        if (
          e.key === "Escape" &&
          !document.querySelector(".context-menu:not([hidden]),.picker-popover")
        ) {
          e.preventDefault();
          hide();
        }
      });
      let timer;
      new ResizeObserver(() => {
        if (!d.open) return;
        const r = d.getBoundingClientRect();
        d.classList.toggle(
          "too-small",
          r.width < (id === "hologram" ? 240 : 350) ||
            r.height < (id === "hologram" ? 180 : 220),
        );
        clearTimeout(timer);
        timer = setTimeout(() => save(id, w), 200);
        window.dispatchEvent(new CustomEvent("moduleresize", { detail: id }));
      }).observe(d);
      window.addEventListener("resize", () => {
        if (!d.open) return;
        d.style.width = Math.min(d.offsetWidth, innerWidth - 12) + "px";
        d.style.height = Math.min(d.offsetHeight, innerHeight - 106) + "px";
        clamp(w);
      });
      return content;
    },
    open: (id) => windows.get(id)?.show(),
    focus: (id) => {
      const w = windows.get(id);
      if (w?.dialog.open) {
        active(w);
        w.bar.focus();
      }
    },
    close: (id) => windows.get(id)?.hide(),
    isOpen: (id) => !!windows.get(id)?.dialog.open,
    closeAll() {
      for (const w of windows.values()) w.hide();
    },
    clearPrivate() {
      for (const [id, w] of windows)
        if (id !== "galaxy" && id !== "settings") w.content.replaceChildren();
    },
    title(id, text) {
      const w = windows.get(id);
      if (w) {
        w.title = text;
        w.h.removeAttribute("data-i18n");
        w.h.textContent = text;
        switcher();
      }
    },
    adjacent(id, anchor, side = "right") {
      const w = windows.get(id),
        a = windows.get(anchor);
      if (!w || !a || w.pinned || w.maximized) return;
      const r = a.dialog.getBoundingClientRect();
      w.dialog.style.left =
        (side === "left"
          ? Math.max(6, r.left - w.dialog.offsetWidth - 8)
          : Math.min(innerWidth - w.dialog.offsetWidth - 6, r.right + 8)) +
        "px";
      w.dialog.style.top = r.y + "px";
      clamp(w);
      save(id, w);
    },
    link(a, b, enabled) {
      if (enabled) {
        if (!links.has(a)) links.set(a, new Set());
        if (!links.has(b)) links.set(b, new Set());
        links.get(a).add(b);
        links.get(b).add(a);
      } else {
        links.get(a)?.delete(b);
        links.get(b)?.delete(a);
        if (!links.get(a)?.size) links.delete(a);
        if (!links.get(b)?.size) links.delete(b);
      }
    },
    linked: (id, anchor) =>
      anchor ? !!links.get(id)?.has(anchor) : links.has(id),
    arrangeShip() {
      const ship = windows.get("ship");
      if (!ship?.dialog.open) return;
      const members = ["hologram", "ship", "cargo"]
        .filter(
          (id) =>
            id === "ship" ||
            (windows.get(id)?.dialog.open && Modules.linked(id, "ship")),
        )
        .map((id) => ({ id, w: windows.get(id) }));
      if (members.some(({ w }) => w.pinned || w.maximized)) {
        for (const { id } of members)
          if (id !== "ship")
            Modules.adjacent(id, "ship", id === "hologram" ? "left" : "right");
        return;
      }
      const sidebar = document.getElementById("sidebar"),
        left =
          innerWidth >= 900 && !sidebar.hidden
            ? sidebar.getBoundingClientRect().right + 12
            : 6,
        available = innerWidth - left - 8,
        gap = 8,
        rects = members.map(({ w }) => w.dialog.getBoundingClientRect());
      if (
        rects.every(
          (r, i) =>
            r.left >= left &&
            r.right <= innerWidth - 8 &&
            (i === 0 || r.left >= rects[i - 1].right + gap - 1),
        )
      )
        return;
      const minima = members.map(({ id }) => (id === "hologram" ? 240 : 350)),
        widths = rects.map((r, i) => Math.max(minima[i], r.width)),
        total = widths.reduce((a, b) => a + b, 0),
        minimum = minima.reduce((a, b) => a + b, 0),
        budget = available - gap * (members.length - 1),
        fits = budget >= minimum;
      if (fits && total > budget) {
        const extra = total - minimum;
        widths.forEach(
          (v, i) =>
            (widths[i] =
              minima[i] + ((v - minima[i]) * (budget - minimum)) / extra),
        );
      } else if (!fits) {
        widths.forEach((v, i) => (widths[i] = Math.min(minima[i], available)));
      }
      const span =
          widths.reduce((a, b) => a + b, 0) + gap * (members.length - 1),
        maxHeight = Math.max(...rects.map((r) => r.height)),
        y = Math.max(
          64,
          Math.min(ship.dialog.offsetTop, innerHeight - maxHeight - 36),
        );
      let x = left + Math.max(0, (available - span) / 2);
      members.forEach(({ id, w }, i) => {
        if (!fits)
          x =
            left +
            (available - widths[i]) *
              (members.length === 1 ? 0.5 : i / (members.length - 1));
        w.dialog.style.width = widths[i] + "px";
        w.dialog.style.left = x + "px";
        w.dialog.style.top = y + "px";
        clamp(w);
        save(id, w);
        x += widths[i] + gap;
      });
    },
  };
  for (const [id, title, w, h] of [
    ["galaxy", "Галактическая карта", 1180, 700],
    ["ship", "Корабль", 480, 640],
    ["hologram", "Голограмма корабля", 360, 470],
    ["cargo", "Трюм", 360, 590],
    ["research", "Разведка и добыча", 1000, 660],
    ["market", "Рынок", 1060, 660],
    ["corporations", "Корпорация", 1000, 660],
    ["profile", "Профиль игрока", 480, 420],
    ["settings", "Настройки", 480, 430],
    ["users", "Пользователи", 920, 620],
    ["gameplay", "Игровой процесс", 800, 700],
    ["galaxies", "Галактики", 1120, 760],
    ["adminchanges", "Журнал изменений", 980, 650],
    ["notifications", "Уведомления", 430, 620],
    ["guide", "Руководство пилота", 950, 650],
    ["tasks", "Планировщик задач", 900, 670],
    ["finance", "История финансов", 820, 580],
    ["benchmark", "Бенчмарк", 960, 660],
    ["portal", "Портал Эдема", 700, 560],
  ])
    Modules.create(id, title, w, h);
  let arrangeTimer;
  window.addEventListener("resize", () => {
    clearTimeout(arrangeTimer);
    arrangeTimer = setTimeout(() => Modules.arrangeShip(), 80);
  });
  const settings = document.querySelector("#window-settings .module-content");
  settings.append(node("h3", tr("Язык")));
  const language = node("select");
  language.id = "language";
  for (const [v, label] of [
    ["ru", "Русский"],
    ["en", "English"],
  ]) {
    const option = node("option", label);
    option.value = v;
    language.append(option);
  }
  settings.append(language);
  language.onchange = () => I18n.set(language.value);
  settings.append(node("h3", tr("Отображение")));
  const density = node("select");
  for (const [v, label] of [
    ["normal", "Обычный интерфейс"],
    ["compact", "Компактный интерфейс"],
  ]) {
    const o = node("option", tr(label));
    o.value = v;
    density.append(o);
  }
  density.value = localStorage.getItem("density") || "normal";
  document.body.classList.toggle("compact-ui", density.value === "compact");
  density.onchange = () => {
    localStorage.setItem("density", density.value);
    document.body.classList.toggle("compact-ui", density.value === "compact");
  };
  settings.append(
    density,
    node(
      "p",
      tr("Плотность меняет размеры элементов во всех рабочих модулях."),
      "muted",
    ),
    node("h3", tr("Управление окнами")),
  );
  const reset = node("button", tr("Восстановить расположение окон"));
  reset.onclick = () => {
    resetting = true;
    for (const [id, w] of windows) {
      w.hide();
      localStorage.removeItem("module:" + id);
    }
    location.reload();
  };
  settings.append(reset);
  for (const el of settings.querySelectorAll("h3,p,button"))
    el.dataset.i18n = el.textContent;
  window.addEventListener("languagechange", () => {
    for (const w of windows.values()) {
      if (w.h.dataset.i18n) w.title = tr(w.h.dataset.i18n);
    }
    switcher();
  });
  document.addEventListener("click", (e) => {
    const b = e.target.closest("[data-open]");
    if (b) {
      Modules.open(b.dataset.open);
      if (b.closest("#profile-menu"))
        document.getElementById("profile-menu").open = false;
    }
    for (const menu of document.querySelectorAll(".dropdown"))
      if (!menu.contains(e.target)) menu.open = false;
  });
  const sidebar = document.getElementById("sidebar");
  sidebar.classList.toggle("collapsed", read("sidebar-collapsed", false));
  const sidebarToggle = document.getElementById("sidebar-toggle");
  const updateSidebar = () => {
    const collapsed = sidebar.classList.contains("collapsed");
    sidebarToggle.setAttribute("aria-expanded", String(!collapsed));
    sidebarToggle.setAttribute(
      "aria-label",
      I18n.t(collapsed ? "Развернуть панель" : "Свернуть панель"),
    );
    sidebarToggle.dataset.help = collapsed
      ? "Развернуть панель"
      : "Свернуть панель";
  };
  sidebarToggle.onclick = () => {
    sidebar.classList.toggle("collapsed");
    localStorage.setItem(
      "sidebar-collapsed",
      JSON.stringify(sidebar.classList.contains("collapsed")),
    );
    updateSidebar();
  };
  window.addEventListener("languagechange", updateSidebar);
  updateSidebar();
  I18n.ready.then(updateSidebar);
  window.notify = (text, level = "success", persist = false) => {
    const toast = document.getElementById("toast");
    toast.replaceChildren(
      node(
        "span",
        level === "error" ? "×" : level === "warning" ? "!" : "✓",
        "toast-icon",
      ),
      node("span", tr(text)),
      node("i", undefined, "toast-countdown"),
    );
    toast.dataset.level = level;
    toast.hidden = false;
    clearTimeout(window.toastTimer);
    window.toastTimer = setTimeout(() => (toast.hidden = true), 6000);
    if (persist && typeof signedIn !== "undefined" && signedIn)
      api("notifications", { action: "add", message: text, level })
        .then(() => window.dispatchEvent(new Event("notificationchange")))
        .catch(() => {});
  };
  const tip = document.getElementById("tooltip");
  function showHelp(target, x, y) {
    if (!target) return;
    tip.textContent = target.dataset.helpRaw || tr(target.dataset.help);
    tip.hidden = false;
    tip.style.left =
      Math.max(8, Math.min(x + 12, innerWidth - tip.offsetWidth - 8)) + "px";
    tip.style.top =
      Math.max(8, Math.min(y + 14, innerHeight - tip.offsetHeight - 10)) + "px";
  }
  document.addEventListener("pointerover", (e) =>
    showHelp(e.target.closest("[data-help]"), e.clientX, e.clientY),
  );
  document.addEventListener("pointerout", (e) => {
    if (e.target.closest("[data-help]")) tip.hidden = true;
  });
  document.addEventListener("focusin", (e) => {
    const target = e.target.closest("[data-help]");
    if (target) {
      const r = target.getBoundingClientRect();
      showHelp(target, r.x, r.bottom);
    }
  });
  document.addEventListener("focusout", () => (tip.hidden = true));
})();
