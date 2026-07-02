// internal/v2/plugins/registry.go

package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/plugins/alistencrypt"
	"github.com/Soltus/encv-go/internal/v2/plugins/audio"
	"github.com/Soltus/encv-go/internal/v2/plugins/image"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/Soltus/encv-go/internal/v2/plugins/pdf"
	"github.com/Soltus/encv-go/internal/v2/plugins/text"
	"github.com/Soltus/encv-go/internal/v2/plugins/video"
	"github.com/Soltus/encv-go/internal/v2/plugins/wps"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
)

// Plugins 是所有已注册插件的列表，顺序代表优先级
var Plugins = []Plugin{
	&video.VideoPlugin{},
	&audio.AudioPlugin{},
	&image.ImagePlugin{},
	&wps.WPSPlugin{},
	&pdf.PDFPlugin{},
	&text.TextPlugin{},
	&alistencrypt.AlistEncryptPlugin{},
}

// Plugin 定义了加解密插件的完整接口
type Plugin interface {
	// 【新增】插件必须实现此方法，返回其唯一的名称标识符
	// 这个名称将作为在配置文件中查找其配置的键。
	Name() string
	// 【新增】插件必须实现此方法，返回其默认配置的 JSON 字节流
	GetDefaultSettings() json.RawMessage
	// 【新增】返回插件配置结构体的零值实例，用于生成 JSON Schema
	GetSettingsSchemaType() interface{}

	//  返回此插件创建的容器文件扩展名，包含点前缀
	GetContainerExtension() string
	// 这将用于在 OpenList 等外部系统中动态生成配置界面
	GetSettingFields() []pluginInterfaces.SettingField

	Initialize(ctx context.Context) error

	// --- 提供处理策略 ---
	GetMetadataExtractor() pluginInterfaces.MetadataExtractor
	GetContentPreprocessor() pluginInterfaces.ContentPreprocessor
	// 保留接口，校验暂时由插件内部编排
	GetContentVirifier() pluginInterfaces.ContentVerifier

	// --- 文件识别与处理 ---
	// GetChunkNamer 返回插件用于其容器的 ChunkNamer。
	GetChunkNamer() namer.ChunkNamer

	//  返回此插件支持的 MIME 类型前缀列表，用于匹配，优先级最高
	SupportedMimePrefixes() []string
	//  返回此插件支持的文件扩展名列表（不含前缀点），用于匹配，优先级次之
	SupportedExtensions() []string
	// 用于排除不想处理的文件，返回 false 表示不处理
	ShouldProcess(inputPath string) bool

	// GroupFiles 允许插件在处理前对输入文件列表进行分组或转换。
	// 它返回一个新的文件路径列表，系统将对此列表进行后续处理。
	// 如果插件不需要分组，可以返回原始列表和 nil 错误。
	GroupFiles(inputPaths []string, inputRootDir, outputDir string) ([]string, error)

	// --- 加密逻辑 ---

	// 加密预处理器
	PreEncryptProcessor(index types.Index, inputPath, inputRootDir, outputDir string) error
	// 将处理后的文件加密到指定容器
	// Encrypt(dataReader io.Reader) error
	Encrypt(dataReader io.Reader) (*crypto.EncryptionResult, error)
	// 加密后处理器
	// PostEncryptProcessor() error
	PostEncryptProcessor(result *crypto.EncryptionResult) (string, error)
	//  打包器
	// GetPhysicalPacker() physical.PhysicalPacker

	// --- 解密逻辑 ---

	//  判断此插件是否能解密给定的容器文件
	CanDecrypt(containerPath string) bool
	// Unpacker 解包器 TODO
	// 解密预处理器
	PreDecryptProcessor(containerPath, outputDir string) error
	// Decrypt 解密容器文件到指定目录，返回解密产物路径
	Decrypt(containerPath, outputDir string) (string, error)
	// 解密后处理器
	PostDecryptProcessor(containerPath string) error

	// --- v4 特性 ---
	ContainerType() uint16
	DefaultIsSeekable(inputPath string) bool
	DisasterZones(inputPath string) []types.DisasterZone

	// === 版本选择支持 ===
	SupportedContainerVersions() []int
	DefaultContainerVersion() int
	ValidateVersion(version int) error

	// === 任务创建选项声明 ===
	GetTaskOptions() pluginInterfaces.TaskOptions
}

