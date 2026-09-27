//go:build js && wasm

// cmd/encv-wasm-container —— ENCV v4 容器的浏览器端解密内核（Go → WASM）。
//
// 目标：让浏览器**不依赖 Go 后端**就能读 ENCV v4 容器（.sccgv/.sccga/.sccgi/.sccgp/.sccgt/.sccgwps），
// 与 CLI / 主应用完全同质：编译的是同一份 internal/v2 代码，不是另写一套。
//
// 目前暴露的能力（第一版，先打通"能编译 + 能解出正确字节"）：
//
//	encvContainer.open(bytes, password) -> handle
//	encvContainer.info(handle)          -> 容器元信息 JSON
//	encvContainer.readRange(handle, off, len) -> Uint8Array（按明文偏移随机读，视频拖动靠它）
//	encvContainer.close(handle)
//
// 所有导出都返回「结果对象」 {ok, ...} 或 {ok:false, error}，坏参数只让本次调用失败。
package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"syscall/js"

	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	textplugin "github.com/Soltus/encv-go/internal/v2/plugins/text"

	// 插件包靠 init() 把各自的 index kind 注册进主线 KVI 注册表；
	// 不加载就只有注册过的那几种能解（未注册时报 "unknown index kind"）。
	// wasm 与主应用对等 = 把同一批插件都加载进来。
	_ "github.com/Soltus/encv-go/internal/v2/plugins/audio"
	_ "github.com/Soltus/encv-go/internal/v2/plugins/image"
	_ "github.com/Soltus/encv-go/internal/v2/plugins/pdf"
	_ "github.com/Soltus/encv-go/internal/v2/plugins/video"
	_ "github.com/Soltus/encv-go/internal/v2/plugins/wps"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

// opened 持有一个已打开的容器。
//
// 两套主线栈都可能，按容器**结构**分派，不是按版本号猜：
//   - fragments：逻辑分片层（插件栈产出的容器，manifest 里是 Fragments）
//   - segments：物理段层（v4 writer 产出的容器，manifest 里是 Segments + Playlists）
//
// ⚠️ v2/v4 是**信封版本**，与上面这套"内容组织层"不是同一个维度：
// 同一个 v4 信封上两种组织方式都存在，混为一谈就会拿错的 reader 去读对的容器。
type opened struct {
	kind     string                  // "fragment" | "segment"
	info     *reader.V4ContainerInfo // segment 栈
	manifest *types.Manifest_v4

	// fragment 栈：主线给的是顺序解密流，随机读在已读出的明文字节上切片完成
	fragReader io.ReadCloser
	plainCache []byte
	plainSize  int64
}

var (
	nextHandle int
	openedMap  = map[int]*opened{}
)

// openContainer 用**主线 reader**打开内存里的容器，按结构选对应的栈。
//
// 不自研段解析/密钥派生：两条路径都是主线代码（reader.NewEncryptedContainerReaderFromSource
// 与 reader.OpenV4ContainerFromSource），主线改了这里自动跟随。
func openContainer(data []byte, password string) (*opened, error) {
	src := containerhandle.NewBytesSource(data, "memory")

	// 先看容器用的是哪一层组织方式，再交给对应的主线 reader。
	h, err := containerhandle.Open(src)
	if err != nil {
		return nil, err
	}
	mf := h.Manifest()
	mfV4 := h.ManifestV4()
	h.Close()

	// 判据必须是 v4 manifest 的 Segments + Playlists：
	// 主线 handle 会把 v4 Segments **映射**成 v2 风格的 Fragments（适配层），
	// 所以"有 Fragments"区分不了两套栈，只有 Segments+Playlists 才是物理段栈。
	if mfV4 != nil && len(mfV4.Segments) > 0 && len(mfV4.Playlists) > 0 {
		info, err := reader.OpenV4ContainerFromSource(src, password)
		if err != nil {
			return nil, err
		}
		return &opened{kind: "segment", manifest: info.Manifest, info: info}, nil
	}

	if mf != nil && len(mf.Fragments) > 0 {
		cr, err := reader.NewEncryptedContainerReaderFromSource(src)
		if err != nil {
			return nil, err
		}
		// 必须是 Seekable 版：SequentialDecryptReader 在这类容器上读出 0 字节
		// （实测：seq=0 / seq-seekable=51 / virtual-seekable=51，内容正确）。
		dr, err := reader.NewSequentialSeekableDecryptReader(cr, password)
		if err != nil {
			return nil, err
		}
		return &opened{kind: "fragment", manifest: mfV4, fragReader: dr}, nil
	}

	return nil, fmt.Errorf("容器既没有 v4 Segments 也没有 Fragments，无法判断用哪套主线 reader")
}

