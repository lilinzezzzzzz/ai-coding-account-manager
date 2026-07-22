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
    credits: Array.isArray(credits.credits)
      ? credits.credits.map(normalizeResetCredit).filter(Boolean).sort(compareResetCredits)
      : [],
  };
}

export function usageResetConfirmationDetail(resetCredits) {
  const content = document.createDocumentFragment();
  const summary = document.createElement("p");
  summary.className = "dialog-reset-summary";
  summary.append("当前可重置次数：");
  const count = document.createElement("strong");
  count.className = "dialog-reset-count";
  count.textContent = `${resetCredits.availableCount}`;
  summary.append(count);
  content.append(summary);

  const explanation = document.createElement("p");
  explanation.className = "dialog-reset-explanation";
  explanation.textContent = "确认后将优先使用最早失效的可用机会。";
  content.append(explanation);

  if (resetCredits.credits.length === 0) {
    const empty = document.createElement("p");
    empty.className = "dialog-reset-empty";
    empty.textContent = "Codex 当前只返回了可用次数，没有返回每次机会的详细信息。";
    content.append(empty);
    return content;
  }

  const list = document.createElement("ol");
  list.className = "dialog-reset-credit-list";
  for (const [index, credit] of resetCredits.credits.entries()) {
    list.append(resetCreditDetailItem(credit, index));
  }
  content.append(list);

  if (resetCredits.availableCount > resetCredits.credits.length) {
    const partial = document.createElement("p");
    partial.className = "dialog-reset-partial";
    partial.textContent = `Codex 返回了 ${resetCredits.credits.length} 条明细；其余 ${resetCredits.availableCount - resetCredits.credits.length} 次机会暂无详细信息。`;
    content.append(partial);
  }
  return content;
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
  if (!rateLimits) {
    return [];
  }
  return [
    limitItem(rateLimitLabel(rateLimits.primary), rateLimits.primary),
    limitItem(rateLimitLabel(rateLimits.secondary), rateLimits.secondary),
  ].filter(Boolean);
}

function normalizeResetCredit(credit) {
  if (!credit || typeof credit !== "object") {
    return null;
  }
  return {
    id: typeof credit.id === "string" ? credit.id.trim() : "",
    status: typeof credit.status === "string" ? credit.status.trim() : "",
    grantedAt: Number.isFinite(credit.grantedAt) ? credit.grantedAt : null,
    expiresAt: Number.isFinite(credit.expiresAt) ? credit.expiresAt : null,
    title: typeof credit.title === "string" ? credit.title.trim() : "",
  };
}

function compareResetCredits(left, right) {
  const leftExpiry = left.expiresAt > 0 ? left.expiresAt : Number.POSITIVE_INFINITY;
  const rightExpiry = right.expiresAt > 0 ? right.expiresAt : Number.POSITIVE_INFINITY;
  return leftExpiry - rightExpiry || left.id.localeCompare(right.id);
}

function resetCreditDetailItem(credit, index) {
  const item = document.createElement("li");
  item.className = "dialog-reset-credit";

  const header = document.createElement("div");
  header.className = "dialog-reset-credit-header";
  const title = document.createElement("strong");
  title.textContent = credit.title || `重置机会 ${index + 1}`;
  const status = document.createElement("span");
  status.className = "dialog-reset-credit-status";
  status.dataset.status = credit.status.toLowerCase();
  status.textContent = resetCreditStatusLabel(credit.status);
  header.append(title, status);
  item.append(header);

  const creditID = document.createElement("div");
  creditID.className = "dialog-reset-credit-id";
  const creditIDValue = document.createElement("code");
  creditIDValue.textContent = credit.id || "未知";
  creditID.append(creditIDValue);

  const times = document.createElement("div");
  times.className = "dialog-reset-credit-times";
  const grantedAt = document.createElement("span");
  grantedAt.textContent = `发放时间: ${formatResetCreditTime(credit.grantedAt)}`;
  const expiresAt = document.createElement("span");
  expiresAt.textContent = `失效时间: ${formatResetCreditTime(credit.expiresAt)}`;
  times.append(grantedAt, expiresAt);

  item.append(creditID, times);
  return item;
}

function formatResetCreditTime(value) {
  return value > 0 ? formatDateTime(value) : "未知";
}

function resetCreditStatusLabel(status) {
  const labels = {
    available: "可用",
    redeemed: "已使用",
    expired: "已失效",
  };
  const normalized = status.toLowerCase();
  return labels[normalized] || status || "状态未知";
}

function rateLimitLabel(limit) {
  const durationMins = limit && Math.trunc(limit.windowDurationMins);
  if (durationMins === 7 * 24 * 60) {
    return "7 天限额";
  }
  if (durationMins === 5 * 60) {
    return "5 小时限额";
  }
  if (durationMins > 0 && durationMins % 60 === 0) {
    return `${durationMins / 60} 小时限额`;
  }
  return durationMins > 0 ? `${durationMins} 分钟限额` : "额度限额";
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
