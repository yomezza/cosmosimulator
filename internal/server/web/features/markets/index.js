import { Session, RequestScope } from "../../core/session.js";
import { scopedView } from "../../core/scoped-view.js";
import { I18n } from "../../core/i18n.js";
import { Format } from "../../core/format.js";
import { api } from "../../core/api.js";
import { refresh } from "../../core/snapshot-store.js";
import { UI, Modules } from "../../ui/runtime.js";
import { notify } from "../../ui/feedback.js";
import { Game, GalaxyAdmin, Scheduler, AdminResources, Logistics, scenes } from "../../features/runtime.js";
import { world, loadWorld } from "../../core/world-store.js";
import { performExpedition } from "../../features/commands.js";
import { d, w, res, num, station, fmtTime, money, known, fly, cargoCapacity, fuelCapacity } from "../../features/selectors.js";
import { reset } from "../../ui/view-state.js";
import { actionAmount } from "../../ui/quantity.js";
import { resourceName } from "../../features/inventory/resource-label.js";
import { can } from "../../features/corporations/permissions.js";

export function createMarkets({ roots, act, command, renderShipyards }) {
  const n=UI.node, tr=(s)=>I18n.t(s), b=UI.button, row=UI.row, field=UI.field, choice=UI.choice;
let marketTab = "local";
let selectedResource = "agni";
let selectedOrder = null;
let marketRequestID = 0;
const renderMarketScope = new RequestScope(() => Session.signedIn && Modules.isOpen("market"));
function tradeForm(root, offer, global = false, starID, stationID) {
    const r = res(offer.resource),
      panel = n("aside", undefined, "detail-panel");
    panel.append(n("h3", tr(r.name)), n("p", tr(r.description), "muted"));
    const q = field(panel, "Количество", "trade-quantity", 1, "number");
    q.min = 1;
    q.max = Math.min(global ? 240 : cargoCapacity(), offer.stock);
    const quote = n("p", undefined, "trade-total"),
      capacity = n("p", undefined, "muted");
    const buy = b(global ? "Забронировать на станции" : "Купить в трюм", () =>
      global
        ? act("market/global", {
            action: "reserve",
            star: starID,
            station: stationID,
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
        : tr("Свободно в трюме") + ": " + (cargoCapacity() - used);
      buy.disabled =
        !Number.isInteger(count) ||
        count < 1 ||
        count > offer.stock ||
        count > (global ? 240 : cargoCapacity()) ||
        cost > d().credits ||
        (!global && (!d().docked || count + used > cargoCapacity()));
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
            Session.data.clock.settings.reservation_hours +
            " h",
          "muted",
        ),
      );
    root.append(panel);
    update();
  }
async function drawrenderMarket(lease) {
    const root = roots.market,
      key = ++marketRequestID,
      epoch = Session.epoch,
      tab = marketTab,
      current = () =>
        key === marketRequestID &&
        epoch === Session.epoch &&
        Session.signedIn &&
        Modules.isOpen("market") &&
        tab === marketTab;
    let globalMarket;
    if (tab === "global") {
      try {
        globalMarket = await api("market/global");
      } catch (error) {
        if (!current()) return false;
        throw error;
      }
    }
    // Keep the current table while loading; only the latest render may replace it.
    if (!current()) return false;
    Modules.title("market", tr(tab === "shipyards" ? "Верфи" : "Рынок"));
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

        ["data", "Разведданные"],
        ["delivery", "Перевозки"],
        ["shipyards", "Верфи"],
      ],
      tab,
      (id) => {
        marketTab = id;
        renderMarket();
      },
    );
    if (tab === "shipyards") {
      renderShipyards(root);
      return;
    }
    if (tab === "local") {
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
    if (tab === "global") {
      const result = globalMarket;
      const eligible = result.ports.sort((a, b) => a.star - b.star);
      const portKey = (p) => p.star + "|" + p.station;
      let target =
        root.dataset.target || d().ship.star + "|" + d().ship.object_id;
      if (!eligible.some((p) => portKey(p) === target))
        target = eligible[0] && portKey(eligible[0]);
      if (!target) {
        UI.empty(root, "Нет доступных станций");
        return;
      }
      const stationLabel = n("label");
      stationLabel.append(n("span", tr("Станция получения")));
      root.append(stationLabel);
      UI.stationPicker(stationLabel, eligible, target, (value) => {
        root.dataset.target = value;
        renderMarket();
      });
      const targetPort = eligible.find((p) => portKey(p) === target);
      root.append(
        n(
          "p",
          tr(targetPort.specialization) +
            " · " +
            tr("Ассортимент зависит от станции"),
          "muted",
        ),
      );
      const layout = n("div", undefined, "master-detail"),
        list = n("section");
      layout.append(list);
      const offers = targetPort.offers || [],
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
      if (offer)
        tradeForm(layout, offer, true, targetPort.star, targetPort.station);
      root.append(layout, n("h3", tr("Мои бронирования")));
      const rows = result.reservations.map((r) => {
        const controls = n("div", undefined, "inline-controls");
        if (r.status === "reserved")
          controls.append(
            b(
              "Получить груз",
              () => act("market/global", { action: "collect", id: r.id }),
              !d().docked ||
                d().ship.star !== r.star ||
                d().ship.object_id !== r.station,
              "Прибудьте на станцию получения",
            ),
            b("Отменить бронь", () =>
              act("market/global", { action: "cancel", id: r.id }),
            ),
          );
        return [
          "#" + r.id,
          UI.stationRef(r.star, r.station),
          resourceName(r.resource),
          "× " + r.quantity,
          money(r.cost),
          n("span", tr(r.status), "reservation-status status-" + r.status),
          r.expires_at ? fmtTime(r.expires_at) : "—",
          controls,
        ];
      });
      root.append(
        UI.table(
          [
            "ID",
            "Станция",
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
    if (tab === "warehouse") {
      UI.empty(root, "Управление запасами перенесено в логистику");
      root.append(b("Открыть логистику", () => Modules.open("logistics")));
    }
    if (tab === "orders") {
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
            UI.stationRef(o.star, o.station),
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
          UI.stationRef(selected.star, selected.station),
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
            selected.seller === Session.data.state.user.id,
          ),
        );
        if (selected.seller === Session.data.state.user.id)
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
    if (tab === "data") {
      const list = n("section");
      for (const offer of w().data_offers || []) {
        const card = n("article", undefined, "data-offer");
        card.append(
          n(
            "h3",
            tr(
              world.data?.regions.find((r) => r.id === offer.region)
                ?.name || offer.region,
            ),
          ),
          n(
            "p",
            offer.seller_name +
              " · " +
              (w().stations.find(
                (p) => p.star === offer.star && p.object_id === offer.station,
              )?.name || offer.star + " / " + offer.station),
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
                  world.data?.stars.find((s) => s.id === id)?.name || id,
              )
              .join(" · "),
          ),
        );
        card.append(
          details,
          n("p", money(offer.price)),
          b(
            offer.seller === Session.data.state.user.id
              ? "Снять предложение"
              : "Купить разведданные",
            () =>
              command({
                action:
                  offer.seller === Session.data.state.user.id
                    ? "data_remove"
                    : "data_buy",
                id: offer.id,
              }),
            offer.seller !== Session.data.state.user.id &&
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
          (world.data?.regions || []).map((r) => [r.id, r.name]),
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
    if (tab === "delivery") {
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
            own = job.owner === Session.data.state.user.id;
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
            job.carrier === Session.data.state.user.id
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
    row(root, "Отправление", UI.stationRef(job.from_star, job.from_station));
    row(root, "Назначение", UI.stationRef(job.to_star, job.to_station));
    row(root, "Груз", tr(res(job.resource).name) + " × " + job.quantity);
    row(root, "Вознаграждение", money(job.reward));
    row(root, "Обеспечение", money(job.collateral));
    row(root, "Срок", fmtTime(job.deadline));
    row(root, "Исполнитель", job.carrier_name || tr("Не назначен"));
    if (job.arrival) row(root, "Прибытие", fmtTime(job.arrival));
    Modules.open("delivery-info");
  }
const renderMarket=scopedView(renderMarketScope,drawrenderMarket,(error)=>notify(error.message,"error"));
function clear() {selectedResource="agni";selectedOrder=null;marketRequestID=0;renderMarketScope.invalidate();}
return {tradeForm,renderMarket,renderDeliveries,deliveryDetails,clear, show(tab){marketTab=tab;marketRequestID++;}, close(){marketRequestID++;renderMarket.invalidate();}, close(){renderMarket.invalidate();}};
}
