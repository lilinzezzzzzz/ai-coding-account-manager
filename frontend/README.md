# Frontend

本目录保存前端源码和静态资源。MVP 使用原生 HTML、CSS 和 JavaScript，不引入前端构建链。

界面使用 Tabler 1.7.5 的预编译 CSS；资源和 MIT 许可证保存在
`static/vendor/tabler-1.7.5/`，由本地服务提供，无运行时 CDN、字体或 npm 依赖。
仅引入核心样式，菜单、原生 `<dialog>`、主题切换和业务请求仍由现有 JavaScript 管理，
不加载 Tabler JavaScript 或第三方插件。来源、校验值及更新方法见
[Tabler 资源说明](static/vendor/tabler-1.7.5/README.md)。

- `static/`：浏览器直接加载的静态资源。
- `tests/`：使用 Node 内置测试运行器的回归测试。在仓库根目录先执行 `nvm use`，再运行
  `node --test frontend/tests/*.test.mjs`。Node.js 环境初始化见[开发环境](../README.md#开发环境)。

前端独立于后端 HTTP server 部署或运行，通过配置的 API 地址访问后端，不在 `localStorage`、`sessionStorage` 或 IndexedDB 中保存账号数据、token 或完整凭证。

外观支持跟随系统、浅色和深色，默认跟随系统。手动主题偏好仅保存在浏览器 `localStorage` 的 `account-manager.theme` 中，选择跟随系统会清除该偏好；不保存账号数据或凭据。
