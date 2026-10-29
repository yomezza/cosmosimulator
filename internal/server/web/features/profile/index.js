import { FormState } from "../../ui/form-state.js";
import { Session } from "../../core/session.js";

import { I18n } from "../../core/i18n.js";

import { UI, Modules } from "../../ui/runtime.js";

import { reset } from "../../ui/view-state.js";

export function createProfile({ roots, act, command }) {
  const drafts = new FormState(() => Session.data.state?.user.id);
  const tr = (s) => I18n.t(s),
    b = UI.button,
    row = UI.row;
  function renderProfile() {
    const root = roots.profile;
    reset(root);
    row(root, "Позывной", Session.data.state.user.username);
    row(
      root,
      "Роль",
      tr(Session.data.state.user.role === "admin" ? "АДМИНИСТРАТОР" : "ИГРОК"),
    );
    row(root, "Корпорация", Session.data.profile.company || tr("Без компании"));
    row(root, "Кораблей", Session.data.fleet.ships.length);
    root.append(
      b("История финансов", () => Modules.open("finance")),
      b("Настройки", () => Modules.open("settings")),
    );
  }
  function clear() {
    drafts.clear();
  }
  window.addEventListener("actionaccepted", () => drafts.clear());
  return { renderProfile, clear };
}
