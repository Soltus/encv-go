//go:build js && wasm

// Command encv-wasm 把 ENCV 的加密层编译成 WebAssembly，供浏览器 / Worker 调用。
//
// 关键点：**这里不实现任何密码学逻辑**，只是把 internal/v2/crypto（以及其上的
// internal/v2/crypto/simple 轻量信封）通过 syscall/js 暴露给 JS。
// 因此 WASM 侧与 Go 原生侧、CLI 侧共用同一份算法实现，天然字节级对等，
// 不存在「前端再写一套、以后慢慢漂移」的可能。
//
// 构建：
//
//	GOOS=js GOARCH=wasm go build -o encv.wasm ./cmd/encv-wasm
//	cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
//
// 产物在 JS 侧挂载为 globalThis.encvCrypto，全部方法返回「结果对象」而不是抛异常
// （JS 异常跨越 syscall/js 边界会让 wasm 实例直接崩溃，不可恢复）：
//
//	{ ok: true,  data: Uint8Array }   // 或 { ok: true, text: string }
//	{ ok: false, error: "..." }
package main

import (
	"encoding/hex"
	"fmt"
	"syscall/js"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/crypto/simple"
)

// version 与仓库构建信息无关，仅用于排障时确认 wasm 与 SDK 版本是否匹配。
const version = "1"

func main() {
	js.Global().Set("encvCrypto", js.ValueOf(map[string]interface{}{
		"version":            js.FuncOf(versionFn),
		"constants":          js.FuncOf(constantsFn),
		"encrypt":            js.FuncOf(encryptFn),
		"encryptWithEntropy": js.FuncOf(encryptWithEntropyFn),
		"decrypt":            js.FuncOf(decryptFn),
		"encryptText":        js.FuncOf(encryptTextFn),
		"decryptText":        js.FuncOf(decryptTextFn),
		"verifyPassword":     js.FuncOf(verifyPasswordFn),
		"deriveKey":          js.FuncOf(deriveKeyFn),
		"parseHeader":        js.FuncOf(parseHeaderFn),
		"randomBytes":        js.FuncOf(randomBytesFn),
	}))

	// 保持 wasm 实例存活：main 返回后所有导出的回调都会失效。
	<-make(chan struct{})
}

// ────────────────────────────── 参数 / 结果辅助 ──────────────────────────────

// toBytes 把 JS 的 Uint8Array 拷进 Go 内存；nil/undefined 视作空切片。
//
// 这里刻意做成「返回 error」而不是 panic：一旦 JS 传错类型，
// panic 会顺着 syscall/js 把整个 wasm 实例打死，Worker 里所有后续调用全部失效。
// 参数错误必须只是这一次调用失败。
func toBytes(v js.Value) ([]byte, error) {
	if v.IsUndefined() || v.IsNull() {
		return nil, nil
	}
	if v.Type() != js.TypeObject {
		return nil, errBadArgs("expected Uint8Array, got " + v.Type().String())
	}
	length := v.Get("length")
	if length.Type() != js.TypeNumber {
		return nil, errBadArgs("expected Uint8Array (no length property)")
	}
	n := length.Int()
	if n <= 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	if copied := js.CopyBytesToGo(buf, v); copied != n {
		return nil, errBadArgs("failed to copy bytes into wasm memory")
	}
	return buf, nil
}

func okBytes(b []byte) map[string]interface{} {
	out := js.Global().Get("Uint8Array").New(len(b))
	if len(b) > 0 {
		js.CopyBytesToJS(out, b)
	}
	return map[string]interface{}{"ok": true, "data": out}
}

func okText(s string) map[string]interface{} {
	return map[string]interface{}{"ok": true, "text": s}
}

func okValue(v interface{}) map[string]interface{} {
	return map[string]interface{}{"ok": true, "value": v}
}

func fail(err error) map[string]interface{} {
	return map[string]interface{}{"ok": false, "error": err.Error()}
}

// readOptions 读取可选参数对象。字段类型不对时回退默认值（不报错）：
// 可选参数的意义就是「可以不给」，给错了按没给处理比中断调用更符合预期。
func readOptions(v js.Value) simple.Options {
	opts := simple.DefaultOptions()
	if v.IsUndefined() || v.IsNull() || v.Type() != js.TypeObject {
		return opts
	}
	if f := v.Get("cipherMode"); f.Type() == js.TypeNumber {
		opts.CipherMode = uint16(f.Int())
	}
	if f := v.Get("iterations"); f.Type() == js.TypeNumber {
		opts.Iterations = uint32(f.Int())
	}
	if f := v.Get("saltLen"); f.Type() == js.TypeNumber {
		opts.SaltLen = uint8(f.Int())
	}
	return opts
}

