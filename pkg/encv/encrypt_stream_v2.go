// pkg/encv/encrypt_stream_v2.go
//
// 主应用的**流式**加密入口：与 wasm 预览页共用同一个编排包
// （internal/v2/container/compose），因此"容器怎么拼、密钥怎么派生"只有一份实现。
//
// 与插件路径（EncryptPathV2）的分工：
//   - 插件路径：先按类型预处理（视频抽帧/GOP 对齐、压缩…），再写容器；
//     大文件的代价是要走一遍插件自己的流水线。
//   - 流式路径：明文分片读入、密文段随写随出，**不落临时文件、不整体进内存**，
//     也不跑插件预处理。适合"就想把一个大文件封进容器"的场景
//     （备份/上传前加密/移动端内存吃紧时）。
//
// 产物仍然带着插件 index（从插件的 MetadataExtractor 取），所以**插件能解**——
// 这一点由 internal/v2/container/compose/contract_test.go 锁住。
package encv

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Soltus/encv-go/internal/v2/container/compose"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// StreamChunk 流式加密时每次读盘的块大小：它决定内存占用上限，与文件大小无关。
const StreamChunk = 1 << 20 // 1MB

// EncryptFileStreamV2 把 inputPath 流式加密成容器 outputPath。
//
// 内存账：常驻约 SegmentSize + StreamChunk，与文件本身大小无关。
// 磁盘账：直接写 outputPath（先占位 2048B 的容器头，收尾时回填），没有 .tmp 中转。
func EncryptFileStreamV2(ctx context.Context, inputPath, outputPath, password string) error {
	if password == "" {
		return fmt.Errorf("流式加密需要口令")
	}
	in, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("打开输入文件失败：%w", err)
	}
	defer in.Close()

	// 插件 index：产物要能被"对应插件"解开，就得带上插件自己那份 index。
	// 用 KVIExtra 注入（compose 不认识 6 种 index 结构，也不该认识）。
	index, p, err := pluginIndex(inputPath)
	if err != nil {
		return err
	}
	// 键名规则就是 "<插件名>_index"（text → text_index、video → video_index…），
	// 与各插件 KVI 结构的 json tag 一致。
	kviKey := p.Name() + "_index"

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建输出文件失败：%w", err)
	}
	defer out.Close()

	// 容器头固定 2048B（types.EnvelopeHeaderSize_v4），先占位，收尾时回填 ——
	// 这样数据可以边算边落盘，不必先攒到临时文件再拼。
	if _, err := out.Write(make([]byte, types.EnvelopeHeaderSize_v4)); err != nil {
		return fmt.Errorf("写容器头占位失败：%w", err)
	}

	sink := &fileStreamSink{f: out}
	enc, err := compose.NewStreamEncryptor(compose.Options{
		Password:      password,
		ContainerType: p.ContainerType(),
		// 1MB 段而不用 writer 默认的 4MB：实测 40MB 文件时 4MB 段的峰值 RSS 约 44MB、
		// 1MB 段约 12MB，而容器只多几个段头。主应用这边段大小只影响内存与 seek 粒度，
		// 没有必须 4MB 的理由（wasm 预览页另按自己的块大小给）。
		SegmentSize:  1 << 20,
		OriginalName: index.GetOriginalFilename(),
		MimeType:     index.GetMimeType(),
		Format:       formatOf(index),
		KVIExtra: func(plainSize int64, plainMD5 string) map[string]interface{} {
			return map[string]interface{}{kviKey: index}
		},
	}, sink)
	if err != nil {
		return fmt.Errorf("创建流式加密器失败：%w", err)
	}

	buf := make([]byte, StreamChunk)
	if _, err := io.CopyBuffer(enc, in, buf); err != nil {
		return fmt.Errorf("加密写入失败：%w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("收尾失败：%w", err)
	}

	// 回填容器头 + 追加 manifest/footer
	if len(sink.head) != types.EnvelopeHeaderSize_v4 {
		return fmt.Errorf("容器头长度异常：%d，期望 %d", len(sink.head), types.EnvelopeHeaderSize_v4)
	}
	if _, err := out.WriteAt(sink.head, 0); err != nil {
		return fmt.Errorf("回填容器头失败：%w", err)
	}
	if _, err := out.Write(sink.tail); err != nil {
		return fmt.Errorf("写 manifest/footer 失败：%w", err)
	}
	return out.Close()
}

// pluginIndex 取插件为该文件生成的 index，以及它在 KVI 里的键名。
//
// 键名规则就是 "<插件名>_index"（text → text_index、video → video_index…），
// 与各插件 KVI 结构的 json tag 一致。
func pluginIndex(inputPath string) (types.Index, plugins.Plugin, error) {
	p, err := plugins.FindEncryptingPlugin(inputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("没有插件能处理 '%s'：%w", inputPath, err)
	}
	extractor := p.GetMetadataExtractor()
	if extractor == nil {
		return nil, nil, fmt.Errorf("插件 '%s' 不提供 index 抽取，无法走流式通道（请用普通加密路径）", p.Name())
	}
	index, err := extractor.ExtractMetadata(inputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("抽取 index 失败：%w", err)
	}
	return index, p, nil
}

// formatOf 取 index 里的格式（video 等插件有）；没有就给个保守默认值。
func formatOf(index types.Index) string {
	if f, ok := index.(interface{ GetFormat() string }); ok && f.GetFormat() != "" {
		return f.GetFormat()
	}
	return "plain"
}

// fileStreamSink 把随写随出的密文段直接落盘；容器头与尾留到收尾时处理。
type fileStreamSink struct {
	f    *os.File
	head []byte
	tail []byte
}

func (s *fileStreamSink) Append(p []byte) error {
	_, err := s.f.Write(p)
	return err
}

func (s *fileStreamSink) Finish(head []byte, tail []byte) error {
	s.head = append([]byte(nil), head...)
	s.tail = append([]byte(nil), tail...)
	return nil
}
