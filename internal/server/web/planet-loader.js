"use strict";
(() => {
  let loading;
  UI.loadPlanet = () =>
    (loading ||= new Promise((resolve, reject) => {
      const script = document.createElement("script");
      script.src = "planet.js";
      script.onload = () => resolve(window.PlanetUI);
      script.onerror = () => {
        loading = null;
        script.remove();
        reject(Error(I18n.t("Не удалось загрузить планетарный интерфейс")));
      };
      document.head.append(script);
    }));
  UI.openPlanet = (star, planet) =>
    UI.loadPlanet()
      .then((p) => p.open(star, planet))
      .catch((e) => notify(e.message, "error"));
  document.addEventListener(
    "click",
    (event) => {
      if (
        event.target.closest(
          '#sidebar [data-open="galaxy"],#current-system-map',
        )
      )
        window.PlanetUI?.dismiss(false);
    },
    true,
  );
  document.getElementById("nav-colonies").onclick = () =>
    UI.loadPlanet()
      .then((p) => p.manager())
      .catch((e) => notify(e.message, "error"));
})();
