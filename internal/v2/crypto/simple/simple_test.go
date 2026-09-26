package simple

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
)

const vectorsPath = "testdata/vectors.json"

type vectorsFile struct {
	Spec          string `json:"spec"`
	FormatVersion int    `json:"formatVersion"`
	KDF           string `json:"kdf"`
	Iterations    uint32 `json:"iterations"`
	HeaderSize    int    `json:"headerSize"`
	Derivations   []struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		SaltHex  string `json:"saltHex"`
		KeyLen   int    `json:"keyLen"`
		KeyHex   string `json:"keyHex"`
	} `json:"derivations"`
	Cases []struct {
		Name         string `json:"name"`
		Password     string `json:"password"`
		CipherMode   uint16 `json:"cipherMode"`
		SaltHex      string `json:"saltHex"`
		IVHex        string `json:"ivHex"`
		PlaintextHex string `json:"plaintextHex"`
		ContainerHex string `json:"containerHex"`
	} `json:"cases"`
}

func loadVectors(t *testing.T) *vectorsFile {
	t.Helper()
	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Skipf("跳过黄金向量测试（未找到 %s，先执行 go run ./cmd/encv-crypto-vectors）：%v", vectorsPath, err)
		return nil
	}
	var v vectorsFile
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("解析 %s 失败: %v", vectorsPath, err)
	}
	return &v
}

// TestHeaderLayoutIsLocked 锁住信封的字段布局常量，防止有人悄悄改偏移导致跨端不兼容。
func TestHeaderLayoutIsLocked(t *testing.T) {
	if HeaderSize != 31 {
		t.Fatalf("HeaderSize 变了（%d），前端 ENCVS1 解析器必须同步更新", HeaderSize)
	}
	if Magic != "ENCS" {
		t.Fatalf("Magic 变了：%q", Magic)
	}
	if DefaultIterations != uint32(crypto.Iterations_v2) {
		t.Fatalf("迭代次数与加密层脱钩: %d vs %d", DefaultIterations, crypto.Iterations_v2)
	}
	if DefaultSaltLen != uint8(types.SaltSize_v2) || DefaultIVLen != uint8(types.IVSize_v2) {
		t.Fatalf("盐/IV 长度与 types 脱钩: %d/%d", DefaultSaltLen, DefaultIVLen)
	}
	if HintSize != crypto.PasswordHintSize {
		t.Fatalf("hint 长度与加密层脱钩: %d vs %d", HintSize, crypto.PasswordHintSize)
	}
}

// TestRoundTrip 覆盖两种 CipherMode 与多种明文长度。
func TestRoundTrip(t *testing.T) {
	passwords := []string{"", "a", "my-encv_key，可以使用中文和标点符号✔", strings.Repeat("长密码", 50)}
	payloads := [][]byte{
		{},
		[]byte("x"),
		[]byte("# 标题\n正文 🔐\n"),
		bytes.Repeat([]byte{0xAB}, 1<<16),
	}
	for _, mode := range []uint16{uint16(CipherAES128CTR), uint16(CipherAES256CTR)} {
		for _, pw := range passwords {
			for _, plain := range payloads {
				opts := Options{CipherMode: mode}
				blob, err := Encrypt(plain, pw, opts)
				if err != nil {
					t.Fatalf("mode=%d 加密失败: %v", mode, err)
				}
				got, err := Decrypt(blob, pw)
				if err != nil {
					t.Fatalf("mode=%d 解密失败: %v", mode, err)
				}
				if !bytes.Equal(got, plain) {
					t.Fatalf("mode=%d 明文不一致: len=%d", mode, len(plain))
				}
				if len(blob) != HeaderSize+int(DefaultSaltLen)+int(DefaultIVLen)+len(plain) {
					t.Fatalf("mode=%d 信封长度错误: %d", mode, len(blob))
				}
			}
		}
	}
}

// TestWrongPassword 密码错误必须明确报错（types.ErrWrongPassword），而不是返回乱码。
func TestWrongPassword(t *testing.T) {
	blob, err := Encrypt([]byte("secret note"), "correct horse", DefaultOptions())
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if _, err := Decrypt(blob, "wrong horse"); !errors.Is(err, types.ErrWrongPassword) {
		t.Fatalf("期望 ErrWrongPassword，实际: %v", err)
	}
	if VerifyPassword(blob, "wrong horse") {
		t.Fatal("VerifyPassword 对错误密码返回了 true")
	}
	if !VerifyPassword(blob, "correct horse") {
		t.Fatal("VerifyPassword 对正确密码返回了 false")
	}
}

