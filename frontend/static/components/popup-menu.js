const menuItemSelector = '[role="menuitem"], [role="menuitemradio"]';

export function closeMenus() {
  document.querySelectorAll('[data-popup-menu-trigger][aria-expanded="true"]').forEach((trigger) => {
    trigger.setAttribute("aria-expanded", "false");
  });
}

export function setupPopupMenu(container, trigger, menu, label, { focusCheckedOnClick = false } = {}) {
  trigger.dataset.popupMenuTrigger = "true";
  trigger.setAttribute("aria-haspopup", "menu");
  trigger.setAttribute("aria-expanded", "false");
  if (menu.id) {
    trigger.setAttribute("aria-controls", menu.id);
  }
  menu.setAttribute("role", "menu");
  menu.setAttribute("aria-label", label);
  menu.querySelectorAll(menuItemSelector).forEach((item) => {
    item.tabIndex = -1;
  });

  function open() {
    closeMenus();
    trigger.setAttribute("aria-expanded", "true");
  }

  function closeAndRestoreFocus() {
    trigger.focus();
    trigger.setAttribute("aria-expanded", "false");
  }

  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    if (trigger.getAttribute("aria-expanded") === "true") {
      closeAndRestoreFocus();
      return;
    }
    open();
    if (event.detail === 0) {
      focusMenuItem(menu, "first");
    } else if (focusCheckedOnClick) {
      menu.querySelector('[aria-checked="true"]')?.focus();
    }
  });
  trigger.addEventListener("keydown", (event) => {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") {
      return;
    }
    event.preventDefault();
    open();
    focusMenuItem(menu, event.key === "ArrowUp" ? "last" : "first");
  });

  menu.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      closeAndRestoreFocus();
      return;
    }
    if (event.key === "Tab") {
      closeAndRestoreFocus();
      return;
    }
    const positions = { ArrowDown: "next", ArrowUp: "previous", Home: "first", End: "last" };
    if (Object.hasOwn(positions, event.key)) {
      event.preventDefault();
      focusMenuItem(menu, positions[event.key]);
    }
  });
  // Return focus before an item's action opens a dialog or redraws the page.
  menu.addEventListener("click", (event) => {
    if (event.target.closest(menuItemSelector)) {
      closeAndRestoreFocus();
    }
  }, { capture: true });
  container.addEventListener("focusout", (event) => {
    if (!container.contains(event.relatedTarget)) {
      trigger.setAttribute("aria-expanded", "false");
    }
  });
  container.append(trigger, menu);
}

function focusMenuItem(menu, position) {
  const items = [...menu.querySelectorAll(menuItemSelector)].filter((item) => !item.disabled);
  if (items.length === 0) {
    return;
  }
  const currentIndex = items.indexOf(document.activeElement);
  let index = 0;
  if (position === "last" || (position === "previous" && currentIndex < 0)) {
    index = items.length - 1;
  } else if (position === "next") {
    index = (currentIndex + 1) % items.length;
  } else if (position === "previous") {
    index = (currentIndex - 1 + items.length) % items.length;
  }
  items[index].focus();
}

export function actionMenuItem(className, { label, description, icon, disabledWhenIdle, loading, handler }) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = `${className} dropdown-item`;
  button.setAttribute("role", "menuitem");
  button.dataset.disabledWhenIdle = String(disabledWhenIdle);
  button.disabled = loading || disabledWhenIdle;

  const itemIcon = document.createElement("span");
  itemIcon.className = `${className}-icon`;
  itemIcon.setAttribute("aria-hidden", "true");
  itemIcon.textContent = icon;
  const copy = document.createElement("span");
  copy.className = `${className}-copy`;
  const title = document.createElement("strong");
  title.textContent = label;
  const detail = document.createElement("span");
  detail.textContent = description;
  copy.append(title, detail);
  button.append(itemIcon, copy);
  button.addEventListener("click", handler);
  return button;
}
