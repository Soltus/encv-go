package text

import (
	"encoding/json"
	"testing"
)

// TestTextIndexJSONFieldNames 锁住 TextIndex 序列化后的字段名。
//
// 为什么需要这个测试：cmd/encv-wasm-container（浏览器端加密）**不能 import 本包**
// —— 本包间接依赖 atomicgo.dev/keyboard，在 js/wasm 下编译不过 ——
// 只能按字段名手写一份等价的 JSON（见那里的 textIndexJSON）。
// 一旦这里改了字段名/加字段，wasm 产出的容器就会缺 index 而解不开，
// 所以必须有测试把这份隐式契约钉死。
func TestTextIndexJSONFieldNames(t *testing.T) {
	raw, err := json.Marshal(&TextIndex{
		ID:                "0",
		OriginalFileSize:  1,
		MimeType:          "text/plain",
		Format:            "plain",
		OriginalFilename:  "a.txt",
		OriginalInputPath: "a.txt",
		OriginalFileMD5:   "md5",
		EncryptedFileMD5:  "md5",
	})
	if err != nil {
		t.Fatalf("序列化失败：%v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("反序列化失败：%v", err)
	}
	for _, name := range []string{
		"id",
		"original_file_size",
		"mime_type",
		"format",
		"original_filename",
		"originalInputPath",
		"original_file_md5",
		"encrypted_file_md5",
	} {
		if _, ok := fields[name]; !ok {
			t.Errorf("TextIndex 缺少字段 %q（wasm 侧依赖这个名字，见 cmd/encv-wasm-container 的 textIndexJSON）", name)
		}
	}
}
