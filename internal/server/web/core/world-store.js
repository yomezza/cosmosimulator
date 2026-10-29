import { api } from "./api.js";
import { Session } from "./session.js";

export const world = { data: null, stars: new Map(), stations: new Map() };
let inFlight;
export function acceptWorld(data) {
  world.data = data;
  world.stars = new Map((data?.stars || []).map((s) => [s.id, s]));
  return data;
}
// Explicitly opt in only the galaxy metadata endpoint, never all GETs.
export function loadWorld() {
  if (world.data) return Promise.resolve(world.data);
  const epoch = Session.epoch;
  if (inFlight?.epoch === epoch) return inFlight.promise;
  const entry = { epoch };
  entry.promise = api("galaxy").then((data) => {
    if (epoch === Session.epoch && Session.signedIn) acceptWorld(data);
    return data;
  }).finally(() => { if (inFlight === entry) inFlight = null; });
  inFlight = entry;
  return entry.promise;
}
window.addEventListener("signedout", () => { acceptWorld(null); inFlight = null; });