// ────────────────────────────── 导出函数 ──────────────────────────────

// versionFn: () => { ok, value: string }
//
// 与其它导出一样包成结果对象：JS 侧（src/core.ts）对所有导出用同一套 unwrap，
// 谁返回裸值谁就会被当成失败（"encv wasm: unknown error"）。
func versionFn(js.Value, []js.Value) interface{} {
	return okValue(version)
}

// constantsFn 把信封常量暴露给 JS，避免前端硬编码一份（硬编码 = 漂移的开始）。
func constantsFn(js.Value, []js.Value) interface{} {
	// 与其它导出一样包成结果对象，JS 侧只用一套解包逻辑。
	return okValue(map[string]interface{}{
		"magic":             simple.Magic,
		"formatVersion":     simple.FormatVersion,
		"headerSize":        simple.HeaderSize,
		"hintSize":          simple.HintSize,
		"defaultSaltLen":    int(simple.DefaultSaltLen),
		"defaultIVLen":      int(simple.DefaultIVLen),
		"defaultIterations": int(simple.DefaultIterations),
		"cipherAES128CTR":   int(simple.CipherAES128CTR),
		"cipherAES256CTR":   int(simple.CipherAES256CTR),
		"defaultCipherMode": int(simple.DefaultCipherMode),
		"kdfPBKDF2SHA256":   int(simple.KDFPBKDF2SHA256),
		"algorithm":         crypto.Algorithm_v2,
	})
}

// encryptFn: (plain: Uint8Array, password: string, opts?: object) => { ok, data }
func encryptFn(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return fail(errBadArgs("encrypt(plain, password, opts?)"))
	}
	plain, err := toBytes(args[0])
	if err != nil {
		return fail(err)
	}
	password, err := argString(args, 1)
	if err != nil {
		return fail(err)
	}
	blob, err := simple.Encrypt(plain, password, readOptions(argOr(args, 2)))
	if err != nil {
		return fail(err)
	}
	return okBytes(blob)
}

// encryptWithEntropyFn: (plain, password, opts, salt: Uint8Array, iv: Uint8Array) => { ok, data }
//
// 用调用方给定的 salt/iv 加密，只用于黄金向量比对（前端测试复现 Go 产出的字节）。
// 业务代码一律用 encrypt()，不要自己传熵。
func encryptWithEntropyFn(_ js.Value, args []js.Value) interface{} {
	if len(args) < 5 {
		return fail(errBadArgs("encryptWithEntropy(plain, password, opts, salt, iv)"))
	}
	plain, err := toBytes(args[0])
	if err != nil {
		return fail(err)
	}
	password, err := argString(args, 1)
	if err != nil {
		return fail(err)
	}
	salt, err := toBytes(args[3])
	if err != nil {
		return fail(err)
	}
	iv, err := toBytes(args[4])
	if err != nil {
		return fail(err)
	}
	blob, err := simple.EncryptWithEntropy(plain, password, readOptions(args[2]), salt, iv)
	if err != nil {
		return fail(err)
	}
	return okBytes(blob)
}

// decryptFn: (blob: Uint8Array, password: string) => { ok, data }
func decryptFn(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return fail(errBadArgs("decrypt(blob, password)"))
	}
	blob, err := toBytes(args[0])
	if err != nil {
		return fail(err)
	}
	password, err := argString(args, 1)
	if err != nil {
		return fail(err)
	}
	plain, err := simple.Decrypt(blob, password)
	if err != nil {
		return fail(err)
	}
	return okBytes(plain)
}

// encryptTextFn: (text: string, password: string, opts?: object) => { ok, data }
func encryptTextFn(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return fail(errBadArgs("encryptText(text, password, opts?)"))
	}
	text, err := argString(args, 0)
	if err != nil {
		return fail(err)
	}
	password, err := argString(args, 1)
	if err != nil {
		return fail(err)
	}
	blob, err := simple.EncryptString(text, password, readOptions(argOr(args, 2)))
	if err != nil {
		return fail(err)
	}
	return okBytes(blob)
}

