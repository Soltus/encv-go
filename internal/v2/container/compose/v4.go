// Package compose 是「明文 → v4 容器」的**编排层**，也是这件事的唯一真源。
//
// 为什么要有这一层：
//
//	之前这段编排（生成盐/DEK/口令提示、装 manifest、装 KVI 与插件 index、
//	决定走整块还是流式写入）写在 wasm 内核里。于是主应用（CLI / 服务端）一旦改
//	加解密逻辑，wasm 就得跟着重写；两边漂移时**不报错**，只会表现为
//	"浏览器加密的东西 CLI 解不开（还是静默乱码）"。
//
//	现在编排在这里，wasm 与未来的主应用都只调用它：
//	改一次，两端一起变。wasm 侧由 cmd/encv-wasm-container/thin_shell_test.go
//	守着，禁止再长出主线逻辑。
//
// 本层不发明密码学，也不复制容器布局 —— 全部委托给 internal/v2 的
// crypto / writer / plugins。
package compose

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	textplugin "github.com/Soltus/encv-go/internal/v2/plugins/text"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

// Options 是加密编排的全部入参（与容器类型/文件名/密钥策略有关的一切）。
type Options struct {
	Password         string
	ContainerType    uint16
	ContainerTypeStr string
	OriginalName     string
	MimeType         string
	Format           string

	// EnableHMAC / DisableHMAC：v4 容器是否在每段末尾写 10 字节 HMAC-SHA1-80。
	//
	// 🆕 2026-10-02 决策：**默认开启**（与 config.V4EnableHMAC 的默认一致）。
	//   事故背景（真机实测）：容器被篡改/位翻转后，没有 MAC 的容器读出来是
	//   「长度正确、内容乱码、不报错」——最恶劣的静默损坏。
	//   因为 Go 的 bool 零值是 false、无法区分"没设"和"显式关"，
	//   这里用两个字段表达三态：
	//     两者都未设      → 默认开启
	//     EnableHMAC=true  → 开启
	//     DisableHMAC=true → 显式关闭（仅兼容/测试场景使用）
	EnableHMAC  bool
	DisableHMAC bool

	SegmentSize int64 // 流式时才用；<=0 取 writer.DefaultStreamSegmentSize

	// Compression 压缩模式：只接受 crypto.CompressionModeNone（零值 "" 也当它用）。
	//
	// ⚠️ 契约（2026-09-30 从"隐式硬编码"改成显式）：**compose 这一层两条路都不支持压缩**，
	// 传 zstd 会被直接拒绝，而不是静默忽略后产出一个"其实没压缩"的容器。
	//
	//   - 流式为什么不行：seekable zstd 要随机访存整段数据，与"一片进一片出"冲突；
	//   - 整块为什么也拒绝：compose 的整块与流式必须同构（wasm 与主应用共用这一层），
	//     一边能压缩一边不能，就会出现"同一个 API 产出两种容器"。
	//
	// 需要压缩的容器走**插件加密路径**（CLI encrypt-v2 / crypto.EncryptToTempFile_v2）；
	// 读取侧两种都能解（压缩段由 internal/v2/crypto/compression 处理）。
	Compression string

	// KVIExtra 往 KVI 里塞的**额外字段**（插件 index 等）。
	//
	// 默认只写 text_index —— 那样产出的容器只有 text 插件认。主应用要加密
	// 视频/音频/PDF 等时，在这里给出对应插件的 index（由插件自己构造），
	// 产物才能被对应插件解开。签名里带明文大小与 MD5 是因为 index 通常要记它们。
	KVIExtra func(plainSize int64, plainMD5 string) map[string]interface{}
}

func (o Options) withDefaults() Options {
	if o.ContainerType == 0 {
		o.ContainerType = types.ContainerTypeText
	}
	if o.ContainerTypeStr == "" {
		o.ContainerTypeStr = "text"
	}
	if o.OriginalName == "" {
		o.OriginalName = "encrypted.bin"
	}
	if o.MimeType == "" {
		o.MimeType = "application/octet-stream"
	}
	if o.Format == "" {
		o.Format = "plain"
	}
	if o.SegmentSize <= 0 {
		o.SegmentSize = writer.DefaultStreamSegmentSize
	}
	// 【完整性默认开启】两者都没显式设置 ⇒ 开启 HMAC（见 Options 的字段注释）
	if !o.EnableHMAC && !o.DisableHMAC {
		o.EnableHMAC = true
	}
	return o
}

// CipherMode：v4 用 AES-128-CTR（与 KeySizeForCipherMode_v4 保持一致）。
const cipherMode = uint16(crypto.CipherModeAES128CTR)

