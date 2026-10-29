// Optional review images of synthetic scenes, not pixel-golden assertions.
import { mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { game } from "./harness.mjs";

const artifacts = new URL("./artifacts/", import.meta.url);
await mkdir(artifacts, { recursive: true });
let cleanup;
const g = await game(
  {
    after(fn) {
      cleanup = fn;
    },
  },
  { signedIn: true },
);
try {
  await g.ready();
  await g.open("tasks");
  await g.page.waitForSelector(".planner-ship-count");
  await g.page.screenshot({
    path: fileURLToPath(new URL("desktop-planner.png", artifacts)),
    animations: "disabled",
  });
  await g.page.evaluate(() => Modules.close("tasks"));
  await g.open("ship");
  await g.page.waitForSelector(".ship-inline-model svg");
  await g.page
    .locator(".ship-controls")
    .getByRole("button", { name: "Трюм", exact: true })
    .click();
  await g.page.waitForFunction(
    () => document.getElementById("window-cargo").open,
  );
  await g.page.screenshot({
    path: fileURLToPath(new URL("desktop-ship-cargo.png", artifacts)),
    animations: "disabled",
  });
  console.log("Synthetic review images: tests/ui/artifacts/");
} finally {
  await cleanup();
}