// PluginCandidate 表示一个能处理给定文件的插件候选
// 用于多候选选择场景（与 FindEncryptingPlugin 返回单一结果不同）
type PluginCandidate struct {
	Plugin    Plugin `json:"-"`
	Name      string `json:"name"`
	MatchType string `json:"matchType"`
	Priority  int    `json:"priority"`
}

// ExtensionConflict 表示插件容器扩展名冲突记录
type ExtensionConflict struct {
	Extension   string   // 冲突的扩展名（如 ".sccgv"）
	PluginNames []string // 声明此扩展名的插件列表（通常 ≥2 个）
}

// ValidateExtensionUniqueness 检查所有插件容器扩展名是否唯一
// 纯检测函数，不做策略决策，由调用方决定处理方式
func ValidateExtensionUniqueness() []ExtensionConflict {
	extToPlugins := make(map[string][]string)
	for _, p := range Plugins {
		ext := normalizeExtension(p.GetContainerExtension())
		if ext != "" {
			extToPlugins[ext] = append(extToPlugins[ext], p.Name())
		}
	}
	var conflicts []ExtensionConflict
	for ext, names := range extToPlugins {
		if len(names) > 1 {
			conflicts = append(conflicts, ExtensionConflict{
				Extension:   ext,
				PluginNames: names,
			})
		}
	}
	return conflicts
}

// GetContainerExtensionsMap 返回所有插件的 容器扩展名→插件名 映射
func GetContainerExtensionsMap() map[string]string {
	result := make(map[string]string)
	for _, p := range Plugins {
		ext := normalizeExtension(p.GetContainerExtension())
		if ext != "" && result[ext] == "" {
			result[ext] = p.Name()
		}
	}
	return result
}

// DefaultPluginVersionMethods 提供版本方法的默认实现
func DefaultSupportedVersions() []int {
	return types.SupportedVersions
}

func DefaultContainerVersion() int {
	return types.DefaultContainerVersion
}

func ValidateVersionDefault(version int) error {
	if !types.IsValidVersion(version) {
		return fmt.Errorf("unsupported container version: %d", version)
	}
	if types.IsDeprecatedVersion(version) {
		slog.Warn("using deprecated container version", "version", version)
	}
	return nil
}

// normalizeExtension 确保扩展名带有前导点，使其符合标准格式
func normalizeExtension(ext string) string {
	if !strings.HasPrefix(ext, ".") {
		return "." + ext
	}
	return ext
}

// --- 延迟初始化相关变量 ---
var (
	once              sync.Once
	registeredExtsMap map[string]bool // 存储带点扩展名，用于 O(1) 查找
	registeredExts    []string        // 存储带点扩展名，用于列表返回
	chunkNamersOnce   sync.Once
	allChunkNamers    []namer.ChunkNamer
)

// 在 InitializePlugins 之后调用
func initializeExtensions() {
	registeredExtsMap = make(map[string]bool)
	tempMap := make(map[string]bool)

	// 此时，我们假设所有插件都已经被 Initialize 过了
	for _, p := range Plugins {
		ext := p.GetContainerExtension()
		if ext != "" {
			// 规范化为带点的格式
			normalizedExt := normalizeExtension(ext)
			tempMap[normalizedExt] = true
		}
	}

	// 将最终结果存入缓存
	registeredExts = make([]string, 0, len(tempMap))
	for ext := range tempMap {
		registeredExtsMap[ext] = true
		registeredExts = append(registeredExts, ext)
	}
}

