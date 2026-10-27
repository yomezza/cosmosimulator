"use strict";
(() => {
  const n = UI.node,
    tr = (s) => I18n.t(s);
  let selected = "edem",
    request = 0,
    logRequest = 0,
    draft = null;
  const admin = () => signedIn && gameData.state?.user.role === "admin";
  const labels = {
    systems: "Системы",
    lanes: "Гиперкоридоры",
    cross_links: "Связи между рукавами",
    station_systems: "Системы со станциями",
    stations: "Станции",
    founder_systems: "Системы основателей",
    neutral_systems: "Нейтральные системы",
  };
  const actions = {
    tick: "Пропустить тик",
    gameplay: "Игровой процесс",
    user_edit: "Редактирование пользователя",
    teleport: "Телепортация",
    galaxy_update: "Изменение галактики",
    galaxy_delete: "Удаление галактики",
  };
  window.GalaxyAdmin = {
    async render(root) {
      if (!admin()) {
        root.replaceChildren();
        return false;
      }
      const key = ++request,
        epoch = authEpoch,
        result = await api("admin/galaxies");
      if (
        key !== request ||
        epoch !== authEpoch ||
        !admin() ||
        !Modules.isOpen("galaxies")
      )
        return false;
      root.replaceChildren();
      root.classList.add("galaxy-admin");
      if (draft) result.galaxies.push(draft);
      const create = UI.button("Создать галактику", () => {
        draft = {
          definition: {
            id: "g-" + Date.now().toString(36),
            name: "",
            shape: "spiral",
            seed: Date.now(),
            opened: true,
            create: true,
            settings: structuredClone(result.galaxies[0].definition.settings),
          },
          stats: Object.fromEntries(Object.keys(labels).map((k) => [k, 0])),
        };
        selected = draft.definition.id;
        root.replaceChildren(n("p", tr("Загрузка…")));
        return GalaxyAdmin.render(root);
      });
      root.append(create);
      const layout = n("div", undefined, "galaxy-admin-layout"),
        list = n("aside", undefined, "galaxy-admin-list"),
        editor = n("section", undefined, "galaxy-admin-editor");
      layout.append(list, editor);
      root.append(layout);
      if (!result.galaxies.some((g) => g.definition.id === selected))
        selected = result.galaxies[0]?.definition.id;
      for (const entry of result.galaxies) {
        const d = entry.definition,
          card = UI.button("", () => {
            selected = d.id;
            return GalaxyAdmin.render(root);
          });
        card.classList.add("galaxy-admin-card");
        card.classList.toggle("selected", d.id === selected);
        card.append(
          n("b", d.name),
          n(
            "small",
            tr(
              {
                rings: "Кольцевая",
                spiral: "Спиральная",
                elliptical: "Эллиптическая",
                irregular: "Нерегулярная",
              }[d.shape],
            ),
          ),
          n(
            "span",
            d.opened
              ? `${entry.stats.systems} ${tr("систем")}`
              : tr("Галактика не открыта"),
          ),
        );
        list.append(card);
      }
      const entry = result.galaxies.find((g) => g.definition.id === selected);
      if (!entry) return;
      const d = entry.definition;
      editor.append(n("h2", d.name));
      const summary = n("div", undefined, "galaxy-stats");
      for (const [key, label] of Object.entries(labels))
        summary.append(n("span", `${tr(label)}: ${entry.stats[key]}`));
      editor.append(summary);
      if (d.id !== "edem" && d.id !== "asterion")
        editor.append(
          UI.button("Удалить галактику", async () => {
            const review = await api("admin/galaxies/delete-preview", {
              id: d.id,
            });
            if (epoch !== authEpoch || !admin()) return;
            const box = Modules.create(
              "galaxy-delete",
              "Удаление галактики",
              600,
              530,
            );
            box.replaceChildren(
              n("h3", d.name),
              n("p", tr("Системы") + ": " + review.systems),
            );
            const names = {
              ships: "Корабли и маршруты",
              warehouses: "Личные склады",
              corporate_storage: "Корпоративные запасы",
              stations: "Станции игроков",
              projects: "Корпоративные проекты",
              tasks: "Задачи планировщика",
              deliveries: "Доставки",
              reservations: "Бронирования",
              offers: "Рыночные предложения",
              research: "Исследование и добыча",
              player_data: "Данные игроков",
            };
            const blocked = Object.entries(review.blockers).filter(
              ([, v]) => v > 0,
            );
            box.append(
              n(
                "p",
                tr(
                  review.can_delete
                    ? "Галактика будет удалена с карт и переходов. Остальные галактики сохранятся."
                    : "Сначала освободите галактику от кораблей, имущества и данных игроков.",
                ),
                "muted",
              ),
            );
            if (blocked.length)
              box.append(
                UI.table(
                  ["Причина", "Количество"],
                  blocked.map(([k, v]) => [tr(names[k] || k), v]),
                ),
              );
            box.append(
              UI.button(
                "Удалить галактику",
                async () => {
                  await api("admin/galaxies/delete", {
                    id: d.id,
                    preview: review.preview,
                  });
                  Modules.close("galaxy-delete");
                  selected = "edem";
                  window.MapData = await api("galaxy");
                  window.dispatchEvent(new Event("universechange"));
                  await refresh(true);
                  await GalaxyAdmin.render(root);
                  notify(tr("Галактика удалена"));
                },
                !review.can_delete,
              ),
              UI.button("Отмена", () => Modules.close("galaxy-delete")),
            );
            Modules.open("galaxy-delete");
          }),
        );
      if (!d.opened) {
        UI.empty(editor, "Галактика откроется после активации портала");
        return;
      }
      const form = n("form"),
        grid = n("div", undefined, "galaxy-fields");
      form.onsubmit = (e) => e.preventDefault();
      form.append(grid);
      editor.append(form);
      const name = UI.field(grid, "Название галактики", "galaxy-name", d.name);
      name.maxLength = 48;
      name.required = true;
      const count = UI.field(
        grid,
        "Количество систем",
        "galaxy-count",
        d.create ? 1000 : entry.stats.systems,
        "number",
      );
      count.min = d.create ? 50 : entry.stats.systems;
      count.max = 10000;
      count.required = true;
      if (d.create) {
        UI.choice(
          grid,
          "Форма галактики",
          [
            ["spiral", "Спиральная"],
            ["rings", "Кольцевая"],
            ["elliptical", "Эллиптическая"],
            ["irregular", "Нерегулярная"],
          ],
          d.shape,
          (value) => {
            d.shape = value;
            form.dispatchEvent(new Event("input", { bubbles: true }));
          },
        );
        const seed = UI.field(
          grid,
          "Зерно генерации",
          "galaxy-seed",
          d.seed,
          "number",
        );
        seed.oninput = () => (d.seed = Number(seed.value));
      }
      const fields = {};
      const section = (title) => {
        const details = n("details");
        details.open = title !== "Распределение ресурсов";
        details.append(n("summary", tr(title)));
        const box = n("div", undefined, "galaxy-fields");
        details.append(box);
        form.append(details);
        return box;
      };
      const lanes = section("Гиперкоридоры");
      for (const [id, label, min, max, help] of [
        [
          "extra_lanes",
          "Дополнительные коридоры, %",
          0,
          100,
          "После коротких основных связей добавляются альтернативные пути. Меньшее значение делает сеть разреженной.",
        ],
        [
          "cross_links",
          "Связи между рукавами",
          0,
          40,
          "Соединяются ближайшие системы соседних рукавов в пределах максимальной длины коридора.",
        ],
        [
          "max_lane_length",
          "Максимальная длина коридора",
          50,
          10000,
          "Малое значение может разделить сеть на несколько областей. Действующие маршруты и два учебных коридора сохраняются.",
        ],
      ]) {
        const field = UI.field(
          lanes,
          label,
          "galaxy-" + id,
          d.settings[id],
          "number",
          help,
        );
        field.min = min;
        field.max = max;
        field.required = true;
        fields[id] = field;
      }
      const ports = section("Распределение станций");
      for (const [id, label, min, max] of [
        ["station_percent", "Частота станций в центре, %", 0, 100],
        ["max_ports", "Максимум портов в системе", 1, 5],
      ]) {
        const field = UI.field(
          ports,
          label,
          "galaxy-" + id,
          d.settings[id],
          "number",
        );
        field.min = min;
        field.max = max;
        field.required = true;
        fields[id] = field;
      }
      form.append(
        n(
          "p",
          tr(
            "К окраинам частота уменьшается до нуля. Владения основателей — небольшие колонии вокруг станций; между ними остаются нейтральные системы.",
          ),
          "muted",
        ),
      );
      form.append(
        n(
          "p",
          tr(
            "Стартовые порты, порты у чёрной дыры и станции с имуществом или действующими заказами сохраняются.",
          ),
          "muted",
        ),
      );
      const weights = { ...d.settings.resource_weights },
        resourceFields = {},
        res = section("Распределение ресурсов");
      form.append(
        n(
          "p",
          tr(
            "100% — базовая распространённость, 0% — новые месторождения этого сырья отсутствуют. Запасы в трюмах, на складах и рынках сохраняются.",
          ),
          "muted",
        ),
      );
      for (const resource of gameData.expedition.resources.filter(
        (r) => !["adamant", "electrum", "nectar", "soma"].includes(r.id),
      )) {
        const f = UI.field(
          res,
          tr(resource.name) + " · %",
          "galaxy-resource-" + resource.id,
          weights[resource.id],
          "number",
        );
        f.min = 0;
        f.max = 300;
        f.required = true;
        resourceFields[resource.id] = f;
      }
      const controls = n("div", undefined, "action-row"),
        preview = n("section", undefined, "galaxy-preview");
      let snapshot = null,
        dirty = 0;
      const values = () => ({
        id: d.id,
        create: !!d.create,
        shape: d.shape,
        seed: d.seed,
        name: name.value,
        systems: Number(count.value),
        settings: {
          ...d.settings,
          ...Object.fromEntries(
            Object.entries(fields).map(([id, f]) => [id, Number(f.value)]),
          ),
          resource_weights: {
            ...weights,
            ...Object.fromEntries(
              Object.entries(resourceFields).map(([id, f]) => [
                id,
                Number(f.value),
              ]),
            ),
          },
        },
      });
      const apply = UI.button(
        "Применить изменения",
        async () => {
          if (!snapshot || !form.reportValidity()) return;
          const body = { ...values(), preview: snapshot.preview };
          await api("admin/galaxies/apply", body);
          if (epoch !== authEpoch || !admin()) return;
          selected = d.id;
          draft = null;
          notify("Галактика обновлена");
          window.MapData = await api("galaxy");
          window.dispatchEvent(new Event("universechange"));
          await refresh(true);
          await GalaxyAdmin.render(root);
        },
        true,
      );
      // UI.button restores its initial disabled state; preview explicitly controls validity.
      const previewButton = UI.button("Предпросмотр", async () => {
        if (!form.reportValidity()) return;
        const current = dirty,
          proposal = await api("admin/galaxies/preview", values());
        if (
          current !== dirty ||
          key !== request ||
          epoch !== authEpoch ||
          !admin()
        )
          return;
        snapshot = proposal;
        preview.replaceChildren(n("h3", tr("Предпросмотр")));
        preview.append(
          UI.table(
            ["Параметр", "Сейчас", "После изменения"],
            Object.entries(labels).map(([id, label]) => [
              tr(label),
              proposal.before[id],
              proposal.after[id],
            ]),
          ),
        );
        const canvas = n("canvas");
        canvas.width = 1000;
        canvas.height = 560;
        canvas.className = "galaxy-preview-map";
        preview.append(canvas);
        const ctx = canvas.getContext("2d"),
          scale = 250 / proposal.radius,
          points = new Map(proposal.stars.map((s) => [s.id, s]));
        for (const area of proposal.territories || []) {
          const center = points.get(area.anchor),
            owner = window.MapData?.founders?.find(
              (c) => c.founder_key === area.founder,
            );
          if (!center || !owner) continue;
          ctx.beginPath();
          ctx.arc(
            500 + center.x * scale,
            280 + center.y * scale,
            area.radius * scale,
            0,
            Math.PI * 2,
          );
          ctx.fillStyle = owner.color;
          ctx.globalAlpha = 0.1;
          ctx.fill();
          ctx.strokeStyle = owner.color;
          ctx.globalAlpha = 0.3;
          ctx.stroke();
        }
        ctx.globalAlpha = 1;
        ctx.strokeStyle = "#324657";
        ctx.lineWidth = 0.55;
        ctx.beginPath();
        for (const lane of proposal.lanes) {
          const a = points.get(lane.from),
            b = points.get(lane.to);
          ctx.moveTo(500 + a.x * scale, 280 + a.y * scale);
          ctx.lineTo(500 + b.x * scale, 280 + b.y * scale);
        }
        ctx.stroke();
        for (const star of proposal.stars) {
          ctx.fillStyle =
            star.id > (window.MapData?.stars.length || 0)
              ? "#efad58"
              : "#9db4c4";
          ctx.beginPath();
          ctx.arc(
            500 + star.x * scale,
            280 + star.y * scale,
            1.45,
            0,
            Math.PI * 2,
          );
          ctx.fill();
        }
        preview.append(
          n(
            "p",
            tr(
              "Янтарные точки — новые системы. Предпросмотр не меняет игровой мир.",
            ),
            "muted",
          ),
        );
        preview.append(
          UI.table(
            ["Ресурс", "Систем с ресурсом до", "После изменения"],
            gameData.expedition.resources
              .filter((r) => resourceFields[r.id])
              .map((r) => [
                tr(r.name),
                proposal.before.resources[r.id] || 0,
                proposal.after.resources[r.id] || 0,
              ]),
          ),
        );
        apply.disabled = false;
      });
      form.oninput = () => {
        dirty++;
        snapshot = null;
        apply.disabled = true;
        preview.replaceChildren(
          n(
            "p",
            tr("Параметры изменены — сформируйте предпросмотр заново."),
            "muted",
          ),
        );
        form.dataset.dirty = "true";
      };
      controls.append(previewButton, apply);
      form.append(controls);
      editor.append(preview);
    },
    async logs(root) {
      if (!admin()) {
        root.replaceChildren();
        return false;
      }
      root.replaceChildren();
      const epoch = authEpoch,
        bar = n("div", undefined, "admin-log-toolbar"),
        search = n("input"),
        rows = n("section");
      search.type = "search";
      search.placeholder = tr("Поиск по администратору, действию или данным");
      search.maxLength = 200;
      search.id = "admin-log-search";
      let page = 1,
        action = "",
        timer;
      const picker = UI.picker(
        [["", "Все действия"], ...Object.entries(actions)],
        "",
        (value) => {
          action = value;
          page = 1;
          load();
        },
      );
      const reload = UI.button("Обновить журнал", () => load());
      bar.append(search, picker, reload);
      root.append(bar, rows);
      const load = async () => {
        const key = ++logRequest;
        try {
          const result = await api(
            "admin/changes?" +
              new URLSearchParams({ page, search: search.value, action }),
          );
          if (
            key !== logRequest ||
            epoch !== authEpoch ||
            !admin() ||
            !Modules.isOpen("adminchanges")
          )
            return;
          rows.replaceChildren();
          rows.append(n("p", tr("Записей") + ": " + result.total, "muted"));
          for (const item of result.items) {
            const entry = n("details", undefined, "admin-log-entry"),
              summary = n("summary"),
              date = new Date(item.created_at).toLocaleString(I18n.language);
            summary.append(
              n("time", date),
              n("b", item.actor_name),
              n("span", tr(actions[item.action] || item.action)),
            );
            entry.append(summary);
            const data = item.data || {};
            if (item.action === "galaxy_update") {
              UI.row(entry, "Галактика", data.name || data.galaxy);
              entry.append(
                UI.table(
                  ["Параметр", "До изменения", "После изменения"],
                  Object.entries(labels).map(([key, label]) => [
                    tr(label),
                    data.before?.[key] ?? "—",
                    data.after?.[key] ?? "—",
                  ]),
                ),
              );
            } else if (item.action === "teleport") {
              UI.row(entry, "Корабль", "#" + data.ship);
              for (const [label, id] of [
                ["Отправление", data.from],
                ["Назначение", data.to],
              ]) {
                const name = window.MapData?.stars[id - 1]?.name || "#" + id;
                UI.row(
                  entry,
                  label,
                  UI.button(name, () => {
                    Modules.open("galaxy");
                    window.dispatchEvent(
                      new CustomEvent("showstar", { detail: id }),
                    );
                  }),
                );
              }
            } else if (item.action === "user_edit") {
              for (const [key, label] of [
                ["user", "Игрок"],
                ["role", "Роль"],
                ["credits", "Кредиты"],
                ["level", "Стаж игрока"],
                ["xp", "Опыт"],
              ])
                UI.row(entry, label, data[key] ?? "—");
            } else if (item.action === "tick")
              UI.row(entry, "Номер фазы", data.tick);
            const full = n("details");
            full.append(
              n("summary", tr("Полные данные")),
              n("pre", JSON.stringify(data, null, 2)),
            );
            entry.append(full);
            rows.append(entry);
          }
          if (!result.items.length) UI.empty(rows, "Записей нет");
          const pager = n("div", undefined, "action-row");
          pager.append(
            UI.button(
              "Назад",
              () => {
                page--;
                return load();
              },
              result.page <= 1,
            ),
            n("span", `${result.page} / ${result.pages}`),
            UI.button(
              "Далее",
              () => {
                page++;
                return load();
              },
              result.page >= result.pages,
            ),
          );
          rows.append(pager);
        } catch (error) {
          notify(error.message, "error");
        }
      };
      search.oninput = () => {
        clearTimeout(timer);
        timer = setTimeout(() => {
          page = 1;
          load();
        }, 250);
      };
      await load();
    },
  };
  window.addEventListener("moduleclose", (event) => {
    if (event.detail === "galaxies") request++;
    if (event.detail === "adminchanges") logRequest++;
  });
  window.addEventListener("signedout", () => {
    request++;
    logRequest++;
  });
})();
