import assert from "node:assert/strict";
import { test } from "node:test";
import { game } from "./harness.mjs";

test("M1: a late Users response must not reinsert private data after logout", async (t) => {
  const g = await game(t, { signedIn: true, role: "admin" });
  await g.ready();
  let respond;
  const pending = new Promise((resolve) => {
    respond = resolve;
  });
  g.handlers.set("admin/users", async ({ json }) => {
    await pending;
    return json(g.data.users);
  });
  const requested = g.page.waitForRequest(
    (r) => new URL(r.url()).pathname === "/api/admin/users",
  );
  await g.open("users");
  await requested;
  await g.page.evaluate(() => window.__testApp.showAuth());
  const delivered = g.page.waitForResponse(
    (r) => new URL(r.url()).pathname === "/api/admin/users",
  );
  respond();
  await delivered;
  await g.page.evaluate(
    () =>
      new Promise((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(resolve)),
      ),
  );
  assert.equal(
    await g.page.locator("#window-users .module-content").textContent(),
    "",
  );
});

test("M3: reused confirmation must show the current title", async (t) => {
  const g = await game(t);
  await g.ready();
  await g.page.evaluate(() => {
    window.__testApp.UI.confirm("First", "one", () => {});
    window.__testApp.UI.confirm("Second", "two", () => {});
  });
  assert.equal(await g.page.locator("#title-confirm").textContent(), "Second");
});

test("M4: an open picker must stay inside a resized viewport", async (t) => {
  const g = await game(t);
  await g.ready();
  await g.page.evaluate(() => {
    const picker = window.__testApp.UI.picker([[1, "one"]], 1, () => {});
    picker.style.cssText = "position:fixed;left:1200px;top:100px;width:180px";
    document.body.append(picker);
    picker.click();
  });
  await g.page.waitForSelector(".picker-popover");
  await g.page.setViewportSize({ width: 600, height: 800 });
  // resize is delivered on the browser's render loop, after setViewportSize.
  await g.page.waitForFunction(() => {
    const r = document.querySelector(".picker-popover").getBoundingClientRect();
    return r.x >= 0 && r.right <= innerWidth;
  });
  const box = await g.page.locator(".picker-popover").boundingBox();
  assert.ok(box.x >= 0 && box.x + box.width <= 600);
});

test("M5: button completion must preserve the callback's disabled state", async (t) => {
  const g = await game(t);
  await g.ready();
  const disabled = await g.page.evaluate(async () => {
    let button;
    button = window.__testApp.UI.button("Action", async () => {
      button.disabled = true;
    });
    document.body.append(button);
    button.click();
    await new Promise((resolve) => setTimeout(resolve, 0));
    return button.disabled;
  });
  assert.equal(disabled, true);
});

test("M2: non-JSON HTTP failure must retain its status", async (t) => {
  const g = await game(t);
  await g.ready();
  g.handlers.set("test-http-error", ({ route }) =>
    route.fulfill({
      status: 502,
      contentType: "text/html",
      body: "<html>Proxy failure</html>",
    }),
  );
  const status = await g.page.evaluate(async () => {
    try {
      await window.__testApp.api("test-http-error");
    } catch (e) {
      return e.status ?? null;
    }
  });
  assert.equal(status, 502);
});
