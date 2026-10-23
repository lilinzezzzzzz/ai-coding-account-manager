import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { runInNewContext } from "node:vm";

const source = readFileSync(new URL("../static/clipboard.js", import.meta.url), "utf8").replace("export async function", "async function");
function copier(clipboard, ClipboardItem) {
  const context = { navigator: { clipboard }, ClipboardItem, Blob };
  runInNewContext(source, context);
  return context.copyTextFrom;
}
class TestClipboardItem {
  constructor(data) { this.data = data; }
}

test("requests clipboard write during the click before asynchronous credentials arrive", async () => {
  let resolveText;
  const text = new Promise((resolve) => { resolveText = resolve; });
  let started = false;
  let copied;
  const copy = copier({ write: async ([item]) => {
    started = true;
    copied = await (await item.data["text/plain"]).text();
  } }, TestClipboardItem);
  let finished = false;
  const pending = copy(() => text).then(() => { finished = true; });
  assert.equal(started, true);
  assert.equal(finished, false);
  const payload = '{\n "tokens": {"access_token":"test-only"}, "unknown":true\n}\n';
  resolveText(payload);
  await pending;
  assert.equal(copied, payload);
  assert.equal(finished, true);
});

test("unavailable clipboard fails before fetching credentials", async () => {
  const copy = copier(undefined, undefined);
  await assert.rejects(copy(() => { assert.fail("must not fetch credentials"); }), /当前浏览器不支持复制/);
});

test("missing credentials preserves the export error and never copies a placeholder", async () => {
  let copied = false;
  const copy = copier({ write: async ([item]) => {
    await item.data["text/plain"];
    copied = true;
  } }, TestClipboardItem);
  await assert.rejects(copy(async () => { throw new Error("此账号没有可导出的登录凭据"); }), /没有可导出/);
  assert.equal(copied, false);
});

test("permission denial reports failure without exposing browser error details", async () => {
  const copy = copier({ write: async () => { throw new Error("test-secret-browser-detail"); } }, TestClipboardItem);
  await assert.rejects(copy(async () => "test-only"), { message: "复制失败，请允许浏览器写入剪贴板后重试" });
});

test("synchronous clipboard rejection and later export failure are handled", async () => {
  const copy = copier({ write: () => { throw new Error("denied"); } }, TestClipboardItem);
  await assert.rejects(copy(async () => { throw new Error("missing credentials"); }));
  await new Promise((resolve) => setImmediate(resolve));
});

test("writeText fallback waits for content and propagates write failures", async () => {
  let copied;
  const copy = copier({ writeText: async (text) => { copied = text; } }, undefined);
  await copy(async () => "test-only-json");
  assert.equal(copied, "test-only-json");
  const denied = copier({ writeText: async () => { throw new Error("denied"); } }, undefined);
  await assert.rejects(denied(async () => "test-only-json"), /复制失败/);
});
