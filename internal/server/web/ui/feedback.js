import { node } from "./dom.js";
import { I18n } from "../core/i18n.js";
let timer, persistNotice = async () => {};
export function configureFeedback(persist) { persistNotice = persist; }
export function notify(text, level = "success", persist = false) {
  if (text?.name === "AbortError") return;
  const toast = document.getElementById("toast");
  toast.replaceChildren(
    node("span", level === "error" ? "×" : level === "warning" ? "!" : "✓", "toast-icon"),
    node("span", I18n.t(text)), node("i", undefined, "toast-countdown"),
  );
  toast.dataset.level = level;
  toast.hidden = false;
  clearTimeout(timer);
  timer = setTimeout(() => { toast.hidden = true; }, 6000);
  if (persist) Promise.resolve(persistNotice(text, level)).catch(() => {});
}
window.addEventListener("signedout", () => { clearTimeout(timer); const toast = document.getElementById("toast"); if (toast) toast.hidden = true; });
