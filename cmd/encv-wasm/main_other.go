//go:build !js

// 非 js/wasm 平台的占位入口。
//
// 存在的原因：cmd/encv-wasm 只有 //go:build js && wasm 的实现文件，
// 若没有这个占位文件，本仓常规的 `go build ./...` / `go vet ./...` 会以
// "build constraints exclude all Go files" 失败。它不参与任何加密逻辑。
package main

import "fmt"

func main() {
	fmt.Println("encv-wasm: 该程序只在 GOOS=js GOARCH=wasm 下有意义。")
	fmt.Println("构建方式：GOOS=js GOARCH=wasm go build -o encv.wasm ./cmd/encv-wasm")
	fmt.Println("（推荐直接用 make wasm / bash scripts/build-wasm.sh）")
}