// GetPluginMetas 返回所有已注册插件的配置元信息列表
// 这个函数是 OpenList 动态生成配置界面的入口
func GetPluginMetas() []pluginInterfaces.PluginMeta {
	var metas []pluginInterfaces.PluginMeta
	for _, p := range Plugins {
		metas = append(metas, pluginInterfaces.PluginMeta{
			Name:          p.Name(),
			SettingFields: p.GetSettingFields(), // 调用插件的新方法
		})
	}
	return metas
}

// GetAllRegisteredContainerExtensions 返回所有已注册插件的容器扩展名（带点号）
// 此函数是线程安全的，并且会在第一次被调用时自动完成初始化。
func GetAllRegisteredContainerExtensions() []string {
	once.Do(initializeExtensions)
	return registeredExts
}

// IsContainerPath 检查路径是否是已知的容器文件（基于扩展名）
// 此函数是线程安全的，并且会在第一次被调用时自动完成初始化。
func IsContainer(path string) bool {
	once.Do(initializeExtensions)
	ext := strings.ToLower(filepath.Ext(path))
	return registeredExtsMap[ext]
}

// 这个函数将由 chunkNamersOnce.Do 保证只执行一次。
func initializeChunkNamers() {
	// 此时，我们假设所有插件都已经被 Initialize 过了。
	var tempNamers []namer.ChunkNamer
	for _, p := range Plugins {
		namer := p.GetChunkNamer()
		// 并非所有插件都必须有分片（例如纯文本插件），所以需要检查 nil
		if namer != nil {
			tempNamers = append(tempNamers, namer)
		}
	}
	allChunkNamers = tempNamers
}

func PredictEncryptOutputName(inputPath string, cfg *config.Config) (string, error) {
	plugin, err := FindEncryptingPlugin(inputPath)
	if err != nil {
		return "", err
	}
	ctx := config.NewContext(context.Background(), cfg)
	if err := plugin.Initialize(ctx); err != nil {
		return "", fmt.Errorf("failed to initialize plugin for prediction: %w", err)
	}
	baseNamer := namer.NewDefaultBaseNamer()
	originalFilename := filepath.Base(inputPath)
	encryptedBaseName := baseNamer.GenerateEncryptedBaseName(originalFilename)
	chunkNamer := plugin.GetChunkNamer()
	if chunkNamer != nil {
		return chunkNamer.GenerateMainChunkName(encryptedBaseName), nil
	}
	ext := plugin.GetContainerExtension()
	if ext != "" {
		return encryptedBaseName + ext, nil
	}
	return encryptedBaseName, nil
}

// 【新增】GetAllRegisteredChunkNamers 返回所有已注册插件的 ChunkNamer 列表。
// 此函数是线程安全的，并且会在第一次被调用时自动完成初始化。
func GetAllRegisteredChunkNamers() []namer.ChunkNamer {
	chunkNamersOnce.Do(initializeChunkNamers)
	return allChunkNamers
}

// GetAllRegisteredSearchableExtractors 返回所有实现了 SearchableContentsExtractor 的插件。
// 2026-07-02 用户反馈：插件自主声明容器内可被全文搜索的内容。
// FTS5 索引构建会调本函数拿到所有可提取器，对每个容器调 ExtractSearchableContents()。
// 未实现 SearchableContentsExtractor 的插件被自动跳过（容器不参与全文搜索）。
func GetAllRegisteredSearchableExtractors() []pluginInterfaces.SearchableContentsExtractor {
	var out []pluginInterfaces.SearchableContentsExtractor
	for _, p := range Plugins {
		ext, ok := p.(pluginInterfaces.SearchableContentsExtractor)
		if !ok {
			continue
		}
		if !ext.GetSearchableContentsManifest().Enabled {
			continue
		}
		out = append(out, ext)
	}
	return out
}

