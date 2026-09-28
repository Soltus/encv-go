//go:build js && wasm

// cmd/encv-wasm-container —— ENCV v4 容器的浏览器端解密内核（Go → WASM）。
//
// 目标：让浏览器**不依赖 Go 后端**就能读 ENCV v4 容器（.sccgv/.sccga/.sccgi/.sccgp/.sccgt/.sccgwps），
// 与 CLI / 主应用完全同质：编译的是同一份 internal/v2 代码，不是另写一套。
//
// 目前暴露的能力：
//
//	encvContainer.open(bytes, password) -> handle
//	encvContainer.info(handle)          -> 容器元信息 JSON
//	encvContainer.readRange(handle, off, len) -> Uint8Array（按明文偏移随机读，视频拖动靠它）
//	encvContainer.close(handle)
//	encvContainer.encryptBytes(plain, password, opts) -> Uint8Array（整块加密）
//	encvContainer.encryptBegin(password, opts) -> handle（流式加密：开始）
//	encvContainer.encryptWrite(handle, chunk) -> Uint8Array（流式加密：喂明文，取回已成型的密文段）
//	encvContainer.encryptEnd(handle)   -> {head, data, tail, …}（流式加密：收尾）
//	encvContainer.encryptAbort(handle) -> 丢弃会话
//
// 整块与流式的差别**只在内存行为**：整块路径要求同时持有"明文+密文+容器"三份，
// 浏览器里明文到 GB 级会 fatal error: out of memory（实测 1GB 就崩）；
// 流式路径 Go 侧只持有一个 Segment 的缓冲，产物格式与整块路径完全同构。
// 前端拼装顺序恒为 head ‖ 每次 write 取回的碎块 ‖ end 的最后一块 ‖ tail。
//
// 所有导出都返回「结果对象」 {ok, ...} 或 {ok:false, error}，坏参数只让本次调用失败。
package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	// src 仅流式打开（openStream）时有值：按需取字节的源，readRange 缺字节时要回去找它
	src *needSource

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

	nextEncHandle int
	encMap        = map[int]*encryptSession{}

	// streamMap 保存"还没打开完（或已打开但字节按需供给）"的流式会话。
	// handle 与 openedMap **共用** nextHandle，避免两套编号撞号。
	streamMap = map[int]*streamSession{}
)

// streamSession 一个流式打开会话：容器字节由 JS 现喂现用。
type streamSession struct {
	src      *needSource
	password string
	ready    bool
}

// openContainer 用**主线 reader**打开内存里的容器，按结构选对应的栈。
//
// 不自研段解析/密钥派生：两条路径都是主线代码（reader.NewEncryptedContainerReaderFromSource
// 与 reader.OpenV4ContainerFromSource），主线改了这里自动跟随。
func openContainer(data []byte, password string) (*opened, error) {
	return openContainerFromSource(containerhandle.NewBytesSource(data, "memory"), password)
}

// openContainerFromSource 从任意容器源打开（内存字节 / 按需取字节的源）。
//
// 与 openContainer 的关系：openContainer 是"整块字节"的特例；这里接受任意源，
// 于是 openStream（按区间向 JS 索取字节）能复用完全相同的分派与主线 reader。
func openContainerFromSource(src containerhandle.ContainerSource, password string) (*opened, error) {
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
			// ⚠️ mac_salt 必须显式写进 manifest：这里加密用的是 DeriveMACKey(password, macSalt)，
			// 留空的话 writer 会自己再生成一个 recorded salt，reader 派生出的 mac_key 就是另一个，
			// 哪天把 EnableHMAC 打开会得到一个"MAC 永远验不过"的容器。
			MACSaltBase64: crypto.Base64Encode_v2(macSalt),
		},
	}
	var buf bytes.Buffer
	if err := writer.WriteV4ContainerTo(&buf, params); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ByteRange 是内核向 JS 声明"我缺这一段字节"。
type ByteRange struct {
	Offset int64 `json:"offset"`
	Length int   `json:"length"`
}

// errNeedBytes 表示"这段字节还没喂进来"，携带需要补齐的范围。
type errNeedBytes struct{ need []ByteRange }

func (e *errNeedBytes) Error() string {
	return fmt.Sprintf("需要容器字节 %v（先 streamFeed 再重试）", e.need)
}

