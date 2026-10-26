import { formatPlanDate, shortId } from "../formatters.js?v=single-plan-date";
import { setTooltip } from "../tooltip.js?v=tooltip-position";
import { closeMenus } from "./add-menu.js?v=account-action-menu";
import { usageBlock, usageResetCredits } from "./usage-limit.js?v=account-action-menu";

export function accountCard({
  account,
  providerInfo,
  loading,
  isRefreshing,
  isResetting,
  onRefresh,
  onActivate,
  onDelete,
  onReset,
  onCopyAuthJSON,
  onUpdatePlan,
}) {
  const card = document.createElement("article");
  card.className = `account-card ${account.isActive ? "active" : ""}`;
  const isBusy = isRefreshing || isResetting;
  if (isRefreshing || isResetting) {
    card.setAttribute("aria-busy", "true");
  }

  const header = document.createElement("div");
  header.className = "account-card-header";

  const main = document.createElement("div");
  main.className = "account-main";
  const name = document.createElement("div");
  name.className = "account-name";
  const title = document.createElement("h3");
  title.textContent = account.label || account.email || account.accountId;
  name.append(title);
  if (account.isActive) {
    const activeBadge = document.createElement("span");
    activeBadge.className = "active-badge";
    activeBadge.textContent = "当前";
    name.append(activeBadge);
  }
  main.append(name);

  const meta = document.createElement("div");
  meta.className = "meta";
  if (account.planType) {
    meta.append(pill(account.planType));
  }
  meta.append(planDatePill(account, onUpdatePlan));
  meta.append(pill(shortId(account.accountId), account.accountId));
  main.append(meta);
  header.append(main);
  const resetCredits = usageResetCredits(account.usage);
  header.append(accountActionMenu({
    resetCredits, loading, isRefreshing, isResetting, onRefresh, onReset, onCopyAuthJSON,
    canExportAuthJSON: providerInfo.capabilities?.canExportAuthJson === true,
  }));
  card.append(header);

  card.append(
    usageBlock({
      usage: account.usage,
    }),
  );

  const actions = document.createElement("div");
  actions.className = "account-actions";
  if (providerInfo.capabilities && providerInfo.capabilities.canActivateAccount && !account.isActive) {
    actions.append(accountActionButton("激活", onActivate, isBusy, loading));
  }
  const deleteButton = accountActionButton("删除", onDelete, isBusy, loading);
  deleteButton.className = "danger";
  actions.append(deleteButton);
  card.append(actions);

  return card;
}

function actionButton(label, handler, loading) {
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = label;
  button.disabled = loading;
  button.addEventListener("click", handler);
  return button;
}

function accountActionMenu({ resetCredits, loading, isRefreshing, isResetting, onRefresh, onReset, onCopyAuthJSON, canExportAuthJSON }) {
  const menu = document.createElement("div");
  menu.className = "account-action-menu";
  const label = isRefreshing
    ? "正在刷新账号状态和额度"
    : isResetting
      ? "正在重置额度"
      : "账号操作";

  const trigger = document.createElement("button");
  trigger.type = "button";
  trigger.className = "account-refresh-button account-action-menu-trigger";
  trigger.classList.toggle("is-refreshing", isRefreshing);
  trigger.setAttribute("aria-label", label);
  trigger.setAttribute("aria-busy", `${isRefreshing || isResetting}`);
  trigger.setAttribute("aria-haspopup", "menu");
  trigger.setAttribute("aria-expanded", "false");
  trigger.dataset.popupMenuTrigger = "true";
  trigger.dataset.disabledWhenIdle = `${isRefreshing || isResetting}`;
  trigger.disabled = loading || trigger.dataset.disabledWhenIdle === "true";
  setTooltip(trigger, label);
  const icon = document.createElement("span");
  icon.className = "account-refresh-icon";
  icon.setAttribute("aria-hidden", "true");
  icon.textContent = "↻";
  trigger.append(icon);
  menu.append(trigger);

  const options = document.createElement("div");
  options.className = "account-action-menu-options";
  options.setAttribute("role", "menu");
  options.setAttribute("aria-label", "账号操作");
  options.append(
    accountMenuItem({
      label: "刷新",
      description: "刷新账号状态和额度",
      icon: "↻",
      disabledWhenIdle: isRefreshing || isResetting,
      loading,
      handler: onRefresh,
    }),
    accountMenuItem({
      label: resetCredits && resetCredits.availableCount > 0
        ? `重置额度（${resetCredits.availableCount}）`
        : "重置额度",
      description: resetCredits
        ? resetCredits.availableCount > 0
          ? `剩余 ${resetCredits.availableCount} 次可用机会`
          : "没有可用的重置次数"
        : "额度信息尚未刷新",
      icon: "↺",
      disabledWhenIdle: isRefreshing || isResetting || !resetCredits || resetCredits.availableCount < 1,
      loading,
      handler: onReset,
    }),
  );

  if (canExportAuthJSON) {
    options.append(accountMenuItem({
      label: "复制 auth.json",
      description: "复制登录凭据，用于迁移账号",
      icon: "⧉",
      disabledWhenIdle: isRefreshing || isResetting,
      loading,
      handler: onCopyAuthJSON,
    }));
  }

  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    const expanded = trigger.getAttribute("aria-expanded") === "true";
    closeMenus();
    trigger.setAttribute("aria-expanded", expanded ? "false" : "true");
  });
  trigger.addEventListener("keydown", (event) => {
    if (!["ArrowDown", "ArrowUp"].includes(event.key)) {
      return;
    }
    event.preventDefault();
    closeMenus();
    trigger.setAttribute("aria-expanded", "true");
    focusAccountMenuItem(options, event.key === "ArrowUp" ? "last" : "first");
  });
  options.addEventListener("keydown", (event) => handleAccountMenuKeydown(event, options, trigger));
  menu.append(options);
  return menu;
}