// BuildFullPluginSettings 构建一个完整的插件配置映射
// userSettings: 从用户配置文件中读取的原始 map
// 返回一个包含所有插件配置（用户+默认）的完整 map
func BuildFullPluginSettings(userSettings map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	fullSettings := make(map[string]json.RawMessage)

	for _, p := range Plugins {
		name := p.Name()

		// 1. 获取插件的默认配置
		defaults := p.GetDefaultSettings()

		// 2. 检查用户是否为该插件提供了配置
		userProvided, exists := userSettings[name]

		if !exists || len(userProvided) == 0 {
			// 如果用户没有提供配置，则完全使用默认值
			fullSettings[name] = defaults
		} else {
			// 如果用户提供了配置，则将其与默认值合并
			merged, err := utils.MergeJSONObjects(defaults, userProvided)
			if err != nil {
				return nil, fmt.Errorf("failed to merge settings for plugin '%s': %w", name, err)
			}
			fullSettings[name] = merged
		}
	}
	return fullSettings, nil
}

func InitializePlugins(ctx context.Context) error {
	for _, p := range Plugins {
		pluginName := p.Name()
		slog.Info("Initializing plugin", "name", pluginName)

		if err := p.Initialize(ctx); err != nil {
			return fmt.Errorf("failed to initialize plugin %s: %w", pluginName, err)
		}
	}

	if conflicts := ValidateExtensionUniqueness(); len(conflicts) > 0 {
		for _, c := range conflicts {
			slog.Error("container extension conflict detected",
				"extension", c.Extension,
				"conflicting_plugins", strings.Join(c.PluginNames, ", "),
			)
		}
	}

	return nil
}

// FindEncryptingPlugin 为给定的输入文件查找合适的加密插件
// 优先级：
// 1. 通过 MIME 类型前缀匹配
// 2. 如果没有匹配到，则通过文件扩展名匹配
// 3. 最后通过 ShouldProcess 进行最终确认
func FindPluginByName(name string) (Plugin, error) {
	for _, p := range Plugins {
		if p.Name() == name {
			return p, nil
		}
	}
	return nil, fmt.Errorf("plugin not found: %s", name)
}

func FindEncryptingPlugin(inputPath string) (Plugin, error) {
	ext := strings.ToLower(filepath.Ext(inputPath))
	mimeType, err := utils.DetectFileMIMEType(inputPath)
	if err != nil {
		slog.Debug("Could not determine MIME type, skipping MIME-based match", "path", inputPath)
	}

	var candidates []Plugin

	// --- 阶段 1: MIME 类型匹配 (优先) ---
	if mimeType != "" {
		slog.Debug("Trying MIME-based match", "path", inputPath, "mime", mimeType)
		for _, p := range Plugins {
			for _, prefix := range p.SupportedMimePrefixes() {
				if strings.HasPrefix(mimeType, prefix) {
					slog.Debug("Plugin is a MIME candidate", "path", inputPath, "plugin", fmt.Sprintf("%T", p), "prefix", prefix)
					candidates = append(candidates, p)
					break // 找到一个匹配的前缀就足够了，进入下一个插件
				}
			}
		}
	} else {
		slog.Debug("Could not determine MIME type, skipping MIME-based match", "path", inputPath)
	}

	// --- 阶段 2: 文件扩展名匹配 (兜底) ---
	// 如果没有从 MIME 匹配中找到候选插件，则尝试扩展名匹配
	if len(candidates) == 0 {
		slog.Debug("No MIME-based candidates found, trying extension-based match", "path", inputPath)
		// 获取不带点的扩展名，用于比较
		extWithoutDot := ext
		if len(extWithoutDot) > 0 {
			extWithoutDot = extWithoutDot[1:]
		}

		for _, p := range Plugins {
			for _, supportedExt := range p.SupportedExtensions() {
				// 比较时不区分大小写
				if strings.ToLower(supportedExt) == extWithoutDot {
					slog.Debug("Plugin is an extension candidate", "path", inputPath, "plugin", fmt.Sprintf("%T", p), "extension", supportedExt)
					candidates = append(candidates, p)
					break // 找到一个匹配的扩展名就足够了，进入下一个插件
				}
			}
		}
	}

	// --- 阶段 3: 最终确认 ---
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no suitable plugin found to encrypt file: %s (MIME: '%s', Ext: '%s')", inputPath, mimeType, ext)
	}

	slog.Debug("Found candidates, running ShouldProcess", "path", inputPath, "count", len(candidates))
	for _, p := range candidates {
		if p.ShouldProcess(inputPath) {
			slog.Info("Successfully selected plugin", "path", inputPath, "plugin", fmt.Sprintf("%T", p))
			return p, nil
		}
	}

	// --- 阶段 4: 失败 ---
	return nil, fmt.Errorf("all candidate plugins for file '%s' were rejected by ShouldProcess", inputPath)
}

