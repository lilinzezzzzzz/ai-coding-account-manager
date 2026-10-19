import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { runInNewContext } from "node:vm";

const source = readFileSync(new URL("../static/theme.js", import.meta.url), "utf8");
function page({ stored = null, dark = false, blocked = false } = {}) {
  const events = {};
  const values = new Map(stored === null ? [] : [["account-manager.theme", stored]]);

  const document = {
    documentElement: { dataset: {} },
    dispatchEvent: () => {},
    addEventListener: (name, fn) => { events[name] = fn; },
  };
  const media = { matches: dark, addEventListener: (name, fn) => { events.media = fn; } };
  const storage = {
    getItem: (key) => { if (blocked) throw Error("blocked"); return values.get(key) ?? null; },
    setItem: (key, value) => { if (blocked) throw Error("blocked"); values.set(key, value); },
    removeItem: (key) => { if (blocked) throw Error("blocked"); values.delete(key); },
  };
  runInNewContext(source, { document, CustomEvent: class { constructor(type) { this.type = type; } }, window: {
    matchMedia: () => media, localStorage: storage,
    addEventListener: (name, fn) => { events.storage = fn; },
  } });
  const initial = document.documentElement.dataset.theme;
  return {
    initial, values,
    preference: () => document.documentElement.dataset.themePreference,
    theme: () => document.documentElement.dataset.theme,
    select: (value) => events["theme-preference-change"]({ detail: value }),
    system: (value) => { media.matches = value; events.media(); },
    storage: (key, newValue) => events.storage({ key, newValue }),
  };
}

test("default follows the OS before DOM ready and on live changes", () => {
  const p = page({ dark: true });
  assert.equal(p.initial, "dark");
  assert.equal(p.preference(), "system");
  p.system(false); assert.equal(p.theme(), "light");
  p.system(true); assert.equal(p.theme(), "dark");
});
test("manual themes persist, survive OS changes and reload, and can return to system", () => {
  const p = page();
  p.select("dark"); assert.equal(p.theme(), "dark");
  p.system(true); p.system(false); assert.equal(p.theme(), "dark");
  assert.equal(page({ stored: p.values.get("account-manager.theme") }).initial, "dark");
  p.select("light"); p.system(true); assert.equal(p.theme(), "light");
  p.select("system"); assert.equal(p.theme(), "dark");
  assert.equal(p.values.has("account-manager.theme"), false);
});
test("storage failures and invalid preferences do not prevent switching", () => {
  assert.equal(page({ stored: "invalid", dark: true }).initial, "dark");
  const p = page({ blocked: true });
  p.select("dark"); assert.equal(p.theme(), "dark");
  p.select("system"); assert.equal(p.theme(), "light");
});
test("other tabs synchronize preference updates and clearing", () => {
  const p = page({ dark: true });
  p.storage("unrelated", "light"); assert.equal(p.theme(), "dark");
  p.storage("account-manager.theme", "light"); assert.equal(p.theme(), "light");
  assert.equal(p.preference(), "light");
  p.storage(null, null); assert.equal(p.theme(), "dark");
  assert.equal(p.preference(), "system");
});