// feedCacheBytes 是喂进来的字节缓存上限。
//
// ⚠️ 不能按"块数"限制：info() 会把**每个**段的段头（34B）都读一遍，
// 1GB/1MB 段的容器有 1024 段 —— 按块数淘汰会让前面喂进来的段头被挤掉，
// 内核于是反复索要同一段，前端永远喂不完（活锁）。按字节预算就没事：
// 1024×34B 才 35KB。
const feedCacheBytes = 64 << 20

type needSource struct {
	size int64
	name string
	off  int64 // 顺序读游标

	cache     map[int64][]byte // offset -> bytes
	order     []int64
	cachedLen int
	miss      map[int64]int // 诊断用：同一个缺口被索要的次数
	pending   []ByteRange   // 最近一次 ReadAt 声明的缺口
}

func newNeedSource(size int64, name string) *needSource {
	return &needSource{size: size, name: name, cache: map[int64][]byte{}, miss: map[int64]int{}}
}

func (s *needSource) Size() int64  { return s.size }
func (s *needSource) Name() string { return s.name }
func (s *needSource) Close() error { return nil }

// Feed 把一段字节喂进缓存（JS 侧取来后调用）。
func (s *needSource) Feed(off int64, b []byte) {
	if old, ok := s.cache[off]; ok {
		s.cachedLen -= len(old)
	} else {
		s.order = append(s.order, off)
	}
	s.cache[off] = append([]byte(nil), b...)
	s.cachedLen += len(b)
	for s.cachedLen > feedCacheBytes && len(s.order) > 1 {
		evict := s.order[0]
		s.cachedLen -= len(s.cache[evict])
		delete(s.cache, evict)
		s.order = s.order[1:]
	}
}

func (s *needSource) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("负偏移：%d", off)
	}
	if len(p) == 0 {
		return 0, nil
	}
	if off >= s.size {
		return 0, io.EOF
	}
	want := int64(len(p))
	if off+want > s.size {
		want = s.size - off
	}
	got, ok := s.cache[off]
	if !ok || int64(len(got)) < want {
		s.miss[off]++
		if s.miss[off] == 4 {
			println(fmt.Sprintf("[dbg] miss#%d off=%d want=%d have=%d keys=%d cachedLen=%d", s.miss[off], off, want, len(got), len(s.cache), s.cachedLen))
		}
		s.pending = []ByteRange{{Offset: off, Length: int(want)}}
		return 0, &errNeedBytes{need: s.pending}
	}
	s.pending = nil
	n := copy(p, got[:want])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (s *needSource) Read(p []byte) (int, error) {
	n, err := s.ReadAt(p, s.off)
	s.off += int64(n)
	return n, err
}

func (s *needSource) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		s.off = offset
	case io.SeekCurrent:
		s.off += offset
	case io.SeekEnd:
		s.off = s.size + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	if s.off < 0 {
		s.off = 0
	}
	return s.off, nil
}

// needValue 把缺口清单转成能安全穿过 syscall/js 的普通容器。
//
// ⚠️ 不能直接把 Go 结构体切片塞进 map[string]interface{}：
// syscall/js.ValueOf 不认识自定义结构体，会在 Value.Set 里 panic
// （表现是 wasm 直接退出、后续调用全是 "Go program has already exited"）。
func needValue(need []ByteRange) []interface{} {
	out := make([]interface{}, 0, len(need))
	for _, r := range need {
		out = append(out, map[string]interface{}{"offset": r.Offset, "length": r.Length})
	}
	return out
}

// advanceStream 尝试推进一个流式打开会话，把"还缺哪些字节"如实告诉 JS。
//
// 打开分两步：先读信封头（2048B）拿到 manifest 的偏移与长度，再读 manifest ——
// 每一步都可能因为字节没喂进来而中断，所以这里可以被反复调用，直到 ready。
func advanceStream(handle int) interface{} {
	sess, ok := streamMap[handle]
	if !ok {
		return fail(fmt.Sprintf("无效 handle：%d", handle))
	}
	if sess.ready {
		return okValue(map[string]interface{}{"handle": handle, "ready": true})
	}
	item, err := openContainerFromSource(sess.src, sess.password)
	if err != nil {
		if need := needOf(err); need != nil {
			return okValue(map[string]interface{}{"handle": handle, "ready": false, "need": needValue(need)})
		}
		delete(streamMap, handle)
		return fail(err.Error())
	}
	item.src = sess.src
	openedMap[handle] = item
	sess.ready = true
	return okValue(map[string]interface{}{"handle": handle, "ready": true})
}