// FindAllEncryptingPlugins 返回所有能加密指定文件的插件候选（按优先级排序）
// 与 FindEncryptingPlugin 不同，此函数返回所有候选而非单一最佳匹配
// 优先级：P0(精确 MIME/扩展名) > P1(通用 ShouldProcess=true)
func FindAllEncryptingPlugins(inputPath string) []PluginCandidate {
	ext := strings.ToLower(filepath.Ext(inputPath))
	mimeType, _ := utils.DetectFileMIMEType(inputPath)

	var candidates []PluginCandidate

	// --- 阶段 1: MIME 精确匹配 (P0) ---
	if mimeType != "" {
		for _, p := range Plugins {
			for _, prefix := range p.SupportedMimePrefixes() {
				if strings.HasPrefix(mimeType, prefix) {
					if p.ShouldProcess(inputPath) {
						candidates = append(candidates, PluginCandidate{
							Plugin: p, Name: p.Name(), MatchType: "mime", Priority: 0,
						})
					}
					break
				}
			}
		}
	}

	// --- 阶段 2: 扩展名精确匹配 (P0, 仅当阶段1无结果时) ---
	if len(candidates) == 0 {
		extWithoutDot := ext
		if len(extWithoutDot) > 0 {
			extWithoutDot = extWithoutDot[1:]
		}
		if extWithoutDot != "" {
			for _, p := range Plugins {
				for _, supportedExt := range p.SupportedExtensions() {
					if strings.ToLower(supportedExt) == extWithoutDot {
						if p.ShouldProcess(inputPath) {
							candidates = append(candidates, PluginCandidate{
								Plugin: p, Name: p.Name(), MatchType: "extension", Priority: 0,
							})
						}
						break
					}
				}
			}
		}
	}

	// --- 阶段 3: 仅限真正的通用插件 (P1) ---
	// 条件：ShouldProcess=true 且 未声明任何 MIME 前缀 且 未声明任何扩展名
	// 声明了特定类型（MIME/扩展名）的插件如果没在阶段1-2匹配到，
	// 说明这个文件不是它们能处理的类型，不应作为候选返回
	for _, p := range Plugins {
		if !p.ShouldProcess(inputPath) {
			continue
		}
		hasMimePrefixes := len(p.SupportedMimePrefixes()) > 0
		hasExtensions := len(p.SupportedExtensions()) > 0
		if hasMimePrefixes || hasExtensions {
			continue
		}
		alreadyIncluded := false
		for _, c := range candidates {
			if c.Name == p.Name() {
				alreadyIncluded = true
				break
			}
		}
		if !alreadyIncluded {
			candidates = append(candidates, PluginCandidate{
				Plugin: p, Name: p.Name(), MatchType: "general", Priority: 1,
			})
		}
	}

	return candidates
}