function accountMenuItem({ label, description, icon, disabledWhenIdle, loading, handler }) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "account-action-menu-item";
  button.setAttribute("role", "menuitem");
  button.dataset.disabledWhenIdle = `${disabledWhenIdle}`;
  button.disabled = loading || button.dataset.disabledWhenIdle === "true";

  const itemIcon = document.createElement("span");
  itemIcon.className = "account-action-menu-item-icon";
  itemIcon.setAttribute("aria-hidden", "true");
  itemIcon.textContent = icon;
  const copy = document.createElement("span");
  copy.className = "account-action-menu-item-copy";
  const title = document.createElement("strong");
  title.textContent = label;
  const detail = document.createElement("span");
  detail.textContent = description;
  copy.append(title, detail);
  button.append(itemIcon, copy);
  button.addEventListener("click", () => {
    closeMenus();
    handler();
  });
  return button;
}

function handleAccountMenuKeydown(event, menu, trigger) {
  if (event.key === "Escape") {
    event.preventDefault();
    event.stopPropagation();
    closeMenus();
    trigger.focus();
    return;
  }
  const positions = { ArrowDown: "next", ArrowUp: "previous", Home: "first", End: "last" };
  if (!positions[event.key]) {
    return;
  }
  event.preventDefault();
  focusAccountMenuItem(menu, positions[event.key]);
}

function focusAccountMenuItem(menu, position) {
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

function accountActionButton(label, handler, accountRefreshing, loading) {
  const button = actionButton(label, handler, loading);
  button.dataset.disabledWhenIdle = `${accountRefreshing}`;
  button.disabled = loading || button.dataset.disabledWhenIdle === "true";
  return button;
}

function pill(text, tooltipText) {
  const item = document.createElement("span");
  item.className = "pill";
  item.textContent = text;
  if (tooltipText) {
    setTooltip(item, tooltipText);
  }
  return item;
}

function planDatePill(account, onUpdatePlan) {
  const autoRenew = account.planAutoRenew;
  const date = account.planDate;
  const formattedDate = date ? formatPlanDate(date) : "YYYY-MM-DD";
  const dateLabel = autoRenew ? "续期日期" : "到期日期";
  const description = `${dateLabel}：${date ? formattedDate : "未填写"}`;
  const item = pill(formattedDate, "点击编辑套餐信息，日期留空可清除");
  item.setAttribute("aria-label", `编辑套餐信息，${description}`);
  item.classList.add("interactive", "plan-expiration");
  item.dataset.autoRenew = String(autoRenew);
  item.tabIndex = 0;
  item.setAttribute("role", "button");
  item.addEventListener("click", onUpdatePlan);
  item.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onUpdatePlan();
    }
  });
  return item;
}