// ErrCompressionUnsupported 是 compose 的压缩契约：这一层不支持压缩（见 Options.Compression）。
//
// 之所以要**显式报错**而不是"忽略传入值照常产出"：调用方请求了 zstd 却拿到一个没压缩的
// 容器，它不会失败、不会告警 —— 只有体积不对，等发现时数据已经按"压缩过"的预期流转了。
var ErrCompressionUnsupported = errors.New("compose 编排层不支持压缩（zstd 需要随机访存整段，与流式写入冲突；整块为与流式同构也一并拒绝）；需要压缩请走插件加密路径（encrypt-v2 / EncryptToTempFile_v2），读取侧两种都能解")

// checkCompression 校验压缩契约；零值（""）按不压缩处理。
func (o Options) checkCompression() error {
	if o.Compression != "" && o.Compression != crypto.CompressionModeNone {
		return fmt.Errorf("%w：请求了 %q", ErrCompressionUnsupported, o.Compression)
	}
	return nil
}

// compressionOf 把 Options 的压缩模式规范化成 crypto 的枚举（空串 → none）。
func compressionOf(o Options) string {
	if o.Compression == "" {
		return crypto.CompressionModeNone
	}
	return o.Compression
}

// material 是一次加密用到的全部密钥素材。
type material struct {
	salt     []byte
	iv       []byte
	macSalt  []byte
	dek      []byte
	macKey   []byte
	wrapped  *types.WrappedDEK
	hint     [16]byte
	idData   []byte
	password string
}

func newMaterial(password string) (*material, error) {
	// ⚠️ 密钥素材必须走主线 crypto.PrepareEncryptionContext，不要在这里再生成一份：
	// 整块路径（插件层）用的就是它，两边各自生成就等于两份"怎么派生密钥"的逻辑，
	// 主应用一改就会出现"CLI 加密的容器 wasm 解不开"这类漂移。
	ctx, err := crypto.PrepareEncryptionContext(password)
	if err != nil {
		return nil, err
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		return nil, err
	}
	hint, err := crypto.CalculatePasswordHint(password, ctx.Salt)
	if err != nil {
		return nil, err
	}
	idData := make([]byte, 16)
	if _, err := rand.Read(idData); err != nil {
		return nil, err
	}
	return &material{
		salt:     ctx.Salt,
		iv:       ctx.IV,
		macSalt:  macSalt,
		dek:      ctx.DEK,
		macKey:   crypto.DeriveMACKey(password, macSalt),
		wrapped:  ctx.WrappedDEK,
		hint:     hint,
		idData:   idData,
		password: password,
	}, nil
}

// kvi 装配：盐 / IV / 插件 index。
//
// 插件 index 必须写进 KVI：主线把它存在这里（而不是别处），插件解密路径靠
// factory.GetIndex() 从这里取，缺了就报 "index missing"。
// 字段名直接引用主线的 TextIndex 类型，主线改了这里自动跟随。
func (m *material) kvi(opts Options, plainSize int64, plainMD5 string) ([]byte, error) {
	kv := map[string]interface{}{
		"salt_base64": crypto.Base64Encode_v2(m.salt),
		"iv_base64":   crypto.Base64Encode_v2(m.iv),
	}
	if opts.KVIExtra != nil {
		for k, v := range opts.KVIExtra(plainSize, plainMD5) {
			kv[k] = v
		}
	} else {
		kv["text_index"] = &textplugin.TextIndex{
			ID:                "0",
			OriginalFileSize:  plainSize,
			MimeType:          opts.MimeType,
			Format:            opts.Format,
			OriginalFilename:  opts.OriginalName,
			OriginalInputPath: opts.OriginalName,
			OriginalFileMD5:   plainMD5,
		}
	}
	return json.Marshal(kv)
}

// manifest 模板：版本、容器 ID、文件名、分层密钥、mac_salt。
//
// ⚠️ mac_salt 必须显式写进 manifest：加密用的是 DeriveMACKey(password, macSalt)，
// 留空的话 writer 会自己再生成一个 recorded salt，reader 派生出的 mac_key 就是另一个 ——
// 哪天把 EnableHMAC 打开会得到"MAC 永远验不过"的容器。
func (m *material) manifest(opts Options) *types.Manifest_v4 {
	return &types.Manifest_v4{
		Version:       4,
		ContainerID:   hex.EncodeToString(m.idData),
		ContainerType: opts.ContainerTypeStr,
		// 文件名：主线里这是可选字段（加密文件名时才额外写 filename_alg）。
		// 不填的话部分插件（text）解密时会按"有文件名"去取，直接 nil 引用崩掉，
		// 所以这里至少给一个明文名；不设 FilenameAlgorithm 即表示"文件名未加密"。
		OriginalName:  opts.OriginalName,
		WrappedDEK:    m.wrapped,
		MACSaltBase64: crypto.Base64Encode_v2(m.macSalt),
	}
}