// decryptTextFn: (blob: Uint8Array, password: string) => { ok, text }
func decryptTextFn(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return fail(errBadArgs("decryptText(blob, password)"))
	}
	blob, err := toBytes(args[0])
	if err != nil {
		return fail(err)
	}
	password, err := argString(args, 1)
	if err != nil {
		return fail(err)
	}
	text, err := simple.DecryptString(blob, password)
	if err != nil {
		return fail(err)
	}
	return okText(text)
}

// verifyPasswordFn: (blob: Uint8Array, password: string) => { ok, value: boolean }
//
// 2026-09-27 修正：此前这里返回裸 bool，坏参数也静默返回 false，
// 与其余导出「统一返回结果对象」的契约不一致 —— JS 侧统一的 unwrap 会把它判成失败，
// 前端校验口令时拿到的是 "encv wasm: unknown error" 而不是 true/false。
func verifyPasswordFn(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return fail(errBadArgs("verifyPassword(blob, password)"))
	}
	blob, err := toBytes(args[0])
	if err != nil {
		return fail(err)
	}
	password, err := argString(args, 1)
	if err != nil {
		return fail(err)
	}
	return okValue(simple.VerifyPassword(blob, password))
}

// deriveKeyFn: (password: string, salt: Uint8Array, keyLen: number) => { ok, data }
//
// 直接转发 crypto.GenerateKey，供需要自行实现信封的高级调用方对齐密钥派生。
func deriveKeyFn(_ js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return fail(errBadArgs("deriveKey(password, salt, keyLen)"))
	}
	password, err := argString(args, 0)
	if err != nil {
		return fail(err)
	}
	salt, err := toBytes(args[1])
	if err != nil {
		return fail(err)
	}
	keyLen := args[2]
	if keyLen.Type() != js.TypeNumber {
		return fail(errBadArgs("keyLen must be a number"))
	}
	return okBytes(crypto.GenerateKey(password, salt, keyLen.Int()))
}

// parseHeaderFn: (blob: Uint8Array) => { ok, value }
func parseHeaderFn(_ js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return fail(errBadArgs("parseHeader(blob)"))
	}
	blob, err := toBytes(args[0])
	if err != nil {
		return fail(err)
	}
	h, err := simple.ParseHeader(blob)
	if err != nil {
		return fail(err)
	}
	return okValue(map[string]interface{}{
		"magic":         string(h.Magic[:]),
		"formatVersion": h.FormatVersion,
		"cipherMode":    h.CipherMode,
		"kdf":           h.KDF,
		"iterations":    h.Iterations,
		"saltLen":       h.SaltLen,
		"ivLen":         h.IVLen,
		"hintLen":       h.HintLen,
		"hintHex":       hex.EncodeToString(h.Hint[:]),
		"saltHex":       hex.EncodeToString(h.Salt(blob)),
		"ivHex":         hex.EncodeToString(h.IV(blob)),
		"payloadOffset": h.PayloadOffset(),
		"payloadLength": len(h.Payload(blob)),
	})
}

// randomBytesFn: (n: number) => { ok, data }，走加密层同一个 crypto/rand。
func randomBytesFn(_ js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return fail(errBadArgs("randomBytes(n)"))
	}
	if args[0].Type() != js.TypeNumber {
		return fail(errBadArgs("n must be a number"))
	}
	n := args[0].Int()
	if n <= 0 {
		return okBytes(nil)
	}
	buf, err := crypto.GenerateSalt_v2(n)
	if err != nil {
		return fail(err)
	}
	return okBytes(buf)
}

// ────────────────────────────── 其它 ──────────────────────────────

func argOr(args []js.Value, i int) js.Value {
	if len(args) > i {
		return args[i]
	}
	return js.Undefined()
}

// argString 取字符串参数。类型不符返回错误而不是 panic（panic 会打死整个 wasm 实例）。
func argString(args []js.Value, i int) (string, error) {
	v := argOr(args, i)
	if v.Type() != js.TypeString {
		return "", errBadArgs(fmt.Sprintf("arg %d must be a string", i))
	}
	return v.String(), nil
}

func errBadArgs(usage string) error {
	return errString("bad arguments, usage: " + usage)
}

// errString 让上面的辅助函数无需引入 errors 包即可构造错误。
type errString string

func (e errString) Error() string { return string(e) }
