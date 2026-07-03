export function parseSnapshot(snapshotJson) {
  if (!snapshotJson) {
    return null;
  }
  try {
    return JSON.parse(snapshotJson);
  } catch (_) {
    return null;
  }
}

export function clampPercent(value) {
  return Math.max(0, Math.min(100, value));
}

export function formatPercent(value) {
  const rounded = Math.round(value * 10) / 10;
  return Number.isInteger(rounded) ? `${rounded}%` : `${rounded.toFixed(1)}%`;
}

export function isValidDateInput(value) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return false;
  }
  const date = new Date(`${value}T00:00:00`);
  return !Number.isNaN(date.getTime()) && formatDateInput(date.getTime()) === value;
}

export function shortId(value) {
  if (!value || value.length <= 18) {
    return value || "id 未知";
  }
  return `${value.slice(0, 10)}...${value.slice(-6)}`;
}

export function formatDateTime(value) {
  if (!value) {
    return "未知";
  }
  const millis = normalizeEpochMillis(value);
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
    timeZoneName: "short",
  }).format(new Date(millis));
}

export function formatPlanDate(value) {
  const millis = normalizeEpochMillis(value);
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "numeric",
    day: "numeric",
  }).format(new Date(millis));
}

export function formatDateInput(value) {
  const millis = normalizeEpochMillis(value);
  const date = new Date(millis);
  const year = date.getFullYear();
  const month = `${date.getMonth() + 1}`.padStart(2, "0");
  const day = `${date.getDate()}`.padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function delay(ms) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function normalizeEpochMillis(value) {
  return value > 0 && value < 100000000000 ? value * 1000 : value;
}
