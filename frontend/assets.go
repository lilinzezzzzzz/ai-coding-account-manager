package frontend

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var staticFiles embed.FS

// StaticFS 返回嵌入二进制的前端静态资源。
func StaticFS() (fs.FS, error) {
	return fs.Sub(staticFiles, "static")
}