// needOf 把"缺字节"错误翻译成给 JS 的缺口清单；不是这类错误时返回 nil。
func needOf(err error) []ByteRange {
	var ne *errNeedBytes
	if errors.As(err, &ne) {
		return ne.need
	}
	// 主线 reader 偶尔会把底层错误丢掉（换成 io.EOF / ErrUnexpectedEOF 之类），
	// 这时回退到"源自己记录的最后一次缺口"，否则 JS 会拿到一个无从下手的错误。
	return nil
}

// pendingNeed 取某个会话最近一次声明的缺口（供丢掉错误类型的路径兜底）。
func pendingNeed(handle int) []ByteRange {
	if sess, ok := streamMap[handle]; ok {
		return sess.src.pending
	}
	if item, ok := openedMap[handle]; ok && item.src != nil {
		return item.src.pending
	}
	return nil
}

// streamSink 把 writer 随写随出的数据段攒住，等 JS 下一次调用来取。
//
// 为什么不把整个容器攒出来：见 internal/v2/writer/stream_v4.go 顶部注释
// （整块路径在浏览器里会在 ~1GB 明文处把 Go 打死：runtime: out of memory）。
//
// 为什么不让 writer 直接回调 JS：跨 syscall/js 边界回调会把"顺序写入"拆成
// 异步拼图，调用方还得自己合成品顺序。让 JS 每次 encryptWrite 后顺手取走这段，
// 顺序天然就是对的。
type streamSink struct{ s *encryptSession }

func (k *streamSink) Append(p []byte) error {
	k.s.pending = append(k.s.pending, append([]byte(nil), p...))
	return nil
}

func (k *streamSink) Finish(head []byte, tail []byte) error {
	k.s.head = append([]byte(nil), head...)
	k.s.tail = append([]byte(nil), tail...)
	return nil
}

// encryptSession 一个进行中的流式加密会话。
type encryptSession struct {
	w       *writer.V4StreamWriter
	pending [][]byte // 已成型、还没被 JS 取走的数据段
	head    []byte   // Close 之后才有：容器头（2048B）
	tail    []byte   // Close 之后才有：manifest + footer
	done    bool
}

// takePending 取出并清空到目前为止写出的数据段（拼成一块交给 JS）。
func (s *encryptSession) takePending() []byte {
	if len(s.pending) == 0 {
		return []byte{}
	}
	total := 0
	for _, p := range s.pending {
		total += len(p)
	}
	out := make([]byte, 0, total)
	for _, p := range s.pending {
		out = append(out, p...)
	}
	s.pending = nil
	return out
}

