import { actionMenuItem, setupPopupMenu } from "./popup-menu.js?v=frontend-cleanup-1";

export function providerActions({ providerInfo, loading, onLogin, onImportFile, onImportJSON }) {
  const disabledWhenIdle = providerInfo.status !== "available";
  const actions = document.createElement("div");
  actions.className = "toolbar";
  const menu = document.createElement("div");
  menu.className = "add-menu";
  const trigger = document.createElement("button");
  trigger.type = "button";
  trigger.className = "primary add-menu-trigger btn btn-primary";
  const triggerIcon = document.createElement("span");
  triggerIcon.className = "add-menu-trigger-icon";
  triggerIcon.setAttribute("aria-hidden", "true");
  trigger.append(triggerIcon, "添加账号");
  trigger.dataset.disabledWhenIdle = String(disabledWhenIdle);
  trigger.disabled = loading || disabledWhenIdle;

  const options = document.createElement("div");
  options.className = "add-menu-options dropdown-menu";
  const createItem = (item) => actionMenuItem("add-menu-item", { ...item, disabledWhenIdle, loading });
  options.append(createItem({
    label: "登录",
    description: "通过浏览器完成账号授权",
    icon: "→",
    handler: onLogin,
  }));

  const importGroup = document.createElement("div");
  importGroup.className = "add-menu-group";
  importGroup.setAttribute("role", "group");
  importGroup.setAttribute("aria-label", "导入账号");
  const importLabel = document.createElement("span");
  importLabel.className = "add-menu-group-label";
  importLabel.textContent = "导入账号";
  importLabel.setAttribute("aria-hidden", "true");
  importGroup.append(
    importLabel,
    createItem({ label: "文件", description: "选择 auth.json 文件", icon: "↑", handler: onImportFile }),
    createItem({ label: "JSON", description: "粘贴 auth.json 内容", icon: "{}", handler: onImportJSON }),
  );
  options.append(importGroup);
  setupPopupMenu(menu, trigger, options, "添加账号");
  actions.append(menu);
  return actions;
}
