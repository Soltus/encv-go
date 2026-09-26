// Command encv-crypto-vectors 生成 ENCVS1 的黄金测试向量（golden vectors）。
//
// 这些向量是「加密层对等」的契约锁：
//   - Go 原生测试（internal/v2/crypto/simple/simple_test.go）读取它，断言本仓实现仍然产出同样的字节；
//   - 前端 WASM 测试（app/packages/encv-crypto/test/parity.test.mjs）读取同一个文件，
//     断言浏览器里跑的那份 wasm 产出同样的字节。
//
// 两边读同一个文件 → 任何一侧改了算法（迭代次数、盐长度、CTR 用法、hint 算法）都会立刻红。
//
// 用法：
//
//	go run ./cmd/encv-crypto-vectors                       # 写到默认路径
//	go run ./cmd/encv-crypto-vectors -out /tmp/vec.json     # 指定路径
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/crypto/simple"
)

const defaultOut = "internal/v2/crypto/simple/testdata/vectors.json"

// fixedEntropy 用一个确定性的字节生成器填充 salt/iv，保证每次生成的向量完全一致。
func fixedEntropy(seed byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = seed + byte(i*7)
	}
	return out
}

type derivationCase struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	SaltHex  string `json:"saltHex"`
	KeyLen   int    `json:"keyLen"`
	KeyHex   string `json:"keyHex"`
}

type vectorCase struct {
	Name         string `json:"name"`
	Password     string `json:"password"`
	CipherMode   uint16 `json:"cipherMode"`
	SaltHex      string `json:"saltHex"`
	IVHex        string `json:"ivHex"`
	PlaintextHex string `json:"plaintextHex"`
	ContainerHex string `json:"containerHex"`
}

type vectors struct {
	Spec          string           `json:"spec"`
	FormatVersion int              `json:"formatVersion"`
	KDF           string           `json:"kdf"`
	Iterations    uint32           `json:"iterations"`
	HeaderSize    int              `json:"headerSize"`
	Magic         string           `json:"magic"`
	GeneratedBy   string           `json:"generatedBy"`
	Derivations   []derivationCase `json:"derivations"`
	Cases         []vectorCase     `json:"cases"`
}

func main() {
	out := flag.String("out", defaultOut, "输出 JSON 路径")
	flag.Parse()

	if err := run(*out); err != nil {
		fmt.Fprintf(os.Stderr, "生成失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ 黄金向量已写入 %s\n", *out)
}

func run(out string) error {
	v := vectors{
		Spec:          "ENCVS1",
		FormatVersion: simple.FormatVersion,
		KDF:           "PBKDF2-HMAC-SHA256",
		Iterations:    simple.DefaultIterations,
		HeaderSize:    simple.HeaderSize,
		Magic:         simple.Magic,
		GeneratedBy:   "cmd/encv-crypto-vectors",
	}

	// ① 密钥派生向量：锁住 PBKDF2(password, salt, 10000, keyLen, SHA256)。
	deriveSalt := fixedEntropy(0x11, int(simple.DefaultSaltLen))
	derivePassword := "my-encv_key，可以使用中文和标点符号✔"
	for _, keyLen := range []int{16, 32} {
		v.Derivations = append(v.Derivations, derivationCase{
			Name:     fmt.Sprintf("pbkdf2-sha256-keylen-%d", keyLen),
			Password: derivePassword,
			SaltHex:  hex.EncodeToString(deriveSalt),
			KeyLen:   keyLen,
			KeyHex:   hex.EncodeToString(crypto.GenerateKey(derivePassword, deriveSalt, keyLen)),
		})
	}

	// ② 信封向量：固定 salt/iv，保证字节级可复现。
	type spec struct {
		name     string
		password string
		mode     uint16
		plain    []byte
		seed     byte
	}
	note := "# 秘密笔记\n\n- 中文 ✔\n- emoji 🔐\n- tail\n"
	big := make([]byte, 4096)
	for i := range big {
		big[i] = byte(i % 251)
	}
	specs := []spec{
		{name: "empty-aes256", password: "p@ss", mode: uint16(simple.CipherAES256CTR), plain: []byte{}, seed: 0x01},
		{name: "ascii-aes256", password: "p@ss", mode: uint16(simple.CipherAES256CTR), plain: []byte("hello encv"), seed: 0x02},
		{name: "markdown-aes256", password: "my-encv_key，可以使用中文和标点符号✔", mode: uint16(simple.CipherAES256CTR), plain: []byte(note), seed: 0x03},
		{name: "markdown-aes128", password: "my-encv_key，可以使用中文和标点符号✔", mode: uint16(simple.CipherAES128CTR), plain: []byte(note), seed: 0x04},
		{name: "binary-4k-aes256", password: "123456", mode: uint16(simple.CipherAES256CTR), plain: big, seed: 0x05},
	}

	for _, s := range specs {
		salt := fixedEntropy(s.seed, int(simple.DefaultSaltLen))
		iv := fixedEntropy(s.seed+0x40, int(simple.DefaultIVLen))
		opts := simple.Options{
			CipherMode: s.mode,
			Iterations: simple.DefaultIterations,
			SaltLen:    simple.DefaultSaltLen,
		}
		blob, err := simple.EncryptWithEntropy(s.plain, s.password, opts, salt, iv)
		if err != nil {
			return fmt.Errorf("case %q 加密失败: %w", s.name, err)
		}
		// 自检：生成的向量必须能被自己解开，避免把「生成时就错了」的向量固化成契约。
		got, err := simple.Decrypt(blob, s.password)
		if err != nil {
			return fmt.Errorf("case %q 解密自检失败: %w", s.name, err)
		}
		if !bytes.Equal(got, s.plain) {
			return fmt.Errorf("case %q 自检不一致", s.name)
		}
		v.Cases = append(v.Cases, vectorCase{
			Name:         s.name,
			Password:     s.password,
			CipherMode:   s.mode,
			SaltHex:      hex.EncodeToString(salt),
			IVHex:        hex.EncodeToString(iv),
			PlaintextHex: hex.EncodeToString(s.plain),
			ContainerHex: hex.EncodeToString(blob),
		})
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if dir := filepath.Dir(out); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(out, data, 0o644)
}
