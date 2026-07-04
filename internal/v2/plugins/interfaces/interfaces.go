package interfaces

import (
	"io"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// MetadataExtractor 定义了如何从文件中提取元数据的策略
type MetadataExtractor interface {
	// ExtractMetadata 从给定的文件路径提取元数据
	ExtractMetadata(inputPath string) (types.Index, error)
}

// ContentPreprocessor 定义了如何预处理文件内容的策略
// 例如，视频需要 FFmpeg 转码，而图片或文档可能不需要任何处理
type ContentPreprocessor interface {
	// Preprocess 接收原始文件路径，返回一个处理后的内容读取器
	// 调用者负责关闭返回的 io.ReadCloser
	Preprocess(inputPath string) (io.ReadCloser, error)
}

// VerifyWarning 表示验证过程中产生的非致命警告信息
type VerifyWarning struct {
	CheckName string `json:"check_name"`
	Message   string `json:"message"`
	Severity  string `json:"severity"`
}

// VerifyOptions 定义 Verify 方法的可选行为参数
type VerifyOptions struct {
	SkipSizeCheck   bool // 跳过精确文件大小比对（用于重编码/转码模式，此时原始文件与解密文件大小天然不同）
	SkipStructCheck bool // 跳过结构完整性检查（用于重编码输出，此时 MP4 结构可能完全不同）
	SkipDeepCheck   bool // 跳过深度完整性检查 L4（用于 PostEncryptProcessor，v4 容器加密后 MP4 结构必然改变）
	CollectWarnings bool // 收集 warnings 而非忽略（默认为 false，warnings 被丢弃）
}

// ContentVerifier 定义了插件校验解密内容完整性的能力。
// 这是一个可选接口，插件应根据自身特性（如是否支持随机访问、文件大小）来实现。
type ContentVerifier interface {
	// Verify 校验解密后的文件与原始文件是否一致。
	// originalPath: 原始输入文件路径。
	// decryptedPath: 经过加密再解密后的文件路径（通常位于临时目录）。
	// opts: 可选的验证选项，不传时使用默认严格模式。
	// 返回 error 表示校验失败，返回的 warnings 列表包含非致命警告信息（仅在 CollectWarnings=true 时有效）。
	Verify(originalPath, decryptedPath string, opts ...*VerifyOptions) (error, []*VerifyWarning)
}

// FragmentBuilder 定义了自定义逻辑分片策略的接口（如视频 GOP 对齐）
type FragmentBuilder interface {
	// BuildFragments 根据逻辑文件大小生成分片元数据
	BuildFragments(logicalFileSize int64) ([]types.Fragment, error)
}

// PasswordStrategy 声明插件的密码使用策略
type PasswordStrategy string

const (
	PasswordGlobal      PasswordStrategy = "global"      // 使用全局密码（video 等大多数插件）
	PasswordIndependent PasswordStrategy = "independent" // 使用插件独立密码，不用全局密码（alist_encrypt）
	PasswordNone        PasswordStrategy = "none"        // 不需要密码
)

// TaskOptions 返回该插件在创建加解密任务时需要的选项声明
// 前端根据此声明动态渲染表单字段，无需硬编码插件特定逻辑
type TaskOptions struct {
	PasswordStrategy     PasswordStrategy `json:"passwordStrategy"`
	SupportVersionSelect bool             `json:"supportVersionSelect"`
	SupportedVersions    []int            `json:"supportedVersions,omitempty"`
	DefaultVersion       int              `json:"defaultVersion"`
	ExtraFields          []TaskField      `json:"extraFields,omitempty"`
}

// TaskField 声明任务创建时的额外输入字段
type TaskField struct {
	Key          string            `json:"key"`
	Label        string            `json:"label"`
	Type         string            `json:"type"`
	Required     bool              `json:"required"`
	DefaultValue string            `json:"defaultValue"`
	Help         string            `json:"help"`
	Options      []string          `json:"options,omitempty"`
	OptionLabels map[string]string `json:"optionLabels,omitempty"`
	Condition    string            `json:"condition,omitempty"`
}

// TaskPasswordResolver 定义插件自定义主密码解析能力
// 插件根据 ExtraFields 和策略返回主密码（L0 或 L1）
// L2 二级密码不在此接口处理，由 TaskManager 单独传递
type TaskPasswordResolver interface {
	ResolveTaskPassword(taskPassword string, extraFields map[string]string) string
}

// TaskExtraFieldsSetter 定义插件接收任务级别 ExtraFields 的能力
// 声明式架构：插件通过 GetTaskOptions() 声明 ExtraFields，
// 前端渲染对应输入，后端在执行前通过此接口将用户输入注入插件实例
type TaskExtraFieldsSetter interface {
	SetTaskExtraFields(fields map[string]string)
}

type TaskStateResetter interface {
	ResetTaskState()
}

// ─── 全文搜索内容声明（2026-07-02 用户反馈：插件自主声明容器内可被全文搜索的内容）──

// SearchableContentType 预定义的搜索内容类型。
// 插件可在 SearchableContentsManifest.Types 里引用这些常量。
const (
	SearchableTypeSubtitle  = "subtitle"  // 字幕
	SearchableTypeTitle     = "title"     // 标题
	SearchableTypeLyrics    = "lyrics"    // 歌词
	SearchableTypeMetadata  = "metadata"  // 元数据
	SearchableTypeBody      = "body"      // 正文（如 text 插件整文）
	SearchableTypeChapters  = "chapters"  // 章节
	SearchableTypeOCR       = "ocr"       // OCR 文本（图片插件）
	SearchableTypeTranscript = "transcript" // 语音转写（音频/视频）
)

// SearchableContentsManifest 声明插件**支持**哪些类型的可搜索内容。
//
// 用户层面意义：UI 显示「此插件可全文搜索: 字幕、标题、章节」
// （透明告知用户加密容器可以被搜到什么内容）
//
// 插件实现示例（video 插件）：
//   func (p *VideoPlugin) GetSearchableContentsManifest() SearchableContentsManifest {
//     return SearchableContentsManifest{Enabled: true, Types: []string{
//       SearchableTypeSubtitle, SearchableTypeTitle, SearchableTypeChapters,
//     }}
//   }
type SearchableContentsManifest struct {
	Enabled bool     // false = 插件不参与全文搜索（容器内任何内容都不进 FTS5）
	Types   []string // 插件支持的可搜索内容类型（见 SearchableType* 常量）
}

// SearchableContentItem 插件从容器里提取出来的单条可搜索内容。
// 多条可以返回：例如视频可能有 zh-CN/EN/JP 多个字幕轨道。
type SearchableContentItem struct {
	Type string // 对应 SearchableContentsManifest.Types 里的某一项
	Name string // 详细名称（如 "subtitle:zh-CN"、"title"、"metadata:artist"）
	Text string // 实际可搜索的纯文本（去除二进制/格式化符号）
}

// SearchableContentsExtractor 定义从加密容器中提取可搜索内容的能力。
//
// FTS5 索引构建（mobile_search_fulltext.go）会：
//   1. 扫描到 .sccgv/.ae 等容器文件
//   2. 调 ExtractSearchableContents() 拿到 SearchableContentItem 列表
//   3. 把每条 item 写成 FileEntry{Content: item.Text, ...} 进 FTS5
//
// 返回 error 表示该容器提取失败（跳过该文件即可，不阻断）
type SearchableContentsExtractor interface {
	// GetSearchableContentsManifest 声明插件支持哪些类型
	GetSearchableContentsManifest() SearchableContentsManifest
	// ExtractSearchableContents 从容器文件中提取可被搜索的内容
	// containerPath: 容器文件绝对路径
	// 返回: 多个 item + error（失败时 error != nil，但 item 可能部分成功）
	ExtractSearchableContents(containerPath string) ([]SearchableContentItem, error)
}
