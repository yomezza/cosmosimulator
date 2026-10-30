import assert from "node:assert/strict";
import { test } from "node:test";
import { game } from "./harness.mjs";

test("idea status has only a colored border and admin can save a review", async (t) => {
  const g = await game(t, { signedIn: true, role: "admin" });
  const items = ["pending", "approved", "rejected"].map((status, i) => ({
    id: i + 1,
    status,
    title: "Idea " + i,
    description: "A proposed improvement",
    category: "other",
    author: "test_pilot",
    created_at: new Date().toISOString(),
  }));
  g.handlers.set("ideas", ({ json }) =>
    json({ items, page: 1, pages: 1, total: items.length }),
  );
  g.handlers.set("admin/ideas", ({ req, json }) => {
    const b = req.postDataJSON();
    items.find((i) => i.id === b.id).status = b.status;
    return json({ id: b.id, status: b.status });
  });
  await g.ready();
  await g.open("ideas");
  await g.page.locator("#window-ideas .idea-status").first().waitFor();
  const styles = await g.page
    .locator("#window-ideas .idea-status")
    .evaluateAll((tags) =>
      tags.map((e) => {
        const s = getComputedStyle(e);
        return {
          status: e.dataset.status,
          color: s.color,
          border: s.borderTopColor,
          background: s.backgroundColor,
          align: s.textAlign,
        };
      }),
    );
  assert.equal(styles.length, 3);
  assert.equal(new Set(styles.map((s) => s.border)).size, 3);
  assert.equal(new Set(styles.map((s) => s.color)).size, 1);
  for (const s of styles) {
    assert.equal(s.background, "rgba(0, 0, 0, 0)");
    assert.equal(s.align, "center");
  }
  await g.page.evaluate(() => window.__testApp.Modules.close("ideas"));
  await g.open("adminideas");
  await g.page.locator('#window-adminideas [data-field="Статус идеи"]').click();
  await g.page.getByRole("option", { name: "Одобрено", exact: true }).click();
  await g.page
    .getByRole("button", { name: "Сохранить статус", exact: true })
    .click();
  await g.page.waitForFunction(
    () =>
      document.querySelector("#window-adminideas .idea-detail .idea-status")
        ?.dataset.status === "approved",
  );
  await g.page.evaluate(() => window.__testApp.Modules.close("adminideas"));
  await g.open("ideas");
  await g.page.waitForFunction(
    () =>
      document.querySelector("#window-ideas .idea-status")?.dataset.status ===
      "approved",
  );
  assert.equal(
    g.requests.filter((r) => r.method === "POST" && r.path === "admin/ideas")
      .length,
    1,
  );
});
