# Frontend

前端界面以 **Tabler 1.7.5** 为主要 UI 样式基础。账号卡片、按钮、徽标、下拉菜单、
表单控件和额度进度条复用 Tabler 的样式类。页面布局和业务组件的样式调整集中在
[`app.css`](static/app.css)。

Go 服务直接提供静态 HTML、CSS 和 JavaScript，运行应用无需安装 npm 依赖、构建前端或
连接 CDN。Tabler 的预编译 CSS 和 MIT 许可证保存在 `static/vendor/tabler-1.7.5/`。
菜单交互、原生 `<dialog>` 弹窗、主题切换和业务请求由项目的 JavaScript 实现。
资源来源、校验值和升级步骤见 [Tabler 资源说明](static/vendor/tabler-1.7.5/README.md)。

- 页面入口负责初始化，并协调应用状态、账号操作和渲染。[`app.js`](static/app.js)
- 界面组件构建服务商分区、账号卡片、额度和外观控件。添加账号、账号操作和外观菜单共用键盘与焦点逻辑。[组件目录](static/components/) · [`popup-menu.js`](static/components/popup-menu.js)
- API 模块处理 HTTP 请求和响应。登录任务模块负责创建、轮询和状态文案。[`api.js`](static/api.js) · [`login-tasks.js`](static/login-tasks.js)
- `tests/`：使用 Node 内置测试运行器的回归测试。在仓库根目录先执行 `nvm use`，再运行
  `node --test frontend/tests/*.test.mjs`。Node.js 环境初始化见[开发环境](../README.md#开发环境)。

前端独立于后端 HTTP server 部署或运行，通过配置的 API 地址访问后端，不在 `localStorage`、`sessionStorage` 或 IndexedDB 中保存账号数据、token 或完整凭证。

外观支持跟随系统、浅色和深色，默认跟随系统。手动主题偏好仅保存在浏览器 `localStorage` 的 `account-manager.theme` 中，选择跟随系统会清除该偏好；不保存账号数据或凭据。
