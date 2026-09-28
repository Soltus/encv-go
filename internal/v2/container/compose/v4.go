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
	EnableHMAC       bool
	SegmentSize      int64 // 流式时才用；<=0 取 writer.DefaultStreamSegmentSize
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
	return o
}

// CipherMode：v4 用 AES-128-CTR（与 KeySizeForCipherMode_v4 保持一致）。
const cipherMode = uint16(crypto.CipherModeAES128CTR)

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
	// 分层密钥：随机 DEK 由口令派生的 KEK 封装（与 reader 的 UnwrapDEK 对称）
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
	idData := make([]byte, 16)
	if _, err := rand.Read(idData); err != nil {
		return nil, err
	}
	return &material{
		salt:     salt,
		iv:       iv,
		macSalt:  macSalt,
		dek:      dek,
		macKey:   crypto.DeriveMACKey(password, macSalt),
		wrapped:  wrapped,
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
	return json.Marshal(map[string]interface{}{
		"salt_base64": crypto.Base64Encode_v2(m.salt),
		"iv_base64":   crypto.Base64Encode_v2(m.iv),
		"text_index": &textplugin.TextIndex{
			ID:                "0",
			OriginalFileSize:  plainSize,
			MimeType:          opts.MimeType,
			Format:            opts.Format,
			OriginalFilename:  opts.OriginalName,
			OriginalInputPath: opts.OriginalName,
			OriginalFileMD5:   plainMD5,
		},
	})
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
	opts = opts.withDefaults()
	m, err := newMaterial(opts.Password)
	if err != nil {
		return err
	}
	seg, err := crypto.EncryptSegment(plain, m.dek, m.macKey, 0, crypto.CompressionModeNone)
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