// encryptBytes 在 wasm 里加密出**与 CLI 同格式**的 v4 容器。
//
// 走的是主线同一条路径（分层密钥 + internal/v2/writer.WriteV4Container），
// 不是另写一套：salt→KEK→WrapDEK，hint 用 password+salt 算（reader 会校它），
// segment 加密用 crypto.EncryptSegment。改动主线后这里自动跟随。
//
// 用 WriteV4ContainerTo（io.Writer 版）而不是 WriteV4Container（文件版）：
// js/wasm 上 os 没有文件系统，os.Create 会直接报 "not implemented on js"。
// 主线 writer 为此拆出了只依赖 io.Writer 的写入路径，wasm 侧就不需要复制布局逻辑。
func encryptBytes(plain []byte, password string, containerType uint16, containerTypeStr, originalName string) ([]byte, error) {
	keyLen := crypto.KeySizeForCipherMode_v4(crypto.CipherModeAES128CTR)

	salt, err := crypto.GenerateSalt_v2(16)
	if err != nil {
		return nil, err
	}
	iv, err := crypto.GenerateIV_v2(16)
	if err != nil {
		return nil, err
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		return nil, err
	}

	// 分层密钥：随机 DEK 由口令派生的 KEK 封装（与 openContainer 的 UnwrapDEK 对称）
	dek := make([]byte, keyLen)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	wrapped, err := crypto.WrapDEK(dek, crypto.DeriveKEK(password, salt), nil)
	if err != nil {
		return nil, err
	}
	hint, err := crypto.CalculatePasswordHint(password, salt)
	if err != nil {
		return nil, err
	}
	macKey := crypto.DeriveMACKey(password, macSalt)

	seg, err := crypto.EncryptSegment(plain, dek, macKey, 0, "none")
	if err != nil {
		return nil, err
	}

	// KVI 除了盐/IV，还要带**插件 index**（主线把它存在 KVI 里，而不是别处）：
	// 插件解密路径靠 factory.GetIndex() 从这里取，缺了就报 "index missing"。
	// 直接引用主线的 TextIndex 类型填，字段名随主线走，不手工抄一份。
	sum := md5.Sum(plain)
	kvi, err := json.Marshal(map[string]interface{}{
		"salt_base64": crypto.Base64Encode_v2(salt),
		"iv_base64":   crypto.Base64Encode_v2(iv),
		"text_index": &textplugin.TextIndex{
			ID:                "0",
			OriginalFileSize:  int64(len(plain)),
			MimeType:          "text/plain; charset=utf-8",
			Format:            "plain",
			OriginalFilename:  originalName,
			OriginalInputPath: originalName,
			OriginalFileMD5:   hex.EncodeToString(sum[:]),
		},
	})
	if err != nil {
		return nil, err
	}
	idData := make([]byte, 16)
	if _, err := rand.Read(idData); err != nil {
		return nil, err
	}

	params := &writer.V4WriteParams{
		IsMain:         true,
		ContainerType:  containerType,
		IDType:         types.IDType_Raw,
		IDData:         idData,
		PasswordHint:   hint,
		CipherMode:     0, // AES-128-CTR，与 keyLen 一致
		SegmentResults: []*crypto.SegmentEncryptionResult{seg},
		Manifest: &types.Manifest_v4{
			Version:       4,
			ContainerID:   hex.EncodeToString(idData),
			ContainerType: containerTypeStr,
			Segments:      []types.Segment_v4{{ID: "seg-0"}}, // offset/size/nonce 由 writer 回填
			// Playlists 也得写：主线 reader 的 sequential/seekable reader 按 playlist 名
			// 取段序列（缺省 "default"），不写就报 "playlist 'default' not found"。
			Playlists:  map[string][]string{"default": {"seg-0"}},
			KVI:        kvi,
			WrappedDEK: wrapped,
			// 文件名：主线里这是可选字段（加密文件名时才额外写 filename_alg）。
			// 不填的话部分插件（text）解密时会按"有文件名"去取，直接 nil 引用崩掉，
			// 所以这里至少给一个明文名；不设 FilenameAlgorithm 即表示"文件名未加密"。
			OriginalName: originalName,
		},
	}
	var buf bytes.Buffer
	if err := writer.WriteV4ContainerTo(&buf, params); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func main() {
	js.Global().Set("encvContainer", map[string]interface{}{
		"open": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) < 2 {
				return fail("open(bytes, password)")
			}
			in := args[0]
			if in.Type() != js.TypeObject {
				return fail("bytes 必须是 Uint8Array")
			}
			raw := make([]byte, in.Get("length").Int())
			js.CopyBytesToGo(raw, in)

			item, err := openContainer(raw, args[1].String())
			if err != nil {
				return fail(err.Error())
			}
			nextHandle++
			openedMap[nextHandle] = item
			return okValue(map[string]interface{}{"handle": nextHandle})
		}),
		"info": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			item, err := get(args)
			if err != nil {
				return fail(err.Error())
			}
			mf := item.manifest
			rows := make([]map[string]interface{}, 0, len(mf.Segments))
			for _, seg := range mf.Segments {
				rows = append(rows, map[string]interface{}{
					"id":            seg.ID,
					"offset":        seg.Offset,
					"size":          seg.Size,
					"manifestNonce": seg.Nonce,
				})
			}
			// plainLength：明文总长。
			//
			// 容器里只记录了**密文**尺寸（seg.Size 含段头/nonce/MAC），
			// 用 seg.Size 当读取长度会在空文件等边界上读越界；
			// 明文长度只能解密后才知道（CTR 无填充时与密文等长，但压缩/将来加填充就不一定）。
			plainLength, err := item.plainLength()
			if err != nil {
				return fail(err.Error())
			}
			return okValue(map[string]interface{}{
				"version":       mf.Version,
				"containerType": mf.ContainerType,
				"containerID":   mf.ContainerID,
				"segments":      len(mf.Segments),
				"plainLength":   plainLength,
				"hasWrappedDEK": mf.WrappedDEK != nil && mf.WrappedDEK.IsValid(),
				"keyLen":        keyLenOf(item),
				"stack":         item.kind,
				"kvi":           string(mf.KVI),
				// 嵌套结构经 syscall/js 传不到 JS（取出来是 undefined），统一序列化成字符串
				"segmentsDetail": marshalJSON(rows),
			})
		}),
		"readRange": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			item, err := get(args)
			if err != nil {
				return fail(err.Error())
			}
			off := int64(args[1].Int())
			length := args[2].Int()
			data, err := item.readRange(off, length)
			if err != nil {
				return fail(err.Error())
			}
			return okBytes(data)
		}),
		"encryptBytes": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) < 2 {
				return fail("encryptBytes(plain, password, [opts])")
			}
			in := args[0]
			if in.Type() != js.TypeObject {
				return fail("plain 必须是 Uint8Array")
			}
			plain := make([]byte, in.Get("length").Int())
			js.CopyBytesToGo(plain, in)

			ct := uint16(types.ContainerTypeText)
			ctStr := "text"
			if len(args) > 2 && args[2].Type() == js.TypeObject {
				opts := args[2]
				if v := opts.Get("containerType"); v.Type() == js.TypeNumber {
					ct = uint16(v.Int())
				}
				if v := opts.Get("containerTypeStr"); v.Type() == js.TypeString && v.String() != "" {
					ctStr = v.String()
				}
			}
			originalName := "encrypted.bin"
			if len(args) > 2 && args[2].Type() == js.TypeObject {
				if v := args[2].Get("originalName"); v.Type() == js.TypeString && v.String() != "" {
					originalName = v.String()
				}
			}
			container, err := encryptBytes(plain, args[1].String(), ct, ctStr, originalName)
			if err != nil {
				return fail(err.Error())
			}
			return okBytes(container)
		}),
		"close": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) < 1 {
				return fail("close(handle)")
			}
			delete(openedMap, args[0].Int())
			return okValue(true)
		}),
	})
	<-make(chan struct{})
}

