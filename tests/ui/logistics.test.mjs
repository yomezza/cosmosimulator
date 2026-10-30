import assert from "node:assert/strict";
import { test } from "node:test";
import { game } from "./harness.mjs";

async function cargoGame(t) {
  const g = await game(t, { signedIn: true });
  g.data.logistics = {
    personal: [{ star: 1, station: "port-1", cargo: { iron: 10, agni: 5 } }],
    corporate: [],
    built: [],
    history: [],
  };
  g.handlers.set("logistics", async ({ req, json }) => {
    if (req.method() === "GET") return json(g.data.logistics);
    const body = req.postDataJSON();
    const source =
      body.from.kind === "ship"
        ? g.data.expedition.ship.cargo
        : g.data.logistics.personal[0].cargo;
    const target =
      body.to.kind === "ship"
        ? g.data.expedition.ship.cargo
        : g.data.logistics.personal[0].cargo;
    for (const item of body.items || [
      { resource: body.resource, quantity: body.quantity },
    ]) {
      source[item.resource] -= item.quantity;
      target[item.resource] = (target[item.resource] || 0) + item.quantity;
    }
    return json({ ok: true });
  });
  await g.ready();
  return g;
}

test("station to cargo transfer refreshes the real snapshot without legacy window globals", async (t) => {
  const g = await cargoGame(t);
  assert.equal(await g.page.evaluate(() => typeof window.refresh), "undefined");
  await g.open("logistics");
  await g.page.locator(".storage-station summary").first().click();
  const tile = g.page.locator(
    '[data-inventory="personal"] [data-resource="iron"]',
  );
  await tile.click();
  await g.page
    .getByRole("menuitem", { name: "Переместить ресурс", exact: true })
    .click();
  await g.page
    .locator("#window-cargo-destination")
    .getByRole("button", { name: "Продолжить", exact: true })
    .click();
  await g.page.locator("#cargo-transfer-quantity").fill("3");
  await g.page
    .locator("#window-cargo-transfer")
    .getByRole("button", { name: "Переместить", exact: true })
    .click();
  await g.page.waitForFunction(
    () => window.__testApp.session.data.expedition.ship.cargo.iron === 9,
  );
  assert.equal(
    await g.page.locator("#toast").getAttribute("data-level"),
    "success",
  );
  assert.equal(
    g.requests.filter((r) => r.method === "POST" && r.path === "logistics")
      .length,
    1,
  );
  assert.equal(g.data.logistics.personal[0].cargo.iron, 7);
});

test("dragging cargo survives a snapshot and transfers cargo back to the station", async (t) => {
  const g = await cargoGame(t);
  await g.open("logistics");
  await g.page
    .locator("#window-logistics .storage-station summary")
    .first()
    .click();
  await g.open("cargo");
  await g.page.evaluate(() => {
    const tile = document.querySelector('#window-cargo [data-resource="iron"]');
    tile.dataset.auditDrag = "true";
    const dt = new DataTransfer();
    tile.dispatchEvent(
      new DragEvent("dragstart", { bubbles: true, dataTransfer: dt }),
    );
  });
  await g.refresh();
  assert.equal(
    await g.page.locator("#window-cargo [data-audit-drag]").count(),
    1,
    "poll must not remove the dragged tile",
  );
  await g.page.evaluate(() => {
    const tile = document.querySelector("#window-cargo [data-audit-drag]");
    tile.dispatchEvent(new DragEvent("dragend", { bubbles: true }));
  });
  // Native drag end suppresses the synthetic click that browsers emit immediately after a drop.
  await g.page.waitForTimeout(220);
  const tile = g.page.locator('#window-cargo [data-resource="iron"]');
  await tile.click();
  await g.page
    .getByRole("menuitem", { name: "Переместить ресурс", exact: true })
    .click();
  await g.page
    .locator("#window-cargo-destination")
    .getByRole("button", { name: "Продолжить", exact: true })
    .click();
  await g.page.locator("#cargo-transfer-quantity").fill("2");
  await g.page
    .locator("#window-cargo-transfer")
    .getByRole("button", { name: "Переместить", exact: true })
    .click();
  await g.page.waitForFunction(
    () => window.__testApp.session.data.expedition.ship.cargo.iron === 4,
  );
  assert.equal(g.data.logistics.personal[0].cargo.iron, 12);
  assert.equal(
    await g.page.locator("#toast").getAttribute("data-level"),
    "success",
  );
});

test("batch selection transfers two resources in one command", async (t) => {
  const g = await cargoGame(t);
  await g.open("logistics");
  await g.page.locator(".storage-station summary").first().click();
  const station = g.page.locator('[data-inventory="personal"]');
  for (const id of ["iron", "agni"])
    await station
      .locator(`[data-resource="${id}"]`)
      .click({ modifiers: ["Control"] });
  await g.page
    .locator("#window-logistics")
    .getByRole("button", { name: "Переместить выбранное", exact: true })
    .click();
  await g.page
    .locator("#window-cargo-destination")
    .getByRole("button", { name: "Продолжить", exact: true })
    .click();
  await g.page.locator("#cargo-transfer-iron").fill("2");
  await g.page.locator("#cargo-transfer-agni").fill("1");
  await g.page
    .locator("#window-cargo-transfer")
    .getByRole("button", { name: "Переместить", exact: true })
    .click();
  await g.page.waitForFunction(
    () => window.__testApp.session.data.expedition.ship.cargo.agni === 3,
  );
  const posts = g.requests.filter(
    (r) => r.method === "POST" && r.path === "logistics",
  );
  assert.equal(posts.length, 1);
  assert.deepEqual(posts[0].body.items, [
    { resource: "iron", quantity: 2 },
    { resource: "agni", quantity: 1 },
  ]);
});
