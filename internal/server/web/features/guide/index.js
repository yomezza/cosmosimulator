import { objectHologram } from "../objects/presentation.js";
import { FormState } from "../../ui/form-state.js";
import { Session } from "../../core/session.js";

import { I18n } from "../../core/i18n.js";

import { UI, Modules } from "../../ui/runtime.js";

import { Game } from "../../features/runtime.js";

import { d, w } from "../../features/selectors.js";
import { reset } from "../../ui/view-state.js";

export function createGuide({
  roots,
  act,
  command,
  shipStats,
  filterShipyards,
}) {
  const drafts = new FormState(() => Session.data.state?.user.id);
  const n = UI.node,
    tr = (s) => I18n.t(s),
    b = UI.button,
    row = UI.row,
    field = (...args) => drafts.field(UI.field, ...args);
  let guideID = "iron";
  let guideQuery = "";
  let objectArticles = [
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
      search = field(left, "Поиск", "guide-search", guideQuery, "search"),
      expandedCategories = new Set();
    let previousQuery = search.value.toLowerCase().trim();
    const all = [
      {
        id: "experience",
        name: "Стаж и размер флота",
        category: "Техническая информация",
        subcategory: "Развитие игрока",
        technical: true,
        desc: "",
      },
      ...d().resources.map((r) => ({
        id: r.id,
        name: r.name,
        code: r.code,
        desc: r.description,
        resource: r,
        category: "Ресурсы",
        subcategory: r.kind,
      })),
      ...(w().ship_classes || []).map((ship) => ({
        id: "ship-" + ship.id,
        name: ship.name,
        desc: ship.description,
        category: "Корабли",
        subcategory: ship.role,
        ship,
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
      const previousList = left.querySelector(".guide-list");
      if (!previousQuery) {
        previousList?.querySelectorAll(".guide-category").forEach((group) => {
          if (group.open) expandedCategories.add(group.dataset.category);
          else expandedCategories.delete(group.dataset.category);
        });
      }
      previousList?.remove();
      const list = n("div", undefined, "guide-list"),
        q = search.value.toLowerCase().trim();
      previousQuery = q;
      const matched = all.filter((item) =>
        [
          tr(item.name),
          item.code || "",
          tr(item.desc),
          tr(item.category),
          tr(item.subcategory),
        ].some((value) => value.toLowerCase().includes(q)),
      );
      for (const category of [
        "Ресурсы",
        "Космические объекты",
        "Корабли",
        "Техническая информация",
      ]) {
        const entries = matched.filter((item) => item.category === category);
        if (!entries.length) continue;
        const group = n("details", undefined, "guide-category");
        group.dataset.category = category;
        group.dataset.fold = "guide:" + category;
        group.open = !!q || expandedCategories.has(category);
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
      if (item.technical) {
        article.append(
          n(
            "p",
            tr(
              "Стаж не ограничен сотым уровнем. Лимит флота определяет число принадлежащих игроку кораблей.",
            ),
          ),
          UI.table(
            ["Стаж игрока", "Кораблей"],
            [
              ["1–5", "2"],
              ["6–10", "3"],
              ["11–16", "4"],
              ["17–25", "6"],
              ["26–35", "7"],
              ["36+", "7"],
            ],
          ),
          n(
            "p",
            tr(
              "После стажа 35 сохраняется лимит 7 кораблей до появления новых диапазонов. Имеющиеся корабли не изымаются. Ограничения стажа для покупки конкретной модели действуют отдельно.",
            ),
          ),
        );
      } else if (item.ship) {
        const model = UI.hologram(
          item.ship.model,
          tr(item.ship.name),
          "ship-model",
        );
        article.append(model, n("p", tr(item.desc)));
        shipStats(article, item.ship);
        article.append(
          b("Найти верфь", () => {
            filterShipyards({ classID: item.ship.id, role: "all", query: "" });
            Game.openShipyards();
          }),
        );
      } else if (item.resource) {
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
        const canvas = objectHologram({ kind: item.kind });
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
  function clear() {
    drafts.clear();
  }
  window.addEventListener("actionaccepted", () => drafts.clear());
  return {
    renderGuide,
    clear,
    open(id) {
      guideID = id;
      Modules.open("guide");
    },
  };
}
