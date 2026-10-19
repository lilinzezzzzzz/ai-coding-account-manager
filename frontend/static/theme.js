// Apply the preference before styles load to avoid a flash of the wrong theme.
(() => {
  const storageKey = "account-manager.theme";
  const systemTheme = window.matchMedia("(prefers-color-scheme: dark)");
  const normalize = (value) => ["light", "dark"].includes(value) ? value : "system";
  let preference = "system";
  try {
    preference = normalize(window.localStorage.getItem(storageKey));
  } catch {
    // Theme switching remains available when browser storage is disabled.
  }

  function applyTheme() {
    document.documentElement.dataset.theme = preference === "system"
      ? (systemTheme.matches ? "dark" : "light")
      : preference;
    document.documentElement.dataset.themePreference = preference;
    document.dispatchEvent(new CustomEvent("theme-change"));
  }

  applyTheme();
  systemTheme.addEventListener("change", applyTheme);
  window.addEventListener("storage", (event) => {
    if (event.key !== storageKey && event.key !== null) return;
    preference = normalize(event.newValue);
    applyTheme();
  });
  document.addEventListener("theme-preference-change", (event) => {
    preference = normalize(event.detail);
    applyTheme();
    try {
      if (preference === "system") {
        window.localStorage.removeItem(storageKey);
      } else {
        window.localStorage.setItem(storageKey, preference);
      }
    } catch {
      // Keep the selected theme for this page even if persistence is blocked.
    }
  });
})();
