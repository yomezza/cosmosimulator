import { notify } from "../../ui/feedback.js";

import { scenes } from "../../features/runtime.js";

let loading;
export const loadPlanet = () =>
  (loading ||= import("./index.js")
    .then((module) => {
      module.initPlanet();
      return scenes.planet;
    })
    .catch((error) => {
      loading = null;
      throw error;
    }));
export const openPlanet = (star, planet) =>
  loadPlanet()
    .then((p) => p.open(star, planet))
    .catch((e) => notify(e.message, "error"));
export const adminPlanetSurface = (star, planet, action) =>
  loadPlanet()
    .then((p) => p.admin(star, planet, action))
    .catch((e) => notify(e.message, "error"));
export function initPlanetLoader() {
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
    loadPlanet()
      .then((p) => p.manager())
      .catch((e) => notify(e.message, "error"));
}
