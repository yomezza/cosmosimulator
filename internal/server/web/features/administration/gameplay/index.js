import { Session, RequestScope } from "../../../core/session.js";
import { scopedView } from "../../../core/scoped-view.js";
import { I18n } from "../../../core/i18n.js";
import { Format } from "../../../core/format.js";
import { api } from "../../../core/api.js";
import { refresh } from "../../../core/snapshot-store.js";
import { UI, Modules } from "../../../ui/runtime.js";
import { notify } from "../../../ui/feedback.js";
import { Game, GalaxyAdmin, Scheduler, AdminResources, Logistics, scenes } from "../../../features/runtime.js";
import { world, loadWorld } from "../../../core/world-store.js";
import { performExpedition } from "../../../features/commands.js";
import { d, w, res, num, station, fmtTime, money, known, fly, cargoCapacity, fuelCapacity } from "../../../features/selectors.js";
import { reset } from "../../../ui/view-state.js";
import { actionAmount } from "../../../ui/quantity.js";
import { resourceName } from "../../../features/inventory/resource-label.js";
import { can } from "../../../features/corporations/permissions.js";

export function createAdministrationGameplay({ roots, act, command }) {
  const n=UI.node, tr=(s)=>I18n.t(s), b=UI.button, row=UI.row, field=UI.field, choice=UI.choice;
const gameplayScope = new RequestScope(() => Session.signedIn && Session.data.state?.user.role === "admin" && Modules.isOpen("gameplay"));
async function drawrenderGameplay(lease) {
    const result = await api("admin/gameplay"),
      root = roots.gameplay;
    if (!lease.current()) return false;
    reset(root);
    const form = n("form"),
      fields = n("div", undefined, "settings-fields"),
      g = result.settings,
      inputs = {};
    const specs = [
      [
        "tick_seconds",
        "Длительность цикла, секунд",
        "Реальные секунды между фазами. Влияет на добычу и производство.",
      ],
      [
        "date_step",
        "Импульсов даты за цикл",
        "Скорость звёздной даты. Не меняет реальную длительность перелётов.",
      ],
      [
        "quota_common",
        "Общий лимит объекта",
        "Общий объём добычи на объекте для всех игроков и ресурсов.",
      ],
      [
        "reset_seconds",
        "Период восстановления, часов",
        "Реальные часы восстановления. Не зависит от звёздной даты.",
      ],
      [
        "rare_min",
        "Минимум редкого ресурса",
        "Редкая квота входит в общий лимит объекта.",
      ],
      [
        "rare_max",
        "Максимум редкого ресурса",
        "Редкая квота входит в общий лимит объекта.",
      ],
      [
        "scan_seconds",
        "Сканирование объекта, секунд",
        "Время сканирования одного объекта.",
      ],
      [
        "company_cost",
        "Создание корпорации, CR",
        "Стоимость регистрации новой корпорации.",
      ],
      [
        "rescue_cost",
        "Эвакуация, CR",
        "Стоимость буксировки к ближайшему доступному порту.",
      ],
      [
        "reservation_hours",
        "Хранение брони, часов",
        "Полный возврат при отмене или истечении срока.",
      ],
    ];
    for (const [key, label, help] of specs) {
      const i = field(
        fields,
        label,
        "setting-" + key,
        key === "reset_seconds" ? g[key] / 3600 : g[key],
        "number",
        help,
      );
      i.required = true;
      if (key === "reset_seconds") i.step = "any";
      inputs[key] = i;
    }
    form.append(
      fields,
      n(
        "p",
        tr(
          "Новые лимиты и период применяются после завершения текущего периода. По умолчанию восстановление — 5 часов.",
        ),
        "muted",
      ),
    );
    const values = () =>
      Object.fromEntries(
        Object.entries(inputs).map(([key, i]) => [
          key,
          key === "reset_seconds"
            ? Math.round(Number(i.value) * 3600)
            : Number(i.value),
        ]),
      );
    UI.dirty(
      form,
      () => act("admin/gameplay", values()),
      () => renderGameplay(),
    );
    root.append(
      form,
      b("Настройки колонизации", () =>
        UI.loadPlanet().then((p) => {
          const r = Modules.create(
            "colony-settings",
            "Настройки колонизации",
            700,
            720,
          );
          r.replaceChildren();
          p.settings(r).then((ok) => {
            if (ok && Session.signedIn && Session.data.state.user.role === "admin")
              Modules.open("colony-settings");
          });
        }),
      ),
      b("Восстановить квоты сейчас", () =>
        UI.confirm(
          "Восстановление квот",
          tr(
            "Обнулить использованную квоту всех объектов? Незавершённые задания сохранятся.",
          ),
          () => act("admin/gameplay", { ...values(), reset_now: true }),
        ),
      ),
    );
  }
const renderGameplay=scopedView(gameplayScope,drawrenderGameplay,(error)=>notify(error.message,"error"));
function clear() {gameplayScope.invalidate();}
return {renderGameplay,clear, close(){renderGameplay.invalidate();}};
}
