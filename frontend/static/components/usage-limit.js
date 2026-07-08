import { clampPercent, formatDateTime, formatPercent, parseSnapshot } from "../formatters.js?v=split-modules";
import { setTooltip } from "../tooltip.js?v=tooltip-position";

export function usageBlock({ usage }) {
  const wrapper = document.createElement("div");
  wrapper.className = "usage";
  const limits = usageLimitItems(usage);
  if (limits.length === 0) {
    const empty = document.createElement("div");
    empty.className = "usage-empty";
    empty.textContent = usage ? "额度数据不可用" : "额度未刷新";
    wrapper.append(empty);
    return wrapper;
  }
  for (const item of limits) {
    wrapper.append(usageLimitBlock(item));
  }
  return wrapper;
}

export function usageResetCredits(usage) {
  if (!usage) {
    return null;
  }
  const credits = usage.rateLimitResetCredits;
  if (!credits || typeof credits.availableCount !== "number") {
    return null;
  }
  return {
    availableCount: Math.max(0, Math.trunc(credits.availableCount)),
  };
}

export function usageResetButton({ resetCredits, loading, isRefreshing, isResetting, onReset }) {
  const hasAvailableCredit = resetCredits.availableCount > 0;
  const label = isResetting
    ? "正在重置额度"
    : hasAvailableCredit
      ? `可重置次数 ${resetCredits.availableCount}，点击重置`
      : "没有可用的重置次数";
  const wrapper = document.createElement("span");
  wrapper.className = "usage-reset-tooltip";
  setTooltip(wrapper, label);

  const button = document.createElement("button");
  button.type = "button";
  button.className = "usage-reset-button";
  const icon = document.createElement("span");
  icon.className = "usage-reset-icon";
  icon.setAttribute("aria-hidden", "true");
  icon.textContent = "↻";
  button.append(icon);
  button.setAttribute("aria-label", label);
  button.classList.toggle("is-resetting", isResetting);
  button.setAttribute("aria-busy", `${isResetting}`);
  button.dataset.disabledWhenIdle = `${isRefreshing || isResetting || !hasAvailableCredit}`;
  button.disabled = loading || button.dataset.disabledWhenIdle === "true";
  button.addEventListener("click", onReset);
  wrapper.append(button);
  return wrapper;
}

function usageLimitItems(usage) {
  if (!usage) {
    return [];
  }
  const snapshot = parseSnapshot(usage.snapshotJson);
  const rateLimits = snapshot && snapshot.rateLimits ? snapshot.rateLimits : null;
  if (rateLimits) {
    return [
      limitItem("5 小时限额", rateLimits.primary),
      limitItem("7 天限额", rateLimits.secondary),
    ].filter(Boolean);
  }
  if (typeof usage.usedPercent === "number") {
    return [limitItem("5 小时限额", { usedPercent: usage.usedPercent, resetsAt: usage.resetsAt })].filter(Boolean);
  }
  return [];
}

function limitItem(label, limit) {
  if (!limit || typeof limit.usedPercent !== "number") {
    return null;
  }
  const usedPercent = clampPercent(limit.usedPercent);
  return {
    label,
    usedPercent,
    remainingPercent: clampPercent(100 - usedPercent),
    resetsAt: limit.resetsAt || null,
  };
}

function usageLimitBlock(item) {
  const section = document.createElement("section");
  section.className = "usage-limit";
  section.dataset.level = item.remainingPercent <= 15 ? "critical" : item.remainingPercent <= 35 ? "warning" : "normal";
  const header = document.createElement("div");
  header.className = "usage-limit-header";
  const title = document.createElement("div");
  title.className = "usage-limit-title";
  title.textContent = item.label;
  const remaining = document.createElement("strong");
  remaining.className = "usage-remaining";
  remaining.textContent = `剩余 ${formatPercent(item.remainingPercent)}`;
  header.append(title, remaining);
  section.append(header);

  const progress = document.createElement("div");
  progress.className = "usage-progress";
  progress.setAttribute("role", "meter");
  progress.setAttribute("aria-label", `${item.label}剩余额度`);
  progress.setAttribute("aria-valuemin", "0");
  progress.setAttribute("aria-valuemax", "100");
  progress.setAttribute("aria-valuenow", `${Math.round(item.remainingPercent)}`);
  const bar = document.createElement("div");
  bar.className = "usage-progress-bar";
  bar.style.width = `${item.remainingPercent}%`;
  progress.append(bar);
  section.append(progress);

  const reset = document.createElement("div");
  reset.className = "usage-reset";
  reset.textContent = `重置时间 ${item.resetsAt ? formatDateTime(item.resetsAt) : "未知"}`;
  section.append(reset);
  return section;
}