// TestRejectsForeignOrBrokenInput 保证非法输入被拒绝而不是 panic 或静默产出垃圾。
func TestRejectsForeignOrBrokenInput(t *testing.T) {
	cases := map[string][]byte{
		"empty":          {},
		"encv-container": append([]byte("ENCV"), make([]byte, 2048)...),
		"random":         []byte("not an encvs1 blob at all"),
	}
	for name, blob := range cases {
		if _, err := Decrypt(blob, "pw"); err == nil {
			t.Fatalf("%s 应当解密失败", name)
		}
	}
	// 截断：砍掉密文尾部仍应被识别为结构异常或解出更短内容，但绝不能 panic。
	blob, err := Encrypt([]byte("hello"), "pw", DefaultOptions())
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	_, _ = Decrypt(blob[:HeaderSize+int(DefaultSaltLen)+int(DefaultIVLen)-1], "pw")
}

// TestGoldenVectors_Derivations 校验密钥派生与黄金向量一致（锁住 PBKDF2 参数）。
func TestGoldenVectors_Derivations(t *testing.T) {
	v := loadVectors(t)
	for _, c := range v.Derivations {
		salt, err := hex.DecodeString(c.SaltHex)
		if err != nil {
			t.Fatalf("%s: salt 解析失败: %v", c.Name, err)
		}
		got := hex.EncodeToString(crypto.GenerateKey(c.Password, salt, c.KeyLen))
		if got != c.KeyHex {
			t.Fatalf("%s: 派生密钥不一致\n want=%s\n got =%s", c.Name, c.KeyHex, got)
		}
	}
}

// TestGoldenVectors_Cases 是「Go 原生 ↔ WASM」的契约锁：
// 前端测试读同一个 vectors.json，断言 wasm 产出同样的 containerHex / plaintext。
func TestGoldenVectors_Cases(t *testing.T) {
	v := loadVectors(t)
	if v.Spec != "ENCVS1" || v.FormatVersion != FormatVersion || v.HeaderSize != HeaderSize || v.Iterations != DefaultIterations {
		t.Fatalf("向量元信息不匹配: spec=%s ver=%d header=%d iter=%d", v.Spec, v.FormatVersion, v.HeaderSize, v.Iterations)
	}
	for _, c := range v.Cases {
		want, err := hex.DecodeString(c.ContainerHex)
		if err != nil {
			t.Fatalf("%s: container 解析失败: %v", c.Name, err)
		}
		plain, err := hex.DecodeString(c.PlaintextHex)
		if err != nil {
			t.Fatalf("%s: plaintext 解析失败: %v", c.Name, err)
		}
		salt, err := hex.DecodeString(c.SaltHex)
		if err != nil {
			t.Fatalf("%s: salt 解析失败: %v", c.Name, err)
		}
		iv, err := hex.DecodeString(c.IVHex)
		if err != nil {
			t.Fatalf("%s: iv 解析失败: %v", c.Name, err)
		}

		// ① 用同样的 salt/iv 重新加密，必须字节级复现。
		got, err := EncryptWithEntropy(plain, c.Password, Options{CipherMode: c.CipherMode}, salt, iv)
		if err != nil {
			t.Fatalf("%s: 加密失败: %v", c.Name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: 信封字节不一致\n want=%s\n got =%s", c.Name, c.ContainerHex, hex.EncodeToString(got))
		}

		// ② 解密黄金信封，必须还原明文（锁住读路径）。
		gotPlain, err := Decrypt(want, c.Password)
		if err != nil {
			t.Fatalf("%s: 解密失败: %v", c.Name, err)
		}
		if !bytes.Equal(gotPlain, plain) {
			t.Fatalf("%s: 明文不一致", c.Name)
		}
	}
}

// TestIterationsCannotDrift 保证「迭代次数不可配置」这条约束真的生效。
func TestIterationsCannotDrift(t *testing.T) {
	opts := Options{Iterations: DefaultIterations + 1}
	if _, err := EncryptWithEntropy([]byte("x"), "pw", opts, make([]byte, DefaultSaltLen), make([]byte, DefaultIVLen)); !errors.Is(err, ErrUnsupportedIter) {
		t.Fatalf("期望 ErrUnsupportedIter，实际: %v", err)
	}
}
