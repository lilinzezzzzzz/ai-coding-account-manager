# Tabler 1.7.5

本目录仅保存 `@tabler/core` 的预编译核心 CSS 和 MIT 许可证。
应用不需要安装 npm 包、构建前端或连接 CDN；不包含 Tabler JS、字体及第三方插件。

- 上游：[tabler/tabler](https://github.com/tabler/tabler)
- 固定版本：[1.7.5](https://github.com/tabler/tabler/releases/tag/%40tabler/core%401.7.5)
- CSS 来源：<https://cdn.jsdelivr.net/npm/@tabler/core@1.7.5/dist/css/tabler.min.css>
- 许可证来源：<https://github.com/tabler/tabler/blob/%40tabler/core%401.7.5/LICENSE>
- 上游 CSS SHA-256：`d1708c69e6b07a5a3138b8549bf64e00cd9d18c130b9b2ba2d9e294956f7c564`
- 本地 CSS SHA-256：`7b82b51d4be98e37d2aeca39f728289031aa14692c5523ed2788974b7e48b376`

唯一的本地处理是移除末尾 `sourceMappingURL` 注释，避免开发者工具请求未分发的
source map；保留上游版权注释，应用定制全部位于 `../../app.css`。

升级时，新建带版本号的目录，下载固定版本 CSS 和对应许可证，记录校验值与本地处理，
更新 `index.html` 中的引用。检查上游浏览器支持要求，并验证深浅主题、账号菜单、
弹窗和窄屏布局后，再移除旧版本目录。Tabler 的浏览器支持见
[官方说明](https://docs.tabler.io/ui/getting-started/browser-support)。
