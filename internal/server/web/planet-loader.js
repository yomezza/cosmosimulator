import { notify } from "./ui/feedback.js";
import { UI } from "./ui/runtime.js";
import { scenes } from "./features/runtime.js";

export function initPlanetLoader() {

  let loading;
  UI.loadPlanet = () => (loading ||= import("./planet.js").then((module) => { module.initPlanet(); return scenes.planet; }).catch((error) => { loading=null; throw error; }));
  UI.openPlanet = (star, planet) =>
    UI.loadPlanet()
      .then((p) => p.open(star, planet))
      .catch((e) => notify(e.message, "error"));
  UI.adminPlanetSurface = (star, planet, action) =>
    UI.loadPlanet()
      .then((p) => p.admin(star, planet, action))
      .catch((e) => notify(e.message, "error"));
  document.addEventListener(
    "click",
    (event) => {
      if (
        event.target.closest(
          '#sidebar [data-open="galaxy"],#current-system-map',
        )
      )
        scenes.planet?.dismiss(false);
    },
    true,
  );
  document.getElementById("nav-colonies").onclick = () =>
    UI.loadPlanet()
      .then((p) => p.manager())
      .catch((e) => notify(e.message, "error"));

}
