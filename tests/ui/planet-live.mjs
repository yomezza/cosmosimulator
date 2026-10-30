// Runs only against TestPlanetBrowser180's nonce-marked disposable server.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { chromium } from "playwright";
const base = new URL(process.env.TERMINAL_TEST_URL);
assert.ok(["127.0.0.1", "localhost"].includes(base.hostname));
const check = await fetch(base);
assert.ok(process.env.TERMINAL_TEST_NONCE?.startsWith("terminal-test-"));
assert.equal(
  check.headers.get("x-cosmos-test-run"),
  process.env.TERMINAL_TEST_NONCE,
);
const input = JSON.parse(readFileSync(0, "utf8"));
const browser = await chromium.launch({
  ...(process.env.CHROMIUM_EXECUTABLE_PATH
    ? { executablePath: process.env.CHROMIUM_EXECUTABLE_PATH }
    : {}),
  ...(process.env.CHROMIUM_NO_SANDBOX === "1"
    ? { args: ["--no-sandbox"] }
    : {}),
});
try {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
  });
  await context.addCookies([
    { name: "session", value: input.cookie, url: base.href },
  ]);
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  const errors = [],
    artRequests = [];
  page.on("request", (r) => {
    if (new URL(r.url()).pathname.startsWith("/planet-art/"))
      artRequests.push(r.url());
  });
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => {
    if (m.type() !== "log") console.log("Browser:", m.type(), m.text());
  });
  page.on("response", async (r) => {
    if (r.status() >= 400)
      console.log(
        "HTTP failure:",
        new URL(r.url()).pathname,
        r.status(),
        (await r.text()).slice(0, 300),
      );
  });
  await page.goto(base.href);
  await page.locator("#game").waitFor({ state: "visible" });
  await page.locator("#startup-screen").waitFor({ state: "hidden" });
  assert.equal(artRequests.length, 0);
  await page.evaluate(async (site) => {
    const { openVisit } = await import("/features/planet/visit.js");
    await openVisit(site);
  }, input.site);
  const view = page.locator("#window-planet-visit");
  try {
    await view
      .locator('canvas[data-ready="true"]')
      .waitFor({ state: "visible" });
  } catch (e) {
    console.log("Preview status:", await view.textContent());
    console.log("Page errors:", errors);
    console.log(
      "Alerts:",
      await page.locator("[role=alert],.toast").allTextContents(),
    );
    throw e;
  }
  assert.equal(
    await view
      .locator("canvas")
      .evaluate(
        (c) =>
          c
            .getContext("2d")
            .getImageData(
              Math.floor(c.width / 2),
              Math.floor(c.height / 2),
              1,
              1,
            ).data[3],
      ),
    255,
  );
  const canvas = view.locator("canvas");
  const start = await canvas.evaluate((c) => ({
    x: Number(c.dataset.xq),
    y: Number(c.dataset.yq),
  }));
  const box = await canvas.boundingBox();
  await canvas.click({
    position: { x: box.width / 2 + 32, y: box.height / 2 },
  });
  await page.waitForFunction(
    (x) =>
      Number(document.querySelector(".visit-canvas")?.dataset.xq) > x + 1024,
    start.x,
  );
  const goalX = (Math.floor(start.x / 1024) + 2) * 1024 + 512;
  await page.waitForFunction(
    (x) =>
      Math.abs(
        Number(document.querySelector(".visit-canvas")?.dataset.xq) - x,
      ) < 3,
    goalX,
  );
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.wheel(0, -250);
  await page.waitForFunction(
    () => Number(document.querySelector(".visit-canvas")?.dataset.zoom) > 16,
  );
  assert.ok(
    Math.abs((await canvas.evaluate((c) => Number(c.dataset.xq))) - goalX) < 3,
    "zoom changed the character position",
  );
  if (process.env.PLANET_TEST_SCREENSHOT)
    await view.screenshot({ path: process.env.PLANET_TEST_SCREENSHOT });
  assert.ok(artRequests.length >= 2, "scene art was not requested lazily");
  const first = await page.evaluate(async () => {
    const { api } = await import("/core/api.js");
    return api("planet/visit");
  });
  assert.equal(first.state, "active");
  await page.reload();
  await page.locator("#game").waitFor({ state: "visible" });
  await page.locator("#startup-screen").waitFor({ state: "hidden" });
  await page.evaluate(async () => {
    const { openVisit } = await import("/features/planet/visit.js");
    await openVisit();
  });
  await view.locator('canvas[data-ready="true"]').waitFor({ state: "visible" });
  const second = await page.evaluate(async () => {
    const { api } = await import("/core/api.js");
    return api("planet/visit");
  });
  assert.equal(second.visit, first.visit);
  await page.waitForFunction(
    (x) =>
      Math.abs(
        Number(document.querySelector(".visit-canvas")?.dataset.xq) - x,
      ) < 3,
    goalX,
  );
  await view
    .getByRole("button", { name: "Вернуться в космос", exact: true })
    .click();
  await view.getByText("Возвращение завершено", { exact: true }).waitFor();
  assert.deepEqual(errors, []);
  console.log(
    "PASS: browser -> cosmic proxy -> planetary WS; click movement, zoom and sprites render; reload resumes same visit/position, return completes; no page errors",
  );
  await context.close();
} finally {
  await browser.close();
}
