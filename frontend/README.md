# Frontend

本目录保存前端源码和静态资源。MVP 使用原生 HTML、CSS 和 JavaScript，不引入前端构建链。

- `static/`：浏览器直接加载的静态资源。

前端独立于后端 HTTP server 部署或运行，通过配置的 API 地址访问后端，不在 `localStorage`、`sessionStorage` 或 IndexedDB 中保存账号数据、token 或完整凭证。
