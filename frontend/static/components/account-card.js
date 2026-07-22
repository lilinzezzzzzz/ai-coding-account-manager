import { formatPlanDate, shortId } from "../formatters.js?v=split-modules";
import { setTooltip } from "../tooltip.js?v=tooltip-position";
import { usageBlock, usageResetButton, usageResetCredits } from "./usage-limit.js?v=reset-refresh-actions";

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
  onUpdatePlanExpiration,
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
  meta.append(planExpirationPill(account, onUpdatePlanExpiration));
  meta.append(pill(shortId(account.accountId), account.accountId));
  main.append(meta);
  header.append(main);
  const resetCredits = usageResetCredits(account.usage);
  header.append(accountRefreshButton({ loading, isRefreshing, isResetting, onRefresh }));
  card.append(header);

  card.append(
    usageBlock({
      usage: account.usage,
    }),
  );

  const actions = document.createElement("div");
  actions.className = "account-actions";
  if (resetCredits) {
    actions.append(
      usageResetButton({
        resetCredits,
        loading,
        isRefreshing,
        isResetting,
        onReset,
      }),
    );
  }
  if (providerInfo.capabilities && providerInfo.capabilities.canActivateAccount && !account.isActive) {
    actions.append(accountActionButton("激活", onActivate, isBusy, loading));
  }
  const deleteButton = accountActionButton("删除", onDelete, isBusy, loading);
  deleteButton.className = "danger";
  deleteButton.dataset.disabledWhenIdle = `${isBusy || account.isActive}`;
  deleteButton.disabled = loading || deleteButton.dataset.disabledWhenIdle === "true";
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

function accountRefreshButton({ loading, isRefreshing, isResetting, onRefresh }) {
  const label = isRefreshing
    ? "正在刷新账号状态和额度"
    : isResetting
      ? "额度重置期间无法刷新"
      : "刷新账号状态和额度";
  const wrapper = document.createElement("span");
  wrapper.className = "account-refresh-tooltip";
  setTooltip(wrapper, label);

  const button = document.createElement("button");
  button.type = "button";
  button.className = "account-refresh-button";
  button.classList.toggle("is-refreshing", isRefreshing);
  button.setAttribute("aria-label", label);
  button.setAttribute("aria-busy", `${isRefreshing}`);
  button.dataset.disabledWhenIdle = `${isRefreshing || isResetting}`;
  button.disabled = loading || button.dataset.disabledWhenIdle === "true";
  const icon = document.createElement("span");
  icon.className = "account-refresh-icon";
  icon.setAttribute("aria-hidden", "true");
  icon.textContent = "↻";
  button.append(icon);
  button.addEventListener("click", onRefresh);
  wrapper.append(button);
  return wrapper;
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

function planExpirationPill(account, onUpdatePlanExpiration) {
  const text = account.planExpiresAt ? formatPlanDate(account.planExpiresAt) : "YYYY-MM-DD";
  const item = pill(text, "点击录入套餐到期日，留空可清除");
  item.classList.add("interactive", "plan-expiration");
  item.tabIndex = 0;
  item.setAttribute("role", "button");
  item.addEventListener("click", onUpdatePlanExpiration);
  item.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onUpdatePlanExpiration();
    }
  });
  return item;
}
