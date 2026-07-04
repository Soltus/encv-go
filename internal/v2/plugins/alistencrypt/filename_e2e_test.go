package alistencrypt

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEndToEndFilenameRoundtrip 验证完整业务流：加密容器文件 → 解密后文件名还原
// 这是用户实际使用场景，测试在 tmpdir 创建真实 .bin 容器，验证输出文件名
func TestEndToEndFilenameRoundtrip(t *testing.T) {
	testCases := []struct {
		name     string
		filename string
		password string
	}{
		{"ascii_video", "sample.mp4", "password123"},
		{"chinese_video", "测试视频.mp4", "密码123"},
		{"plain_text", "secret.txt", "pw"},
		{"no_extension", "README", "pw"},
		{"multi_dots", "my.file.name.tar.gz", "pw"},
		{"unicode_long", "長い日本語のファイル名.jpg", "パスワード"},
		{"empty_password", "test.png", ""},
		{"short_password", "1.mp4", "a"},
	}

	encType := "aesctr"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// 准备真实可加密的明文 payload
			plaintext := bytes.Repeat([]byte("dummy_payload_"), 64)
			inputPath := filepath.Join(tmpDir, tc.filename)
			if err := os.WriteFile(inputPath, plaintext, 0644); err != nil {
				t.Fatal(err)
			}

			// 用真实 EncryptToFile 走完整加密流程
			settings := &AlistEncryptPluginConfig{
				Suffix:          ".bin",
				DefaultPassword: tc.password,
				EncType:         encType,
			}
			encResult, err := EncryptToFile(bytes.NewReader(plaintext), tc.password, tmpDir, settings)
			if err != nil {
				t.Fatalf("EncryptToFile failed: %v", err)
			}

			// 走 RenameToFinalEncrypted 命名（这是 PostEncryptProcessor 真实流程）
			finalPath, err := RenameToFinalEncrypted(encResult.TempPath, tc.filename, tmpDir, ".bin", tc.password, encType)
			if err != nil {
				t.Fatalf("RenameToFinalEncrypted failed: %v", err)
			}
			encBaseName := filepath.Base(finalPath)

			if !strings.HasSuffix(encBaseName, ".bin") {
				t.Errorf("encrypted file should end with .bin, got %q", encBaseName)
			}

			// 解密到另一个目录，验证输出文件名 == 原文件名
			outDir := t.TempDir()
			outputPath, err := DecryptFile(finalPath, outDir, tc.password, encType)
			if err != nil {
				t.Fatalf("DecryptFile failed: %v", err)
			}

			gotName := filepath.Base(outputPath)
			if gotName != tc.filename {
				t.Errorf("decrypted filename mismatch\n  original: %q\n  encoded:  %q\n  decoded:  %q",
					tc.filename, encBaseName, gotName)
			}

			// 验证文件内容确实解密回原 payload
			decoded, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("read decrypted file failed: %v", err)
			}
			if !bytes.Equal(decoded, plaintext) {
				t.Errorf("decrypted content mismatch: want %d bytes, got %d", len(plaintext), len(decoded))
			}
		})
	}
}

// TestDecryptFileWrongPassword 密码错误时 DecryptFile 必须报密码错（不静默写文件）
// 范式：CRC6 校验失败 = 密码错，应该早返回，不能产生"丢 ext 的文件名"
func TestDecryptFileWrongPassword(t *testing.T) {
	tmpDir := t.TempDir()
	plaintext := bytes.Repeat([]byte("x"), 1024)
	inputName := "测试视频.mp4"
	inputPath := filepath.Join(tmpDir, inputName)
	if err := os.WriteFile(inputPath, plaintext, 0644); err != nil {
		t.Fatal(err)
	}

	// 正确密码加密
	settings := &AlistEncryptPluginConfig{Suffix: ".bin", DefaultPassword: "correct_pw", EncType: "aesctr"}
	encResult, err := EncryptToFile(bytes.NewReader(plaintext), "correct_pw", tmpDir, settings)
	if err != nil {
		t.Fatal(err)
	}
	containerPath, err := RenameToFinalEncrypted(encResult.TempPath, inputName, tmpDir, ".bin", "correct_pw", "aesctr")
	if err != nil {
		t.Fatal(err)
	}

	// 错误密码解密 → 必须报错
	outDir := t.TempDir()
	_, err = DecryptFile(containerPath, outDir, "wrong_pw", "aesctr")
	if err == nil {
		t.Fatal("DecryptFile should fail for wrong password (CRC6 mismatch)")
	}
	if !strings.Contains(err.Error(), "password mismatch") {
		t.Errorf("expected password mismatch error, got: %v", err)
	}

	// 关键：密码错时绝不能在 outDir 创建任何文件（"丢 ext" 的文件名就是从这里来的）
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("no file should be created on wrong password, found: %v", names)
	}
}

// TestDecryptFileUnrecognizedContainer 容器文件不带可识别编码（CRC6 必失败）时必须报密码错
// 模拟历史数据：旧插件未编码 baseName 的文件
func TestDecryptFileUnrecognizedContainer(t *testing.T) {
	tmpDir := t.TempDir()
	plaintext := bytes.Repeat([]byte("legacy_data_"), 64)

	// 模拟旧插件生成的未编码容器
	legacyContainer := filepath.Join(tmpDir, "temp1234.bin")
	if err := os.WriteFile(legacyContainer, plaintext, 0644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	_, err := DecryptFile(legacyContainer, outDir, "any_password", "aesctr")
	if err == nil {
		t.Fatal("DecryptFile should fail when filename CRC6 cannot be validated")
	}

	// 不应该创建文件
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Errorf("no file should be created for unrecognized container")
	}
}

// TestTrimContainerExt 直接验证兜底辅助函数
func TestTrimContainerExt(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"foo.bin", "foo"},
		{"foo.alist", "foo"},
		{"foo.enc", "foo"},
		{"foo.bin.bin", "foo.bin"}, // 只剥一次
		{"foo", "foo"},             // 无后缀
		{"foo.txt", "foo.txt"},     // 非容器后缀不动
		{"encoded_test.mp4", "encoded_test.mp4"},
	}
	for _, c := range cases {
		got := TrimContainerExt(c.in)
		if got != c.want {
			t.Errorf("TrimContainerExt(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
