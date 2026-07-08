let tooltipShowDelayMs = 300;
const tooltipHideDelayMs = 150;
let tooltipShowTimer = 0;
let tooltipHideTimer = 0;
let tooltipTarget = null;
let tooltipPreviousDescribedBy = null;
let tooltipNode = null;
let tooltipEventsBound = false;

export function setupTooltips(options = {}) {
  tooltipShowDelayMs = options.showDelayMs ?? tooltipShowDelayMs;
  if (tooltipEventsBound) {
    return;
  }
  tooltipEventsBound = true;
  document.addEventListener("pointerover", handleTooltipPointerOver);
  document.addEventListener("pointerout", handleTooltipPointerOut);
  document.addEventListener("focusin", handleTooltipFocusIn);
  document.addEventListener("focusout", handleTooltipFocusOut);
  window.addEventListener("scroll", hideTooltip, true);
  window.addEventListener("resize", hideTooltip);
}

export function setTooltip(element, text) {
  element.dataset.tooltip = text;
}

export function hideTooltip() {
  if (tooltipShowTimer) {
    window.clearTimeout(tooltipShowTimer);
    tooltipShowTimer = 0;
  }
  if (tooltipHideTimer) {
    window.clearTimeout(tooltipHideTimer);
    tooltipHideTimer = 0;
  }
  if (tooltipTarget) {
    if (tooltipPreviousDescribedBy) {
      tooltipTarget.setAttribute("aria-describedby", tooltipPreviousDescribedBy);
    } else {
      tooltipTarget.removeAttribute("aria-describedby");
    }
  }
  tooltipTarget = null;
  tooltipPreviousDescribedBy = null;
  if (tooltipNode) {
    tooltipNode.hidden = true;
  }
}

function handleTooltipPointerOver(event) {
  const target = tooltipTrigger(event.target);
  if (!target || target.contains(event.relatedTarget)) {
    return;
  }
  scheduleTooltip(target);
}

function handleTooltipPointerOut(event) {
  const target = tooltipTrigger(event.target);
  if (!target || target.contains(event.relatedTarget)) {
    return;
  }
  scheduleHideTooltip(event.relatedTarget);
}

function handleTooltipFocusIn(event) {
  const target = tooltipTrigger(event.target);
  if (target) {
    scheduleTooltip(target);
  }
}

function handleTooltipFocusOut(event) {
  const target = tooltipTrigger(event.target);
  if (target) {
    hideTooltip();
  }
}

function tooltipTrigger(target) {
  return target instanceof Element ? target.closest("[data-tooltip]") : null;
}

function scheduleTooltip(target) {
  const text = target.dataset.tooltip;
  if (!text) {
    return;
  }
  cancelTooltipHide();
  if (tooltipTarget === target && tooltipNode && !tooltipNode.hidden) {
    return;
  }
  hideTooltip();
  tooltipTarget = target;
  tooltipShowTimer = window.setTimeout(() => showTooltip(target, text), tooltipShowDelayMs);
}

function scheduleHideTooltip(relatedTarget) {
  if (isTooltipOrTarget(relatedTarget)) {
    return;
  }
  cancelTooltipHide();
  tooltipHideTimer = window.setTimeout(hideTooltip, tooltipHideDelayMs);
}

function cancelTooltipHide() {
  if (tooltipHideTimer) {
    window.clearTimeout(tooltipHideTimer);
    tooltipHideTimer = 0;
  }
}

function showTooltip(target, text) {
  if (!document.contains(target)) {
    hideTooltip();
    return;
  }
  const node = ensureTooltipNode();
  node.textContent = text;
  node.hidden = false;
  node.id = "app-tooltip";
  tooltipPreviousDescribedBy = target.getAttribute("aria-describedby");
  target.setAttribute("aria-describedby", node.id);
  positionTooltip(node, target);
}

function ensureTooltipNode() {
  if (!tooltipNode) {
    tooltipNode = document.createElement("div");
    tooltipNode.className = "app-tooltip";
    tooltipNode.setAttribute("role", "tooltip");
    tooltipNode.hidden = true;
    tooltipNode.addEventListener("pointerover", cancelTooltipHide);
    tooltipNode.addEventListener("pointerout", (event) => {
      if (!tooltipNode.contains(event.relatedTarget)) {
        scheduleHideTooltip(event.relatedTarget);
      }
    });
    document.body.append(tooltipNode);
  }
  return tooltipNode;
}

function isTooltipOrTarget(node) {
  if (!(node instanceof Node)) {
    return false;
  }
  return Boolean((tooltipNode && tooltipNode.contains(node)) || (tooltipTarget && tooltipTarget.contains(node)));
}

function positionTooltip(node, target) {
  const gap = 8;
  const margin = 8;
  const rect = target.getBoundingClientRect();
  const tooltipRect = node.getBoundingClientRect();
  const maxLeft = window.innerWidth - tooltipRect.width - margin;
  const maxTop = window.innerHeight - tooltipRect.height - margin;
  const left = Math.max(margin, Math.min(rect.left + rect.width / 2 - tooltipRect.width / 2, maxLeft));
  const topCandidate = rect.top - tooltipRect.height - gap;
  const bottomCandidate = rect.bottom + gap;
  const preferredTop =
    topCandidate >= margin && !overlapsReservedControls(left, topCandidate, tooltipRect, target)
      ? topCandidate
      : bottomCandidate;
  const top = Math.max(margin, Math.min(preferredTop, maxTop));
  node.style.left = `${left}px`;
  node.style.top = `${top}px`;
}

function overlapsReservedControls(left, top, tooltipRect, target) {
  const tooltipBox = {
    left,
    top,
    right: left + tooltipRect.width,
    bottom: top + tooltipRect.height,
  };
  return Array.from(document.querySelectorAll(".toolbar, .add-menu")).some((element) => {
    if (element.contains(target)) {
      return false;
    }
    const rect = element.getBoundingClientRect();
    return boxesOverlap(tooltipBox, rect);
  });
}

function boxesOverlap(a, b) {
  return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
}
