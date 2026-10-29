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

export function createAdministrationUsers({ roots, act, command }) {
  const n=UI.node, tr=(s)=>I18n.t(s), b=UI.button, row=UI.row, field=UI.field, choice=UI.choice;
let userPage = 1;
let userQuery = "";
const usersScope = new RequestScope(() => Session.signedIn && Session.data.state?.user.role === "admin" && Modules.isOpen("users"));
async function drawrenderUsers(lease) {
    const result = await api(
        "admin/users?page=" + userPage + "&q=" + encodeURIComponent(userQuery),
      ),
      root = roots.users,
      focused = document.activeElement?.id === "user-search",
      caret = document.activeElement?.selectionStart;
    if (!lease.current()) return false;
    reset(root);
    userPage = result.page;
    const search = field(
      root,
      "Поиск по никнейму или ID",
      "user-search",
      userQuery,
      "search",
    );
    let timer;
    search.oninput = () => {
      userQuery = search.value;
      clearTimeout(timer);
      timer = setTimeout(() => {
        userPage = 1;
        renderUsers();
      }, 250);
    };
    search.onkeydown = (e) => {
      if (e.key === "Enter") {
        clearTimeout(timer);
        userQuery = search.value;
        userPage = 1;
        renderUsers();
      }
    };
    const paging = n("div", undefined, "pagination sticky-toolbar");
    paging.append(
      b(
        "‹",
        () => {
          userPage--;
          renderUsers();
        },
        userPage <= 1,
      ),
      n("span", `${userPage} / ${result.pages} · ${num(result.total)}`),
      b(
        "›",
        () => {
          userPage++;
          renderUsers();
        },
        userPage >= result.pages,
      ),
    );
    root.append(
      paging,
      UI.table(
        ["ID", "Игрок", "Роль", "Баланс", "Стаж игрока", "Действие"],
        result.items.map((u) => [
          u.id,
          u.username,
          tr(u.role) + (u.bot ? " · BOT" : ""),
          money(u.credits),
          u.level,
          b("Редактировать", () => editUser(u)),
        ]),
      ),
    );
    if (focused) {
      search.focus();
      search.setSelectionRange(caret, caret);
    }
  }
function editUser(u) {
    const id = "user-" + u.id,
      root = Modules.create(id, "Редактирование пользователя", 550, 640, {transient:true});
    reset(root);
    const form = n("form");
    form.append(n("h3", tr("Основные сведения")));
    const name = field(form, "Позывной", "edit-name-" + u.id, u.username);
    name.disabled = u.bot;
    form.append(n("h3", tr("Игровые показатели")));
    const values = n("div", undefined, "settings-fields"),
      credits = field(
        values,
        "Кредиты",
        "edit-credits-" + u.id,
        u.credits,
        "number",
      ),
      level = field(
        values,
        "Стаж игрока",
        "edit-level-" + u.id,
        u.level,
        "number",
      ),
      xp = field(values, "Опыт", "edit-xp-" + u.id, u.xp, "number");
    credits.min = 0;
    credits.max = Number.MAX_SAFE_INTEGER;
    credits.step = 1;
    level.min = 1;
    level.max = 1000000;
    xp.min = 0;
    xp.max = 1000000000;
    form.append(
      values,
      b("Выдача ресурсов", () => AdminResources.open(u.id)),
      n("h3", tr("Доступ")),
    );
    const role = choice(
      form,
      "Роль",
      [
        ["player", "ИГРОК"],
        ["admin", "АДМИНИСТРАТОР"],
      ],
      u.role,
    );
    role.disabled = u.bot;
    const pass = field(
      form,
      "Новый пароль · пусто — без изменений",
      "edit-password-" + u.id,
      "",
      "password",
    );
    pass.disabled = u.bot;
    pass.autocomplete = "new-password";
    const dirty = UI.dirty(
      form,
      async () => {
        const ok = await act("admin/user", {
          id: u.id,
          username: name.value,
          credits: Number(credits.value),
          level: Number(level.value),
          xp: Number(xp.value),
          role: role.value,
          password: pass.value,
        });
        if (ok) {
          Object.assign(u, {
            username: name.value,
            credits: Number(credits.value),
            level: Number(level.value),
            xp: Number(xp.value),
            role: role.value,
          });
          pass.value = "";
          renderUsers();
          editUser(u);
        }
        return ok;
      },
      () => editUser(u),
    );
    role.onchange = dirty;
    form.addEventListener("input", (e) =>
      e.target.classList.add("field-dirty"),
    );
    root.append(form);
    Modules.open(id);
  }
const renderUsers=scopedView(usersScope,drawrenderUsers,(error)=>notify(error.message,"error"));
function clear() {userPage=1;userQuery="";usersScope.invalidate();}
return {renderUsers,editUser,clear, close(){renderUsers.invalidate();}};
}