// readRange 按**明文偏移**随机读：直接用主线 seekable reader 的 Seek + Read。
//
// 不自己定位段、不自己解 nonce：那些是 reader 的职责，重复实现只会与主线漂移
// （曾经在这里按 seg.Size 反推段内长度，manifest 的 nonce 是 base64 字符串、
// 长度不等于 nonce 字节数，小文件直接算成负数读空）。
func (o *opened) readRange(off int64, length int) ([]byte, error) {
	if length <= 0 {
		return []byte{}, nil // 读 0 字节是合法的（空文件）
	}
	if o.kind == "fragment" {
		p, err := o.plainAll()
		if err != nil {
			return nil, err
		}
		if off >= int64(len(p)) {
			return []byte{}, nil
		}
		end := off + int64(length)
		if end > int64(len(p)) {
			end = int64(len(p))
		}
		return p[off:end], nil
	}

	r, err := reader.NewSegmentSeekableReader(o.info, "")
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if length <= 0 {
		return []byte{}, nil // 读 0 字节是合法的（空文件）
	}
	if _, err := r.Seek(off, io.SeekStart); err != nil {
		return nil, err
	}
	buf := make([]byte, length)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:n], nil
}

// plainLength 明文总长：容器里只记录密文尺寸，真实长度由主线 reader 读到底得出。
func (o *opened) plainLength() (int64, error) {
	p, err := o.plainAll()
	if err != nil {
		return 0, err
	}
	return int64(len(p)), nil
}