// FindDecryptingPlugin 为给定的容器文件查找合适的解密插件
func FindDecryptingPlugin(containerPath string) (Plugin, error) {
	for _, p := range Plugins {
		if p.CanDecrypt(containerPath) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no suitable plugin found to decrypt container: %s", containerPath)
}

func FindDecryptingPluginByContainerType(containerType uint16) (Plugin, error) {
	for _, p := range Plugins {
		if p.ContainerType() == containerType {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no plugin found for container type: %d", containerType)
}

// ProcessFileWithPlugin 是一个通用的辅助函数，它使用插件提供的策略来处理文件。
// ProcessFileWithPlugin 封装了需要元数据提取和内容预处理的插件的文件处理流程。
// 仅当插件声明了 MetadataExtractor 或 ContentPreprocessor 时使用。
func ProcessFileWithPlugin(p Plugin, inputPath string) (types.Index, io.ReadCloser, error) {
	var index types.Index

	extractor := p.GetMetadataExtractor()
	if extractor != nil {
		var err error
		index, err = extractor.ExtractMetadata(inputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("metadata extraction failed for '%s': %w", inputPath, err)
		}
	}

	preprocessor := p.GetContentPreprocessor()
	if preprocessor != nil {
		dataReader, err := preprocessor.Preprocess(inputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("content preprocessing failed for '%s': %w", inputPath, err)
		}
		return index, dataReader, nil
	}

	file, err := os.Open(inputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open file '%s': %w", inputPath, err)
	}
	return index, file, nil
}

func EncryptFileWithPlugin(ctx context.Context, plugin Plugin, inputPath, inputRootDir, outputDir string, collector *performance.Collector) (string, error) {
	var outputPath string
	if vp, ok := plugin.(*video.VideoPlugin); ok {
		vp.SetOutputDir(outputDir)
	}

	defer func() {
		if r := recover(); r != nil {
			slog.Error("EncryptFileWithPlugin panicked", "path", inputPath, "panic", r)
			outputPath = ""
		}
	}()

	if collector != nil {
		collector.StartPhase("analyzing")
	}
	needsPreprocessing := plugin.GetMetadataExtractor() != nil || plugin.GetContentPreprocessor() != nil
	slog.Debug("EncryptFileWithPlugin", "plugin", plugin.Name(), "needsPreprocessing", needsPreprocessing, "path", inputPath)

	var index types.Index
	var dataReader io.ReadCloser

	if needsPreprocessing {
		var err error
		index, dataReader, err = ProcessFileWithPlugin(plugin, inputPath)
		if err != nil {
			return "", fmt.Errorf("plugin failed to process file '%s': %w", inputPath, err)
		}
		defer dataReader.Close()
	}

	if collector != nil {
		collector.EndPhase("analyzing", 0)
		collector.StartPhase("preprocessing")
	}
	if err := plugin.PreEncryptProcessor(index, inputPath, inputRootDir, outputDir); err != nil {
		return "", fmt.Errorf("pre-encryption failed for '%s': %w", inputPath, err)
	}
	slog.Debug("PreEncryptProcessor completed", "path", inputPath)
	if collector != nil {
		collector.EndPhase("preprocessing", 0)
		collector.StartPhase("encrypting")
	}

	var result *crypto.EncryptionResult
	var err error
	if needsPreprocessing {
		result, err = plugin.Encrypt(dataReader)
	} else {
		file, ferr := os.Open(inputPath)
		if ferr != nil {
			return "", fmt.Errorf("failed to open file '%s': %w", inputPath, ferr)
		}
		defer file.Close()
		result, err = plugin.Encrypt(file)
	}
	if err != nil {
		return "", fmt.Errorf("encryption failed for '%s': %w", inputPath, err)
	}
	slog.Debug("Encrypt completed", "path", inputPath)
	if collector != nil {
		collector.EndPhase("encrypting", 0)
		collector.StartPhase("packing")
	}

	if vp, ok := plugin.(*video.VideoPlugin); ok {
		vp.SetPostEncryptVerify(true)
	}
	outputPath, err = plugin.PostEncryptProcessor(result)
	if err != nil {
		return "", fmt.Errorf("post-encryption failed for '%s': %w", inputPath, err)
	}
	slog.Debug("PostEncryptProcessor completed", "path", inputPath, "outputPath", outputPath)
	if collector != nil {
		collector.EndPhase("packing", 0)
	}

	if vp, ok := plugin.(*video.VideoPlugin); ok {
		sourcePath := vp.EncryptedSourcePath()
		if sourcePath != "" && sourcePath != inputPath {
			tempDir := filepath.Dir(sourcePath)
			if strings.Contains(filepath.Base(tempDir), ".encv_tmp") {
				slog.Info("Cleaning up preprocessed temp file", "path", sourcePath)
				if rmErr := os.Remove(sourcePath); rmErr != nil {
					slog.Warn("Failed to clean up preprocessed temp file", "path", sourcePath, "error", rmErr)
				}
			}
		}
	}

	return outputPath, nil
}

// DecryptContainerWithPlugin 是一个新的辅助函数，封装了完整的解密流程
func DecryptContainerWithPlugin(ctx context.Context, plugin Plugin, containerPath, outputDir string, collector *performance.Collector) (string, error) {
	if collector != nil {
		collector.StartPhase("analyzing")
	}
	if err := plugin.PreDecryptProcessor(containerPath, outputDir); err != nil {
		return "", fmt.Errorf("pre-decryption failed for '%s': %w", containerPath, err)
	}
	slog.Debug("PreDecryptProcessor completed", "path", containerPath)
	if collector != nil {
		collector.EndPhase("analyzing", 0)
		collector.StartPhase("decrypting")
	}

	outputPath, err := plugin.Decrypt(containerPath, outputDir)
	if err != nil {
		return "", fmt.Errorf("decryption failed for '%s': %w", containerPath, err)
	}
	slog.Debug("Decrypt completed", "path", containerPath, "outputPath", outputPath)
	if collector != nil {
		collector.EndPhase("decrypting", 0)
		collector.StartPhase("verifying")
	}

	if err := plugin.PostDecryptProcessor(containerPath); err != nil {
		return "", fmt.Errorf("post-decryption failed for '%s': %w", containerPath, err)
	}
	slog.Debug("PostDecryptProcessor completed", "path", containerPath)
	if collector != nil {
		collector.EndPhase("verifying", 0)
	}

	return outputPath, nil
}

// 遍历文件夹自动选择插件加密，这是使用 EncryptFileWithPlugin 而不是 Plugin.EncryptFile 的原因。
func WalkAndEncrypt(ctx context.Context, walkPath string, inputRootDir, outputDir string) error {
	// 1. 遍历目录，收集所有文件并按插件分组
	filesByPlugin := make(map[Plugin][]string)

	err := filepath.WalkDir(walkPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		plugin, err := FindEncryptingPlugin(path)
		if err != nil {
			slog.Debug("Skipping file, no handler found", "path", path, "error", err)
			return nil
		}
		filesByPlugin[plugin] = append(filesByPlugin[plugin], path)
		return nil
	})
	if err != nil {
		return err
	}

	// 2. 对每个插件的文件进行批处理
	for plugin, paths := range filesByPlugin {
		slog.Info("Processing files with plugin", "plugin", fmt.Sprintf("%T", plugin), "count", len(paths))

		// 3. 调用插件的 GroupFiles 方法进行预处理
		processedPaths, err := plugin.GroupFiles(paths, inputRootDir, outputDir)
		if err != nil {
			return fmt.Errorf("plugin '%T' failed to group files: %w", plugin, err)
		}

		// 4. 对处理后的文件列表进行逐个加密
		for _, path := range processedPaths {
			slog.Debug("Processing file with plugin", "path", path, "plugin", fmt.Sprintf("%T", plugin))
			if _, err := EncryptFileWithPlugin(ctx, plugin, path, inputRootDir, outputDir, nil); err != nil {
				slog.Warn("Failed to encrypt file, continuing", "path", path, "plugin", fmt.Sprintf("%T", plugin), "error", err)
			}
		}
	}
	return nil
}