// startEncrypt 起一个流式加密会话。密钥素材（salt/DEK/hint/macSalt）在这里生成，
// 之后所有明文块继续喂给同一个 writer —— 与 encryptBytes 走的是同一条分层密钥路径。
func startEncrypt(password string, opts js.Value) (*encryptSession, error) {
	ct := uint16(types.ContainerTypeText)
	ctStr := "text"
	originalName := "encrypted.bin"
	mimeType := "application/octet-stream"
	format := "plain"
	segmentSize := int64(writer.DefaultStreamSegmentSize)
	enableHMAC := false

	if opts.Type() == js.TypeObject {
		if v := opts.Get("containerType"); v.Type() == js.TypeNumber {
			ct = uint16(v.Int())
		}
		if v := opts.Get("containerTypeStr"); v.Type() == js.TypeString && v.String() != "" {
			ctStr = v.String()
		}
		if v := opts.Get("originalName"); v.Type() == js.TypeString && v.String() != "" {
			originalName = v.String()
		}
		if v := opts.Get("mimeType"); v.Type() == js.TypeString && v.String() != "" {
			mimeType = v.String()
		}
		if v := opts.Get("format"); v.Type() == js.TypeString && v.String() != "" {
			format = v.String()
		}
		if v := opts.Get("segmentSize"); v.Type() == js.TypeNumber && v.Int() > 0 {
			segmentSize = int64(v.Int())
		}
		if v := opts.Get("enableHMAC"); v.Type() == js.TypeBoolean {
			enableHMAC = v.Bool()
		}
	}

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
	idData := make([]byte, 16)
	if _, err := rand.Read(idData); err != nil {
		return nil, err
	}

	sess := &encryptSession{}
	params := &writer.V4StreamParams{
		IsMain:        true,
		ContainerType: ct,
		IDType:        types.IDType_Raw,
		IDData:        idData,
		PasswordHint:  hint,
		Key:           dek,
		MacKey:        macKey,
		CipherMode:    0, // AES-128-CTR，与 keyLen 一致
		EnableHMAC:    enableHMAC,
		SegmentSize:   segmentSize,
		Manifest: &types.Manifest_v4{
			Version:       4,
			ContainerID:   hex.EncodeToString(idData),
			ContainerType: ctStr,
			OriginalName:  originalName,
			WrappedDEK:    wrapped,
			// mac_salt 必须写进 manifest：留空的话 writer 会自己再生成一个，
			// 于是加密用一个 mac_key、校验用另一个 —— EnableHMAC=true 时永远验不过。
			MACSaltBase64: crypto.Base64Encode_v2(macSalt),
		},
		// KVI 里的插件 index 要等"明文总长 + MD5"出来才知道，所以放在最后这个回调里填。
		OnManifest: func(m *types.Manifest_v4, plainSize int64, plainMD5 string) error {
			kvi, err := json.Marshal(map[string]interface{}{
				"salt_base64": crypto.Base64Encode_v2(salt),
				"iv_base64":   crypto.Base64Encode_v2(iv),
				// 与整块路径一致：主线把插件 index 存在 KVI 里，text 插件解密时会去取，
				// 缺了就报 "index missing"。流式路径不做因此改变。
				"text_index": &textplugin.TextIndex{
					ID:                "0",
					OriginalFileSize:  plainSize,
					MimeType:          mimeType,
					Format:            format,
					OriginalFilename:  originalName,
					OriginalInputPath: originalName,
					OriginalFileMD5:   plainMD5,
				},
			})
			if err != nil {
				return err
			}
			m.KVI = kvi
			return nil
		},
	}

	w, err := writer.NewV4StreamWriter(&streamSink{s: sess}, params)
	if err != nil {
		return nil, err
	}
	sess.w = w
	return sess, nil
}

