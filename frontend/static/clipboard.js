// 在点击调用栈中申请剪贴板写入，让异步加载不丢失浏览器的用户手势。
export async function copyTextFrom(loadText) {
  const clipboard = globalThis.navigator?.clipboard;
  const canWriteItem = typeof clipboard?.write === "function" && typeof globalThis.ClipboardItem === "function";
  if (!canWriteItem && typeof clipboard?.writeText !== "function") {
    throw new Error("当前浏览器不支持复制，请使用支持剪贴板的浏览器访问本地页面");
  }

  const textPromise = Promise.resolve().then(loadText);
  if (canWriteItem) {
    const blobPromise = textPromise.then((text) => new Blob([text], { type: "text/plain" }));
    // 某些浏览器会在读取数据前拒绝权限，仍需处理数据 Promise 的拒绝。
    void blobPromise.catch(() => {});
    const writePromise = writeClipboard(() => clipboard.write([
      new ClipboardItem({ "text/plain": blobPromise }),
    ]));
    await Promise.all([textPromise, writePromise]);
    return;
  }

  const text = await textPromise;
  await writeClipboard(() => clipboard.writeText(text));
}

async function writeClipboard(write) {
  try {
    await write();
  } catch {
    throw new Error("复制失败，请允许浏览器写入剪贴板后重试");
  }
}
