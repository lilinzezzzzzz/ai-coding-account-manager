import { formatPlanDate, shortId } from "../formatters.js?v=single-plan-date";
import { setTooltip } from "../tooltip.js?v=tooltip-position";
import { actionMenuItem, setupPopupMenu } from "./popup-menu.js?v=frontend-cleanup-1";
import { usageBlock, usageResetCredits } from "./usage-limit.js?v=tabler-1";

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
  card.className = `account-card card ${account.isActive ? "active" : ""}`;
  const isBusy = isRefreshing || isResetting;
  if (isRefreshing || isResetting) {
    card.setAttribute("aria-busy", "true");
  }

  const header = document.createElement("header");
  header.className = "account-card-header card-header";

  const main = document.createElement("div");
  main.className = "account-main";
  const name = document.createElement("div");
  name.className = "account-name";
  const title = document.createElement("h3");
  title.textContent = account.label || account.email || account.accountId;
  name.append(title);
  if (account.isActive) {
    const activeBadge = document.createElement("span");
    activeBadge.className = "active-badge badge bg-blue-lt";
    activeBadge.textContent = "当前";
    name.append(activeBadge);
  }
  main.append(name);

  const meta = document.createElement("div");
  meta.className = "meta";
  if (account.planType) {
    meta.append(pill(account.planType));
  }
  meta.append(planDateButton(account, onUpdatePlan, loading));
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

  const actions = document.createElement("footer");
  actions.className = "account-actions card-footer";
  if (providerInfo.capabilities && providerInfo.capabilities.canActivateAccount && !account.isActive) {
    actions.append(accountActionButton("激活", onActivate, isBusy, loading));
  }
  const deleteButton = accountActionButton("删除", onDelete, isBusy, loading);
  deleteButton.className = "danger btn btn-sm btn-outline-danger";
  actions.append(deleteButton);
  card.append(actions);

  return card;
}

function actionButton(label, handler, loading) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-sm btn-outline-primary";
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
  trigger.className = "account-refresh-button account-action-menu-trigger btn btn-icon btn-ghost-secondary";
  trigger.classList.toggle("is-refreshing", isRefreshing);
  trigger.setAttribute("aria-label", label);
  trigger.setAttribute("aria-busy", `${isRefreshing || isResetting}`);
  trigger.dataset.disabledWhenIdle = `${isRefreshing || isResetting}`;
  trigger.disabled = loading || trigger.dataset.disabledWhenIdle === "true";
  setTooltip(trigger, label);
  const icon = document.createElement("span");
  icon.className = "account-refresh-icon";
  icon.setAttribute("aria-hidden", "true");
  icon.textContent = "↻";
  trigger.append(icon);

  const options = document.createElement("div");
  options.className = "account-action-menu-options dropdown-menu";
  const createItem = (item) => actionMenuItem("account-action-menu-item", { ...item, loading });
  options.append(
    createItem({
      label: "刷新",
      description: "刷新账号状态和额度",
      icon: "↻",
      disabledWhenIdle: isRefreshing || isResetting,
      handler: onRefresh,
    }),
    createItem({
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
      handler: onReset,
    }),
  );

  if (canExportAuthJSON) {
    options.append(createItem({
      label: "复制 auth.json",
      description: "复制登录凭据，用于迁移账号",
      icon: "⧉",
      disabledWhenIdle: isRefreshing || isResetting,
      handler: onCopyAuthJSON,
    }));
  }

  setupPopupMenu(menu, trigger, options, "账号操作");
  return menu;
}

function accountActionButton(label, handler, accountRefreshing, loading) {
  const button = actionButton(label, handler, loading);
  button.dataset.disabledWhenIdle = `${accountRefreshing}`;
  button.disabled = loading || button.dataset.disabledWhenIdle === "true";
  return button;
}

function pill(text, tooltipText) {
  const item = document.createElement("span");
  item.className = "pill badge";
  item.textContent = text;
  if (tooltipText) {
    setTooltip(item, tooltipText);
  }
  return item;
}

function planDateButton(account, onUpdatePlan, loading) {
  const autoRenew = account.planAutoRenew;
  const date = account.planDate;
  const formattedDate = date ? formatPlanDate(date) : "YYYY-MM-DD";
  const dateLabel = autoRenew ? "续期日期" : "到期日期";
  const description = `${dateLabel}：${date ? formattedDate : "未填写"}`;
  const button = document.createElement("button");
  button.type = "button";
  button.className = "pill badge interactive plan-expiration";
  button.textContent = formattedDate;
  button.disabled = loading;
  button.dataset.autoRenew = String(autoRenew);
  button.setAttribute("aria-label", `编辑套餐信息，${description}`);
  setTooltip(button, "点击编辑套餐信息，日期留空可清除");
  button.addEventListener("click", onUpdatePlan);
  return button;
}