func getEncryptSession(args []js.Value) (*encryptSession, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("缺少 handle")
	}
	s, ok := encMap[args[0].Int()]
	if !ok {
		return nil, fmt.Errorf("无效 handle：%d", args[0].Int())
	}
	return s, nil
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
				// 流式打开时明文长度要读段头才知道 —— 段头还没喂进来就把缺口交出去，
				// 让 JS 补上再问一次（与 readRange 同一种"要字节"协议）。
				if need := needOf(err); need != nil {
					return map[string]interface{}{"ok": false, "need": needValue(need)}
				}
				if need := pendingNeed(args[0].Int()); need != nil {
					// 兜底：把原始错误一并带回，否则前端只会看到"反复索要同一段"却不知真因
					return map[string]interface{}{"ok": false, "need": needValue(need), "error": err.Error()}
				}
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
				// 流式打开时：命中的段还没喂进来 → 把缺口交给 JS，喂完重试即可
				if need := needOf(err); need != nil {
					return map[string]interface{}{"ok": false, "need": needValue(need)}
				}
				if need := pendingNeed(args[0].Int()); need != nil {
					// 兜底：把原始错误一并带回，否则前端只会看到"反复索要同一段"却不知真因
					return map[string]interface{}{"ok": false, "need": needValue(need), "error": err.Error()}
				}
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
			delete(streamMap, args[0].Int())
			return okValue(true)
		}),

		// ── 流式打开（大容器）：不把容器整体读进内存 ──
		//
		// open(bytes) 的增量版。协议是**同步**的「声明缺口 → 喂字节 → 重试」：
		//   1. openStream 返回 {handle, ready:false, need:[[off,len],…]}
		//   2. JS 按需取来这些字节后 streamFeed(handle, off, bytes)，内核继续推进
		//   3. ready:true 之后，info/readRange/close 用法与 open 完全一致
		//      （readRange 也可能再返回 need —— 命中的段还没喂进来）
		//
		// 为什么不直接调 JS 的 read(offset,length)：Go/wasm 不能在导出给 JS 的
		// 同步函数里挂起等 Promise（实测会让 Go 程序直接退出）。
		"openStream": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) < 1 || args[0].Type() != js.TypeString || args[0].String() == "" {
				return fail("openStream(password, opts)")
			}
			if len(args) < 2 || args[1].Type() != js.TypeObject {
				return fail("openStream 需要 opts {size}")
			}
			opts := args[1]
			if v := opts.Get("size"); v.Type() != js.TypeNumber || v.Int() <= 0 {
				return fail("opts.size 必须是正整数（容器总字节数）")
			}
			name := "stream"
			if v := opts.Get("name"); v.Type() == js.TypeString && v.String() != "" {
				name = v.String()
			}
			src := newNeedSource(int64(opts.Get("size").Int()), name)
			nextHandle++
			handle := nextHandle
			streamMap[handle] = &streamSession{src: src, password: args[0].String()}
			return advanceStream(handle)
		}),

		// streamFeed 把内核声明缺少的那段字节喂进去，随即继续推进打开流程。
		"streamFeed": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) < 3 || args[0].Type() != js.TypeNumber || args[1].Type() != js.TypeNumber {
				return fail("streamFeed(handle, offset, bytes)")
			}
			// ⚠️ 拿到 length 之前必须先确认它真的是数字：JS 侧一旦传错
			// （比如忘了 await、把 Promise 传进来），Value.Int() 会直接 panic ——
			// 而 wasm 里一次 panic 会打死整个内核实例，后面所有调用全废。
			if args[2].Type() != js.TypeObject || args[2].Get("length").Type() != js.TypeNumber {
				return fail("bytes 必须是 Uint8Array（是不是忘了 await？）")
			}
			sess, ok := streamMap[args[0].Int()]
			if !ok {
				if _, opened := openedMap[args[0].Int()]; opened {
					return okValue(map[string]interface{}{"handle": args[0].Int(), "ready": true})
				}
				return fail(fmt.Sprintf("无效 handle：%d", args[0].Int()))
			}
			buf := make([]byte, args[2].Get("length").Int())
			js.CopyBytesToGo(buf, args[2])
			sess.src.Feed(int64(args[1].Int()), buf)
			return advanceStream(args[0].Int())
		}),

		// ── 流式加密（大附件）：begin → write* → end ──
		//
		// 与 encryptBytes 的差别只有"内存行为"：明文按块喂、密文按块取，
		// Go 侧同时只持有一个 Segment 的缓冲；产物格式与整块路径完全同构。
		"encryptBegin": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) < 1 || args[0].Type() != js.TypeString || args[0].String() == "" {
				return fail("encryptBegin(password, opts)")
			}
			opts := js.Undefined()
			if len(args) > 1 {
				opts = args[1]
			}
			sess, err := startEncrypt(args[0].String(), opts)
			if err != nil {
				return fail(err.Error())
			}
			nextEncHandle++
			encMap[nextEncHandle] = sess
			return okValue(map[string]interface{}{"handle": nextEncHandle})
		}),
		"encryptWrite": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			s, err := getEncryptSession(args)
			if err != nil {
				return fail(err.Error())
			}
			if s.done {
				return fail("该会话已结束（encryptEnd 之后不能再 write）")
			}
			if len(args) < 2 || args[1].Type() != js.TypeObject {
				return fail("encryptWrite(handle, chunk)")
			}
			raw := make([]byte, args[1].Get("length").Int())
			js.CopyBytesToGo(raw, args[1])
			if _, err := s.w.Write(raw); err != nil {
				return fail(err.Error())
			}
			// 把本次写调用攒出的、已成型的数据段交给 JS。
			// 调用方应立刻把它们追加到自己的 sink（Blob parts / 文件句柄）而不是攒着，
			// 否则只是把内存压力从 Go 侧挪回 JS 侧。
			return okBytes(s.takePending())
		}),
		"encryptEnd": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			s, err := getEncryptSession(args)
			if err != nil {
				return fail(err.Error())
			}
			if s.done {
				return fail("该会话已结束")
			}
			if err := s.w.Close(); err != nil {
				return fail(err.Error())
			}
			s.done = true
			delete(encMap, args[0].Int())
			// 最后一次还没取走的数据段也要吐出去：容器 = head ‖ 全部数据段 ‖ tail
			return okValue(map[string]interface{}{
				"head":           jsBytes(s.head),
				"tail":           jsBytes(s.tail),
				"data":           jsBytes(s.takePending()),
				"containerBytes": int(s.w.ContainerSize()),
				"plainSize":      int(s.w.PlainSize()),
				"segments":       s.w.SegmentCount(),
			})
		}),
		"encryptAbort": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) < 1 {
				return fail("encryptAbort(handle)")
			}
			delete(encMap, args[0].Int())
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
// segmentPlainLength 按 manifest 算术算出明文总长（**不读密文**）。
//
// 为什么不能沿用 plainAll()：段栈的明文长度是"逐段解密后累加"算出来的，
// 也就是为了拿到一个长度要把整个容器读一遍 —— 流式打开时这会把刚省下的内存
// 又全花掉（实测：1GB 容器会在这一步把 64MB 字节缓存挤爆，反复索要同一段）。
//
// 无压缩的段，明文长度可以纯从段头算出来：
//
//	plain = seg.Size - SegmentHeaderSize - NonceSize - MACSize - SeekTableLength
//
// 遇到任一声明了 zstd 压缩的段，就放弃这条捷径（压缩后长度无法预测），
// 交回 plainAll() 处理。
func (o *opened) segmentPlainLength() (int64, error) {
	if o.info == nil || o.info.Src == nil || len(o.manifest.Segments) == 0 {
		return 0, fmt.Errorf("段栈信息不全，无法算术求长")
	}
	src, ok := o.info.Src.(io.ReaderAt)
	if !ok {
		return 0, fmt.Errorf("容器源不支持随机读")
	}
	segs := o.manifest.Segments

	// 段头**分散在整个容器里**（每段开头 34B），逐段读一遍等于把文件读遍 ——
	// 1GB/1MB 段的容器就是 1024 个 Range 请求、19 秒，只为了拿一个长度。
	// 所以这里抽样：首段 + 末段，两段开销一致就按统一开销推算全部。
	// 不一致（历史容器、混合配置）则报错，交回 plainAll() 的老路子。
	readOverhead := func(seg types.Segment_v4) (int64, error) {
		buf := make([]byte, types.SegmentHeaderSize)
		if _, err := src.ReadAt(buf, int64(seg.Offset)); err != nil {
			return 0, err
		}
		var hdr types.SegmentHeader
		if err := hdr.UnmarshalBinary(buf); err != nil {
			return 0, fmt.Errorf("解析段头 %s 失败：%w", seg.ID, err)
		}
		if hdr.ModeFlags&types.ModeFlagCompressionZstd != 0 {
			return 0, fmt.Errorf("段 %s 声明了 zstd 压缩，明长只能解密后才知道", seg.ID)
		}
		return int64(types.SegmentHeaderSize) + int64(hdr.NonceSize) + int64(hdr.MACSize) + int64(hdr.SeekTableLength), nil
	}

	head, err := readOverhead(segs[0])
	if err != nil {
		return 0, err
	}
	if len(segs) > 1 {
		tail, err := readOverhead(segs[len(segs)-1])
		if err != nil {
			return 0, err
		}
		if tail != head {
			return 0, fmt.Errorf("首段/末段的段头开销不一致（%d vs %d），明长只能逐段读", head, tail)
		}
	}

	var total int64
	for _, seg := range segs {
		plain := int64(seg.Size) - head
		if plain < 0 {
			return 0, fmt.Errorf("段 %s 的尺寸算不出明文长度（Size=%d, 开销=%d）", seg.ID, seg.Size, head)
		}
		total += plain
	}
	return total, nil
}

func (o *opened) plainLength() (int64, error) {
	if o.kind == "segment" {
		n, err := o.segmentPlainLength()
		if err == nil {
			return n, nil
		}
		// ⚠️ "还缺字节"必须原样上抛：一旦在这里被当成普通失败吞掉、退回 plainAll()，
		// 就等于为了拿一个长度去读遍所有段头（1GB 容器 = 1024 次 Range 请求）。
		var ne *errNeedBytes
		if errors.As(err, &ne) {
			return 0, err
		}
	}
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
	return map[string]interface{}{"ok": true, "data": jsBytes(b)}
}

// jsBytes 拷一段 Go 字节到 JS 的 Uint8Array。
// 嵌在 map 里返回是可以的：syscall/js 的 Value.Set 会对每个值走 ValueOf，而
// ValueOf 遇到 js.Value 原样透传（所以 head/tail 能直接作为 Uint8Array 到手）。
func jsBytes(b []byte) js.Value {
	out := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(out, b)
	return out
}
