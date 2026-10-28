"use strict";
const $ = (id) => document.getElementById(id),
  t = (s) => I18n.t(s);
let signedIn = false,
  starting = true,
  pending = false,
  mode = "login",
  authEpoch = 0,
  lastPoll = 0,
  balanceOwner = null,
  previousBalance = null,
  balanceTimer;
window.gameData = {};
async function api(path, body, signal) {
  const r = await fetch("/api/" + path, {
    signal,
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
function startupStatus(failed = false) {
  const title = failed ? "Не удалось подключиться" : "Подключение к серверу",
    message = failed
      ? "Сервер пока не отвечает. Проверьте соединение и повторите попытку."
      : "Проверяем сессию и загружаем данные пилота…";
  $("startup-title").dataset.i18n = title;
  $("startup-title").textContent = t(title);
  $("startup-message").dataset.i18n = message;
  $("startup-message").textContent = t(message);
  $("startup-screen").classList.toggle("startup-failed", failed);
  $("startup-retry").hidden = !failed;
}
function finishStartup() {
  starting = false;
  $("startup-screen").hidden = true;
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
  balanceOwner = null;
  previousBalance = null;
  clearTimeout(balanceTimer);
  $("balance-change")?.replaceChildren();
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
  finishStartup();
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
  const controller = starting ? new AbortController() : null,
    timeout = controller ? setTimeout(() => controller.abort(), 15000) : null;
  pending = true;
  lastPoll = Date.now();
  try {
    const [state, profile, expedition, fleet, clock, workbench, notifications] =
      await Promise.all([
        api("state", undefined, controller?.signal),
        api("profile", undefined, controller?.signal),
        api("expedition", undefined, controller?.signal),
        api("fleet", undefined, controller?.signal),
        api("gameplay", undefined, controller?.signal),
        api("workbench", undefined, controller?.signal),
        api("notifications", undefined, controller?.signal),
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
    const balance = Number(expedition.credits),
      indicator = $("balance-change");
    if (
      balanceOwner === state.user.id &&
      previousBalance !== null &&
      balance !== previousBalance
    ) {
      const delta = balance - previousBalance;
      clearTimeout(balanceTimer);
      const text = document.createElement("span");
      text.className = "balance-delta " + (delta > 0 ? "income" : "expense");
      text.textContent =
        (delta > 0 ? "+" : "-") +
        Math.abs(delta).toLocaleString(I18n.language) +
        " CR";
      indicator.replaceChildren(text);
      balanceTimer = setTimeout(() => indicator.replaceChildren(), 3200);
    } else if (balanceOwner !== state.user.id) {
      clearTimeout(balanceTimer);
      indicator.replaceChildren();
    }
    balanceOwner = state.user.id;
    previousBalance = balance;
    $("credits").textContent = balance.toLocaleString(I18n.language);
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
    finishStartup();
  } catch (e) {
    if (epoch !== authEpoch) return;
    if (e.status === 401) showAuth();
    else {
      $("server-status").textContent = t("Переподключение…");
      if (starting) startupStatus(true);
    }
  } finally {
    clearTimeout(timeout);
    controller?.abort();
    if (epoch === authEpoch) pending = false;
  }
}
$("startup-retry").onclick = () => {
  if (!starting || pending) return;
  startupStatus();
  refresh(true);
};
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
