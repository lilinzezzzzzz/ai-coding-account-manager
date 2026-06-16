# Frontend

本目录保存前端源码和静态资源。MVP 使用原生 HTML、CSS 和 JavaScript，不引入前端构建链。

- `static/`：浏览器直接加载的静态资源。
- `assets.go`：将 `static/` 嵌入 Go 二进制，并只暴露 `fs.FS` 给后端入口。

前端通过同源 API 访问后端，不在 `localStorage`、`sessionStorage` 或 IndexedDB 中保存账号数据、token、Cookie、CSRF token 或完整凭证。
