// cmd/encv-wasm-container/thin_shell_test.go
//
// 范式守卫：**wasm 内核必须是薄壳**。
//
// 为什么要这条锁：
//
//	加解密/容器格式的真源在 internal/v2（crypto / writer / reader / plugins）。
//	只要 wasm 侧也写了一份"怎么拼容器 / 怎么算长度 / 怎么派生密钥"，
//	主应用改了这些逻辑后 wasm 就得跟着重写 —— 而且两边漂移时**不报错**，
//	表现出来就是"浏览器加密的东西 CLI 解不开（还是静默乱码）"。
//
// 因此约定 wasm 侧只做三件事：
//  1. 把 js.Value 翻成 Go 参数（含类型校验）
//  2. 调用主线能力（pkg/encv / internal/v2/...）
//  3. 把结果翻回 js 值
//
// 出现 forbidden 里的符号 = 有人在 wasm 里重新实现了主线逻辑 → 红灯。
// 修法不是给名单开后门，而是**把那段逻辑下沉到主线、wasm 改为调用**。
package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// forbidden：wasm 侧不得出现的符号 → 该怎么改
var forbidden = map[string]string{
	// 密码学：一律走主线封装好的编排入口
	"crypto.EncryptSegment":        "段加密：改调主线编排入口",
	"crypto.WrapDEK":               "分层密钥封装：改调主线编排入口",
	"crypto.UnwrapDEK":             "分层密钥解封：主线 reader 的事，wasm 不要自己解",
	"crypto.DeriveKEK":             "密钥派生：改调主线编排入口",
	"crypto.DeriveCTRIV":           "CTR IV 派生：主线 reader 内部的事",
	"crypto.GenerateSalt_v2":       "盐生成：改调主线编排入口",
	"crypto.GenerateIV_v2":         "nonce 生成：改调主线编排入口",
	"crypto.GenerateMACSalt":       "mac salt 生成：改调主线编排入口",
	"crypto.CalculatePasswordHint": "口令提示：改调主线编排入口",
	"crypto.DeriveMACKey":          "mac key 派生：改调主线编排入口",

	// 容器布局：偏移/尺寸必须由 writer/reader 说了算
	"types.SegmentHeaderSize":   "段头尺寸：别自己算偏移，用主线能力",
	"types.EnvelopeHeaderSize":  "信封头尺寸：别自己算偏移，用主线能力",
	"types.SegmentHeader":       "段头解析：改调主线 reader",
	"types.EnvelopeFooterV4":    "footer 布局：改调主线 reader",
	"writer.WriteV4Container":   "写容器：改调主线编排入口（由它决定走哪条写入路径）",
	"writer.WriteV4ContainerTo": "写容器：改调主线编排入口",
	"writer.V4WriteParams":      "写入参数装配：下沉到主线编排入口",
	"writer.V4StreamParams":     "流式写入参数装配：下沉到主线编排入口",
	"writer.NewV4StreamWriter":  "流式写入器：下沉到主线编排入口，wasm 只喂字节",
	"types.Manifest_v4{":        "manifest 装配：下沉到主线编排入口",
	"types.Segment_v4{":         "段元数据装配：下沉到主线编排入口",

	// KVI / 插件 index 的内容同样是主线知识
	"textplugin.TextIndex": "插件 index 装配：下沉到主线编排入口",
}

func TestWasmKernelIsThinShell(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("读 main.go 失败：%v", err)
	}
	// 只看代码：注释里提到这些符号（解释性文字）不算违规
	body := stripComments(string(src))

	var violations []string
	for sym, how := range forbidden {
		idx := strings.Index(body, sym)
		if idx < 0 {
			continue
		}
		line := strings.Count(body[:idx], "\n") + 1
		violations = append(violations, fmt.Sprintf("main.go:%d  %s → %s", line, sym, how))
	}

	if len(violations) > 0 {
		// 排序保证输出稳定
		sortStrings(violations)
		t.Errorf(
			"wasm 内核里长了主线逻辑（%d 处）：主应用改加解密后会两边漂移且不报错，必须下沉。\n  %s",
			len(violations), strings.Join(violations, "\n  "),
		)
	}
}

// stripComments 去掉整行注释与块注释，避免注释里的说明文字被误判成违规。
func stripComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
