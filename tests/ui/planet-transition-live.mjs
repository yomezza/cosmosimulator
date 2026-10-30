// Runs only against a nonce-marked server and disposable DB owned by the Go test.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { chromium } from "playwright";
const base = new URL(process.env.TERMINAL_TEST_URL);
assert.ok(["localhost", "127.0.0.1"].includes(base.hostname));
assert.ok(process.env.TERMINAL_TEST_NONCE?.startsWith("terminal-test-"));
assert.equal(
  (await fetch(base)).headers.get("x-cosmos-test-run"),
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
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(base.href);
  await page.locator("#game").waitFor({ state: "visible" });
  await page.locator("#startup-screen").waitFor({ state: "hidden" });
  const open = () =>
    page.evaluate(async () => {
      const { openVisit } = await import("/features/planet/visit.js");
      await openVisit();
    });
  await open();
  const root = page.locator("#window-planet-visit"),
    canvas = root.locator("canvas");
  await root.getByText("Соседняя локация готова", { exact: true }).waitFor();
  assert.equal(await canvas.getAttribute("data-sector"), "1");
  await root.getByRole("button", { name: "Остаться", exact: true }).click();
  await root.locator(".visit-transition").waitFor({ state: "hidden" });
  assert.equal(await canvas.getAttribute("data-sector"), "1");
  // The fixture is at the portal. Selecting it explicitly re-arms an offer.
  await canvas.click({
    position: {
      x: (await canvas.boundingBox()).width / 2,
      y: (await canvas.boundingBox()).height / 2,
    },
  });
  await root.getByText("Соседняя локация готова", { exact: true }).waitFor();
  await root
    .getByRole("button", { name: "Перейти в соседний сектор", exact: true })
    .click();
  await page.waitForFunction(() => {
    const c = document.querySelector(".visit-canvas");
    return c?.dataset.ready === "true" && c.dataset.sector === "2";
  });
  if (process.env.PLANET_TEST_SCREENSHOT)
    await root.screenshot({ path: process.env.PLANET_TEST_SCREENSHOT });
  await page.reload();
  await page.locator("#game").waitFor({ state: "visible" });
  await page.locator("#startup-screen").waitFor({ state: "hidden" });
  await open();
  await page.waitForFunction(() => {
    const c = document.querySelector(".visit-canvas");
    return c?.dataset.ready === "true" && c.dataset.sector === "2";
  });
  await root
    .getByRole("button", { name: "Вернуться в космос", exact: true })
    .click();
  await root.getByText("Возвращение завершено", { exact: true }).waitFor();
  assert.deepEqual(errors, []);
  console.log(
    "PASS: browser crossing offer, cancel, explicit reselect, confirm, scene swap, reload in sector without module, return; no page errors",
  );
  await context.close();
} finally {
  await browser.close();
}
