const messageAutoHideMs = 4200;
const errorMessageAutoHideMs = 7000;

export function createStatusMessage(element) {
  let hideTimer;

  function hide() {
    window.clearTimeout(hideTimer);
    element.hidden = true;
    element.textContent = "";
    element.classList.remove("error");
  }

  function show(text, isError = false) {
    window.clearTimeout(hideTimer);
    element.hidden = false;
    element.textContent = text;
    element.classList.toggle("error", isError);
    hideTimer = window.setTimeout(hide, isError ? errorMessageAutoHideMs : messageAutoHideMs);
  }

  return { show, hide };
}
