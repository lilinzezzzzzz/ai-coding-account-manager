import { closeMenus } from "./add-menu.js?v=account-action-menu";
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

export function setupThemeMenu() {
  const container = document.querySelector("#theme-menu");
  if (!container) return;
  const trigger = document.createElement("button");
  trigger.type = "button";
  trigger.className = "theme-menu-trigger";
  trigger.dataset.popupMenuTrigger = "true";
  trigger.setAttribute("aria-haspopup", "menu");
  trigger.setAttribute("aria-expanded", "false");
  trigger.setAttribute("aria-controls", "theme-options");
  const menu = document.createElement("div");
  menu.id = "theme-options";
  menu.className = "theme-menu-options";
  menu.setAttribute("role", "menu");
  menu.setAttribute("aria-label", "外观");
  const items = modes.map((mode) => {
    const item = document.createElement("button");
    item.type = "button";
    item.className = "theme-menu-item";
    item.setAttribute("role", "menuitemradio");
    item.tabIndex = -1;
    const label = document.createElement("span");
    label.textContent = mode.label;
    const check = icon(["M5 12l4 4L19 6"]);
    check.classList.add("theme-menu-check");
    item.append(icon(mode.paths), label, check);
    item.addEventListener("click", () => {
      document.dispatchEvent(new CustomEvent("theme-preference-change", { detail: mode.value }));
      closeMenus();
      trigger.focus();
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
  function open(index) {
    closeMenus();
    trigger.setAttribute("aria-expanded", "true");
    items[index].focus();
  }
  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    if (trigger.getAttribute("aria-expanded") === "true") closeMenus();
    else open(items.findIndex((item) => item.getAttribute("aria-checked") === "true"));
  });
  trigger.addEventListener("keydown", (event) => {
    if (!["ArrowDown", "ArrowUp"].includes(event.key)) return;
    event.preventDefault();
    open(event.key === "ArrowUp" ? items.length - 1 : 0);
  });
  menu.addEventListener("keydown", (event) => {
    const index = items.indexOf(document.activeElement);
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      closeMenus();
      trigger.focus();
    } else if (event.key === "Tab") {
      closeMenus();
      trigger.focus();
    } else if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
      event.preventDefault();
      const next = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1
        : (index + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
      items[next].focus();
    }
  });
  container.addEventListener("focusout", (event) => {
    if (!container.contains(event.relatedTarget)) trigger.setAttribute("aria-expanded", "false");
  });
  container.append(trigger, menu);
  document.addEventListener("theme-change", update);
  update();
}
