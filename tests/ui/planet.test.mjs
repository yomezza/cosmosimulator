import assert from "node:assert/strict";
import { test } from "node:test";
import { game } from "./harness.mjs";
import { planetFixture, planetState } from "./planet-fixture.mjs";

test("planet scenes load public art lazily, reuse six textures and keep geography intact", async (t) => {
  const g = await game(t, { signedIn: true });
  g.data["planet/surface"] = planetFixture();
  g.data["planet/state"] = planetState();
  await g.ready();
  assert.equal(
    g.assets.some(
      (p) => p.endsWith("/planet/index.js") || p.includes("surface-ground-"),
    ),
    false,
  );
  const before = JSON.stringify(g.data["planet/surface"].geography);
  for (const kind of [
    "Садовый мир",
    "Ледяная",
    "Пустынная",
    "Вулканическая",
    "Каменистая",
  ]) {
    g.data["planet/surface"].kind = kind;
    await g.page.evaluate(async () => {
      const { openPlanet } = await import("/features/planet/loader.js");
      await openPlanet(1, 1);
    });
    await g.page.waitForSelector('#planet-map[data-textures="ready"]');
  }
  const png = g.assets.filter((p) => p.includes("surface-ground-"));
  assert.equal(png.length, 6);
  assert.equal(new Set(png).size, 6);
  assert.equal(JSON.stringify(g.data["planet/surface"].geography), before);
  assert.equal(
    g.requests.some((r) => r.method === "POST"),
    false,
  );
  await g.page.evaluate(() => I18n.set("en"));
  await g.page.waitForSelector('#planet-map[data-textures="ready"]');
  assert.equal(g.assets.filter((p) => p.includes("surface-ground-")).length, 6);
});

test("texture failure falls back and retries without replacing geography", async (t) => {
  const g = await game(t, { signedIn: true });
  g.data["planet/surface"] = planetFixture();
  g.data["planet/state"] = planetState();
  await g.ready();
  await g.context.route("**/surface-ground-*.png", (r) => r.abort());
  await g.page.evaluate(async () => {
    const { openPlanet } = await import("/features/planet/loader.js");
    await openPlanet(1, 1);
  });
  await g.page.waitForSelector('#planet-map[data-textures="partial"]');
  await g.context.unroute("**/surface-ground-*.png");
  await g.page
    .getByRole("button", { name: "Повторить загрузку текстур", exact: true })
    .click();
  await g.page.waitForSelector('#planet-map[data-textures="ready"]');
  await g.page.evaluate(() => showAuth());
  await g.page.waitForFunction(() => !document.querySelector(".planet-ui"));
  assert.equal(await g.page.locator("#planet-map").count(), 0);
});

test("late images cannot resurrect a scene after logout", async (t) => {
  const g = await game(t, { signedIn: true });
  g.data["planet/surface"] = planetFixture();
  g.data["planet/state"] = planetState();
  await g.ready();
  let release, received;
  const gate = new Promise((r) => (release = r)),
    allRequested = new Promise((r) => (received = r)),
    requests = [];
  await g.context.route("**/surface-ground-*.png", async (route) => {
    requests.push(route.request());
    if (requests.length === 5) received();
    await gate;
    await route.continue();
  });
  try {
    await g.page.evaluate(async () => {
      const { openPlanet } = await import("/features/planet/loader.js");
      await openPlanet(1, 1);
    });
    await allRequested;
    await g.page.evaluate(() => showAuth());
    await g.page.waitForFunction(() => !document.querySelector(".planet-ui"));
    release();
    await Promise.all(
      requests.map(async (req) => (await req.response())?.finished()),
    );
    await g.page.evaluate(
      () =>
        new Promise((r) =>
          requestAnimationFrame(() => requestAnimationFrame(r)),
        ),
    );
    assert.equal(await g.page.locator(".planet-ui,#planet-map").count(), 0);
  } finally {
    release();
  }
});
