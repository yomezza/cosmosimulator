"use strict";
const $ = (id) => document.getElementById(id),
  t = (s) => I18n.t(s);
let signedIn = false,
  pending = false,
  mode = "login",
  authEpoch = 0,
  lastPoll = 0;
window.gameData = {};
async function api(path, body) {
  const r = await fetch("/api/" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await r.json();
  if (!r.ok) {
    const e = new Error(t(data.error || "Ошибка сервера"));
    e.status = r.status;
    throw e;
  }
  return data;
}
function authMode(value) {
  mode = value;
  $("login-tab").classList.toggle("active", mode === "login");
  $("register-tab").classList.toggle("active", mode === "register");
  $("submit").textContent = t(
    mode === "login" ? "Войти в терминал →" : "Создать аккаунт →",
  );
  $("auth-form").elements.password.minLength = mode === "login" ? 1 : 1;
  $("auth-form").elements.password.autocomplete =
    mode === "login" ? "current-password" : "new-password";
  $("password-hint").textContent = t(
    mode === "login"
      ? "Используйте данные вашей компании."
      : "Имя: латиница, цифры, _ и -.",
  );
}
function showAuth() {
  authEpoch++;
  pending = false;
  signedIn = false;
  for (const menu of document.querySelectorAll(".dropdown")) menu.open = false;
  Modules.closeAll();
  Modules.clearPrivate();
  gameData = {};
  window.dispatchEvent(new Event("signedout"));
  for (const id of [
    "game",
    "sidebar",
    "profile-menu",
    "admin-menu",
    "notifications-button",
  ])
    $(id).hidden = true;
  $("auth").hidden = false;
}
let ongoingRefresh = null;
function refresh(force = false) {
  if (ongoingRefresh)
    return force ? ongoingRefresh.then(() => refresh(false)) : ongoingRefresh;
  const task = refreshData();
  ongoingRefresh = task;
  task.finally(() => {
    if (ongoingRefresh === task) ongoingRefresh = null;
  });
  return task;
}
async function refreshData() {
  if (pending) return;
  const epoch = authEpoch;
  pending = true;
  lastPoll = Date.now();
  try {
    const [state, profile, expedition, fleet, clock, workbench, notifications] =
      await Promise.all([
        api("state"),
        api("profile"),
        api("expedition"),
        api("fleet"),
        api("gameplay"),
        api("workbench"),
        api("notifications"),
      ]);
    if (epoch !== authEpoch) return;
    signedIn = true;
    gameData = {
      state,
      profile,
      expedition,
      fleet,
      clock,
      workbench,
      notifications,
      serverOffset: new Date(clock.server_time).getTime() - Date.now(),
    };
    $("auth").hidden = true;
    for (const id of [
      "game",
      "sidebar",
      "profile-menu",
      "notifications-button",
    ])
      $(id).hidden = false;
    $("admin-menu").hidden = state.user.role !== "admin";
    if (state.user.role !== "admin")
      for (const id of [
        "users",
        "gameplay",
        "benchmark",
        "galaxies",
        "adminchanges",
      ]) {
        Modules.close(id);
        document
          .querySelector("#window-" + id + " .module-content")
          ?.replaceChildren();
      }
    $("profile-nick").textContent = state.user.username;
    $("welcome-name").textContent = state.user.username;
    const company = $("overview-company");
    company.className = profile.company_tag ? "corp-badge" : "muted";
    company.style.color = profile.company_color || "";
    company.style.background = "";
    company.textContent = profile.company_tag
      ? `${profile.company_flag} [${profile.company_tag}]`
      : t("Без компании");
    if (profile.company_tag)
      UI.paintFlag(company, {
        tag: profile.company_tag,
        color: profile.company_color,
        flag: profile.company_flag,
        flag_design: profile.company_flag_design,
      });
    company.dataset.helpRaw =
      profile.company_id === 1
        ? t(profile.company)
        : profile.company || t("Без компании");
    company.dataset.help = company.dataset.helpRaw;
    company.tabIndex = 0;
    $("credits").textContent = expedition.credits.toLocaleString(I18n.language);
    $("star-date").textContent = state.date.replace(
      "ЭД",
      I18n.language === "en" ? "ED" : "ЭД",
    );
    $("location").textContent = expedition.ship.destination
      ? expedition.system.name +
        " → " +
        (window.MapData?.stars[expedition.ship.destination - 1]?.name ||
          String(expedition.ship.destination))
      : expedition.system.name;
    $("overview-region").textContent = t(expedition.region);
    $("overview-level").textContent = profile.level;
    $("overview-xp").max = profile.required_xp;
    $("overview-xp").value = profile.xp;
    $("overview-xp").dataset.help =
      "Опыт даётся за разведку (+10) и завершённый цикл добычи (+5). Стаж принадлежит игроку, не кораблю.";
    $("overview-xp-text").textContent =
      `${profile.xp} / ${profile.required_xp} XP`;
    $("overview-upgrade").disabled =
      profile.xp < profile.required_xp ||
      expedition.credits < profile.upgrade_cost ||
      profile.level >= 1000000;
    $("overview-upgrade").dataset.help = `${profile.upgrade_cost} CR`;
    $("connection").textContent =
      t("Данные обновлены: ") + new Date().toLocaleTimeString(I18n.language);
    $("server-status").textContent = t("● СЕРВЕР ОНЛАЙН");
    window.dispatchEvent(new Event("gamedata"));
  } catch (e) {
    if (epoch !== authEpoch) return;
    if (e.status === 401) showAuth();
    else {
      $("server-status").textContent = t("Переподключение…");
    }
  } finally {
    if (epoch === authEpoch) pending = false;
  }
}
$("login-tab").onclick = () => authMode("login");
$("register-tab").onclick = () => authMode("register");
$("auth-form").onsubmit = async (e) => {
  e.preventDefault();
  authEpoch++;
  pending = false;
  $("submit").disabled = true;
  $("auth-error").textContent = "";
  try {
    await api(mode, {
      username: e.target.elements.username.value,
      password: e.target.elements.password.value,
    });
    e.target.elements.password.value = "";
    await refresh(true);
  } catch (error) {
    $("auth-error").textContent = error.message;
    for (const input of e.target.querySelectorAll("input")) {
      input.setAttribute("aria-invalid", "true");
      input.setAttribute("aria-describedby", "auth-error");
      input.oninput = () => input.removeAttribute("aria-invalid");
    }
  } finally {
    $("submit").disabled = false;
  }
};
$("logout").onclick = async () => {
  authEpoch++;
  pending = false;
  try {
    await api("logout", {});
    showAuth();
  } catch (e) {
    notify(e.message, "error");
  }
};
$("admin-tick").onclick = async () => {
  const b = $("admin-tick");
  b.disabled = true;
  try {
    await api("admin/tick", {});
    await refresh(true);
    notify("Тик выполнен");
  } catch (e) {
    notify(e.message, "error");
  } finally {
    b.disabled = false;
  }
};
$("overview-upgrade").onclick = async () => {
  try {
    await api("profile/upgrade", {});
    await refresh(true);
    notify("Стаж повышен");
  } catch (e) {
    notify(e.message, "error");
  }
};
window.addEventListener("languagechange", () => {
  authMode(mode);
  if (signedIn) refresh();
});
I18n.ready.then(() => {
  I18n.apply();
  authMode("login");
  refresh();
});
setInterval(() => {
  if (
    signedIn &&
    Date.now() - lastPoll >=
      Math.min(
        5000,
        Math.max(1000, (gameData.state?.tick_seconds || 10) * 1000),
      )
  )
    refresh();
}, 1000);
