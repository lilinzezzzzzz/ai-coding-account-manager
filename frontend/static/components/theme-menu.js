import { setupPopupMenu } from "./popup-menu.js?v=frontend-cleanup-1";
import { setTooltip } from "../tooltip.js?v=tooltip-position";

const modes = [
  { value: "system", label: "跟随系统", paths: ["M3 4h18v12H3z", "M12 16v4M8 20h8"] },
  { value: "light", label: "浅色", paths: ["M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8", "M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"] },
  { value: "dark", label: "深色", paths: ["M20.5 13A8.5 8.5 0 0 1 11 3.5 8.5 8.5 0 1 0 20.5 13Z"] },
];

function icon(paths) {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  for (const [key, value] of Object.entries({ viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", "stroke-width": "1.7", "stroke-linecap": "round", "stroke-linejoin": "round", "aria-hidden": "true", focusable: "false" })) {
    svg.setAttribute(key, value);
  }
  for (const d of paths) {
    const path = document.createElementNS(svg.namespaceURI, "path");
    path.setAttribute("d", d);
    svg.append(path);
  }
  return svg;
}

export function setupThemeMenu(container) {
  const trigger = document.createElement("button");
  trigger.type = "button";
  trigger.className = "theme-menu-trigger btn btn-icon btn-outline-secondary";
  const menu = document.createElement("div");
  menu.id = "theme-options";
  menu.className = "theme-menu-options dropdown-menu";
  const items = modes.map((mode) => {
    const item = document.createElement("button");
    item.type = "button";
    item.className = "theme-menu-item dropdown-item";
    item.setAttribute("role", "menuitemradio");
    const label = document.createElement("span");
    label.textContent = mode.label;
    const check = icon(["M5 12l4 4L19 6"]);
    check.classList.add("theme-menu-check");
    item.append(icon(mode.paths), label, check);
    item.addEventListener("click", () => {
      document.dispatchEvent(new CustomEvent("theme-preference-change", { detail: mode.value }));
    });
    menu.append(item);
    return item;
  });
  function update() {
    const preference = document.documentElement.dataset.themePreference || "system";
    const selected = modes.findIndex((mode) => mode.value === preference);
    const mode = modes[selected];
    trigger.replaceChildren(icon(mode.paths));
    trigger.setAttribute("aria-label", `外观：${mode.label}`);
    setTooltip(trigger, `外观：${mode.label}`);
    items.forEach((item, index) => item.setAttribute("aria-checked", String(index === selected)));
  }
  setupPopupMenu(container, trigger, menu, "外观", { focusCheckedOnClick: true });
  document.addEventListener("theme-change", update);
  update();
}
