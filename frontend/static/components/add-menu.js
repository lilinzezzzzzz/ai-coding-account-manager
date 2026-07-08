export function closeAddMenus() {
  document.querySelectorAll(".add-menu-trigger[aria-expanded='true']").forEach((trigger) => {
    trigger.setAttribute("aria-expanded", "false");
  });
}

export function providerActions({ providerInfo, loading, onLogin, onImportFile, onImportJSON }) {
  const actions = document.createElement("div");
  actions.className = "toolbar";
  const menu = document.createElement("div");
  menu.className = "add-menu";
  const trigger = document.createElement("button");
  trigger.type = "button";
  trigger.className = "primary add-menu-trigger";
  const triggerIcon = document.createElement("span");
  triggerIcon.className = "add-menu-trigger-icon";
  triggerIcon.setAttribute("aria-hidden", "true");
  trigger.append(triggerIcon, "添加账号");
  trigger.setAttribute("aria-haspopup", "menu");
  trigger.setAttribute("aria-expanded", "false");
  trigger.dataset.disabledWhenIdle = `${providerInfo.status !== "available"}`;
  trigger.disabled = loading || trigger.dataset.disabledWhenIdle === "true";
  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    const expanded = trigger.getAttribute("aria-expanded") === "true";
    closeAddMenus();
    trigger.setAttribute("aria-expanded", expanded ? "false" : "true");
  });
  trigger.addEventListener("keydown", (event) => {
    if (!["ArrowDown", "ArrowUp"].includes(event.key)) {
      return;
    }
    event.preventDefault();
    closeAddMenus();
    trigger.setAttribute("aria-expanded", "true");
    focusAddMenuItem(options, event.key === "ArrowUp" ? "last" : "first");
  });
  menu.append(trigger);

  const options = document.createElement("div");
  options.className = "add-menu-options";
  options.setAttribute("role", "menu");
  options.setAttribute("aria-label", "添加账号");
  options.append(
    addMenuItem(
      {
        label: "登录",
        description: "通过浏览器完成账号授权",
        icon: "→",
        handler: onLogin,
      },
      providerInfo,
      loading,
    ),
  );
  const importGroup = document.createElement("div");
  importGroup.className = "add-menu-group";
  importGroup.setAttribute("role", "group");
  importGroup.setAttribute("aria-label", "导入账号");
  const importLabel = document.createElement("span");
  importLabel.className = "add-menu-group-label";
  importLabel.textContent = "导入账号";
  importLabel.setAttribute("aria-hidden", "true");
  importGroup.append(importLabel);
  importGroup.append(
    addMenuItem(
      {
        label: "文件",
        description: "选择 auth.json 文件",
        icon: "↑",
        handler: onImportFile,
      },
      providerInfo,
      loading,
    ),
  );
  importGroup.append(
    addMenuItem(
      {
        label: "JSON",
        description: "粘贴 auth.json 内容",
        icon: "{}",
        handler: onImportJSON,
      },
      providerInfo,
      loading,
    ),
  );
  options.append(importGroup);
  options.addEventListener("keydown", (event) => handleAddMenuKeydown(event, options, trigger));
  menu.append(options);
  actions.append(menu);
  return actions;
}

function addMenuItem(item, providerInfo, loading) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "add-menu-item";
  button.setAttribute("role", "menuitem");
  button.dataset.disabledWhenIdle = `${providerInfo.status !== "available"}`;
  button.disabled = loading || button.dataset.disabledWhenIdle === "true";

  const icon = document.createElement("span");
  icon.className = "add-menu-item-icon";
  icon.textContent = item.icon;
  icon.setAttribute("aria-hidden", "true");
  const copy = document.createElement("span");
  copy.className = "add-menu-item-copy";
  const label = document.createElement("strong");
  label.textContent = item.label;
  const description = document.createElement("span");
  description.textContent = item.description;
  copy.append(label, description);
  button.append(icon, copy);

  button.addEventListener("click", () => {
    closeAddMenus();
    item.handler();
  });
  return button;
}

function handleAddMenuKeydown(event, menu, trigger) {
  if (event.key === "Escape") {
    event.preventDefault();
    event.stopPropagation();
    closeAddMenus();
    trigger.focus();
    return;
  }
  const positions = { ArrowDown: "next", ArrowUp: "previous", Home: "first", End: "last" };
  const position = positions[event.key];
  if (!position) {
    return;
  }
  event.preventDefault();
  focusAddMenuItem(menu, position);
}

function focusAddMenuItem(menu, position) {
  const items = [...menu.querySelectorAll("[role='menuitem']:not(:disabled)")];
  if (items.length === 0) {
    return;
  }
  const currentIndex = items.indexOf(document.activeElement);
  let nextIndex = 0;
  if (position === "last") {
    nextIndex = items.length - 1;
  } else if (position === "next") {
    nextIndex = currentIndex < 0 ? 0 : (currentIndex + 1) % items.length;
  } else if (position === "previous") {
    nextIndex = currentIndex < 0 ? items.length - 1 : (currentIndex - 1 + items.length) % items.length;
  }
  items[nextIndex].focus();
}