// plainAll 读出完整明文（fragment 栈只能顺序读，随机读在结果上切片）。
func (o *opened) plainAll() ([]byte, error) {
	if o.plainCache != nil {
		return o.plainCache, nil
	}
	if o.kind == "fragment" {
		if o.fragReader == nil {
			return nil, fmt.Errorf("容器未打开")
		}
		p, err := io.ReadAll(o.fragReader)
		if err != nil {
			return nil, err
		}
		o.plainCache = p
		return p, nil
	}

	r, err := reader.NewSegmentSeekableReader(o.info, "")
	if err != nil {
		return nil, err
	}
	defer r.Close()
	p, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	o.plainCache = p
	o.plainSize = int64(len(p))
	return p, nil
}

// keyLenOf 当前栈派生出的密钥长度（fragment 栈由主线内部持有，不暴露）。
func keyLenOf(o *opened) int {
	if o.info == nil {
		return 0
	}
	return len(o.info.EncryptKey)
}

func get(args []js.Value) (*opened, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("缺少 handle")
	}
	item, ok := openedMap[args[0].Int()]
	if !ok {
		return nil, fmt.Errorf("无效 handle：%d", args[0].Int())
	}
	return item, nil
}

func marshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func fail(msg string) map[string]interface{} {
	return map[string]interface{}{"ok": false, "error": msg}
}

func okValue(v interface{}) map[string]interface{} {
	return map[string]interface{}{"ok": true, "value": v}
}

func okBytes(b []byte) map[string]interface{} {
	out := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(out, b)
	return map[string]interface{}{"ok": true, "data": out}
}
