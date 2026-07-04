package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/preview/*
var previewFS embed.FS

// PreviewHandler 返回一个用于提供预览页面的 http.Handler
func PreviewHandler() http.Handler {
	// 创建一个 sub FS，只包含 static/preview 目录下的内容
	subFS, err := fs.Sub(previewFS, "static/preview")
	if err != nil {
		// 这个错误在编译时就会发生，如果路径不对的话
		panic(err)
	}
	// 使用 http.FileServer 来提供文件
	return http.FileServer(http.FS(subFS))
}