// EncryptBytes 整块加密成 v4 容器，写到 w。
//
// 产物格式与 CLI 的 v4 容器一致（同一条 writer 路径），明文大小已知时用它最省事；
// 大附件请用 NewStreamEncryptor。
func EncryptBytes(plain []byte, opts Options, w io.Writer) error {
	if err := opts.checkCompression(); err != nil {
		return err
	}
	opts = opts.withDefaults()
	m, err := newMaterial(opts.Password)
	if err != nil {
		return err
	}
	// 压缩模式走 Options（已被 checkCompression 限定为 None），
	// 不再写死常量：将来若真的支持压缩，只需放开校验，不用再改这里。
	seg, err := crypto.EncryptSegment(plain, m.dek, m.macKey, 0, compressionOf(opts))
	if err != nil {
		return err
	}
	sum := md5.Sum(plain)
	kvi, err := m.kvi(opts, int64(len(plain)), hex.EncodeToString(sum[:]))
	if err != nil {
		return err
	}
	mf := m.manifest(opts)
	mf.Segments = []types.Segment_v4{{ID: "seg-0"}} // offset/size/nonce 由 writer 回填
	// Playlists 必须写：主线 reader 按 playlist 名（缺省 "default"）取段序列，
	// 不写就报 "playlist 'default' not found"。
	mf.Playlists = map[string][]string{"default": {"seg-0"}}
	mf.KVI = kvi

	return writer.WriteV4ContainerTo(w, &writer.V4WriteParams{
		IsMain:         true,
		ContainerType:  opts.ContainerType,
		IDType:         types.IDType_Raw,
		IDData:         m.idData,
		PasswordHint:   m.hint,
		CipherMode:     cipherMode,
		// 【完整性】修前这里**漏传** EnableHMAC —— 于是整块加密路径产出的容器
		// 永远不带 MAC（即使 Options 要求开）。篡改后读取端无从校验 ⇒ 静默乱码。
		EnableHMAC:     opts.EnableHMAC,
		SegmentResults: []*crypto.SegmentEncryptionResult{seg},
		Manifest:       mf,
	})
}

// StreamEncryptor 流式加密：明文分片喂入，密文段经 sink 随写随出。
//
// Go 侧常驻内存只与 Options.SegmentSize 有关，与文件总大小无关。
type StreamEncryptor struct {
	w *writer.V4StreamWriter
}

// NewStreamEncryptor 起一个流式加密会话；sink 决定成型的数据段往哪里去。
func NewStreamEncryptor(opts Options, sink writer.V4StreamSink) (*StreamEncryptor, error) {
	if err := opts.checkCompression(); err != nil {
		return nil, err
	}
	opts = opts.withDefaults()
	m, err := newMaterial(opts.Password)
	if err != nil {
		return nil, err
	}
	mf := m.manifest(opts)

	// KVI 里的插件 index 要等"明文总长 + MD5"出来才知道，所以放在 OnManifest 回调里填。
	onManifest := func(mf2 *types.Manifest_v4, plainSize int64, plainMD5 string) error {
		kvi, err := m.kvi(opts, plainSize, plainMD5)
		if err != nil {
			return err
		}
		mf2.KVI = kvi
		return nil
	}

	w, err := writer.NewV4StreamWriter(sink, &writer.V4StreamParams{
		IsMain:        true,
		ContainerType: opts.ContainerType,
		IDType:        types.IDType_Raw,
		IDData:        m.idData,
		PasswordHint:  m.hint,
		Key:           m.dek,
		MacKey:        m.macKey,
		CipherMode:    cipherMode,
		EnableHMAC:    opts.EnableHMAC,
		SegmentSize:   opts.SegmentSize,
		Manifest:      mf,
		OnManifest:    onManifest,
	})
	if err != nil {
		return nil, err
	}
	return &StreamEncryptor{w: w}, nil
}

func (e *StreamEncryptor) Write(p []byte) (int, error) {
	if e == nil || e.w == nil {
		return 0, fmt.Errorf("流式加密会话未启动")
	}
	return e.w.Write(p)
}

// Close 收尾：封最后一段并交付容器头/尾。
func (e *StreamEncryptor) Close() error {
	if e == nil || e.w == nil {
		return fmt.Errorf("流式加密会话未启动")
	}
	return e.w.Close()
}

// PlainSize 已写入的明文总字节数。
func (e *StreamEncryptor) PlainSize() int64 {
	if e == nil || e.w == nil {
		return 0
	}
	return e.w.PlainSize()
}

// ContainerSize 产物总字节数（header + 数据 + manifest/footer）；Close 之后才齐全。
func (e *StreamEncryptor) ContainerSize() int64 {
	if e == nil || e.w == nil {
		return 0
	}
	return e.w.ContainerSize()
}

// SegmentCount 已封存的段数。
func (e *StreamEncryptor) SegmentCount() int {
	if e == nil || e.w == nil {
		return 0
	}
	return e.w.SegmentCount()
}
