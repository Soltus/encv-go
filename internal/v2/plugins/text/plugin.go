package text

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/container/detector"
	"github.com/Soltus/encv-go/internal/v2/container/fragment"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/physical"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/Soltus/encv-go/internal/v2/plugins/interfaces/packer"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/service"
	"github.com/Soltus/encv-go/internal/v2/types"
)

type TextPlugin struct {
	ctx              context.Context
	cfg              *config.Config
	settings         TextPluginConfig
	index            TextIndex
	outputDir        string
	inputPath        string
	inputRootDir     string
	baseNamer        namer.BaseNamer           // 注入容器命名器
	containerManager *service.ContainerManager // 注入 ContainerManager
	physicalPacker   physical.PhysicalPacker
}

func (p *TextPlugin) Name() string {
	return "text" // 这个字符串必须与配置文件中的键对应
}

// Plugin 接口实现
func (p *TextPlugin) GetContainerExtension() string {
	return p.settings.Ext
}

type TextPluginConfig struct {
	// 容器扩展名，包含点前缀，默认值为 ".sccgt"
	Ext string `json:"ext"`
}

func (p *TextPlugin) GetSettingsSchemaType() interface{} {
	return TextPluginConfig{}
}

// 2. 实现接口方法，返回默认配置的 JSON
func (p *TextPlugin) GetDefaultSettings() json.RawMessage {
	defaultCfg := TextPluginConfig{
		Ext: ".sccgt",
	}
	data, _ := json.Marshal(defaultCfg) // 忽略错误，因为默认值是硬编码的，不会出错
	return data
}

func (p *TextPlugin) GetSettingFields() []pluginInterfaces.SettingField {
	return []pluginInterfaces.SettingField{
		{
			Key:          "ext",
			Type:         "string",
			DefaultValue: ".sccgt",
			Help:         "The container file extension for encrypted text files (e.g., '.sccgt').",
		},
	}
}

// init 在包被导入时自动执行，完成自注册
func init() {
	types.RegisterKVIProvider(IndexKindText, func(rawKVI json.RawMessage) (types.KVIProvider, error) {
		var kvi TextKVI_v2
		if err := json.Unmarshal(rawKVI, &kvi); err != nil {
			return nil, fmt.Errorf("failed to unmarshal KVI: %w", err)
		}
		return kvi, nil
	})
}

// Plugin 接口实现
func (p *TextPlugin) Initialize(ctx context.Context) error {
	if ctx == p.ctx {
		return nil // 避免重复初始化
	}
	p.ctx = ctx
	p.cfg = config.FromContext(ctx)
	settings, err := config.GetPluginSettingsFor[TextPluginConfig](p.cfg, p.Name())
	if err != nil {
		return fmt.Errorf("could not get settings for plugin %s: %w", p.Name(), err)
	}
	p.settings = *settings // 将指针解引用，存入
	p.containerManager = service.NewContainerManager()
	p.baseNamer = namer.NewDefaultBaseNamer()
	p.physicalPacker = physical.NewSinglePhysicalPacker()
	return nil
}

// Plugin 接口实现
//
//	返回在 Initialize 阶段已经配置好的 chunkNamer
func (p *TextPlugin) GetChunkNamer() namer.ChunkNamer {
	return nil
}

// Plugin 接口实现
func (p *TextPlugin) SupportedMimePrefixes() []string {
	return []string{
		"text/",                  // 匹配 text/plain, text/html 等
		"application/json",       // 精确匹配 JSON
		"application/javascript", // 精确匹配 JS
		"application/x-sh",       // 精确匹配 Shell
		"application/x-yaml",     // 精确匹配 YAML
	}
}

// Plugin 接口实现
func (p *TextPlugin) SupportedExtensions() []string {
	// 当 MIME 类型无法识别时，通过这些扩展名进行兜底匹配
	return []string{
		"txt",
		"htm",
		"html",
		"xml",
		"java",
		"properties",
		"sql",
		"js",
		"md",
		"json",
		"conf",
		"ini",
		"vue",
		"php",
		"py",
		"bat",
		"gitignore",
		"yml",
		"yaml",
		"go",
		"sh",
		"c",
		"cpp",
		"h",
		"hpp",
		"tsx",
		"vtt",
		"srt",
		"ass",
		"rs",
		"lrc",
		"strm",
	}
}

// Plugin 接口实现
func (p *TextPlugin) ShouldProcess(inputPath string) bool {
	// 获取文件扩展名（小写，不带点）
	ext := strings.ToLower(filepath.Ext(inputPath))
	if len(ext) > 0 {
		ext = ext[1:]
	}

	// 这些文件在业务上被视为视频的一部分，而非独立的文本文件
	excludedSubtitles := map[string]struct{}{
		"ass": {},
		"srt": {},
		"vtt": {},
	}

	if _, shouldExclude := excludedSubtitles[ext]; shouldExclude {
		return false
	}

	return true
}

//	Plugin 接口实现
//
// 判断此插件是否能解密给定的容器文件。
// 【关键修复】不再依赖不可靠的文件扩展名，而是通过读取容器元数据来判断其内容类型。
func (p *TextPlugin) CanDecrypt(containerPath string) bool {
	kind, err := detector.DetectIndexKind(containerPath)
	if err != nil {
		// 如果无法判断类型（例如，文件损坏或不是 ENCV 容器），则认为不能解密
		// 这里的日志可以帮助调试
		// log.Printf("DEBUG: [TextPlugin.CanDecrypt] Failed to detect kind for '%s': %v\n", containerPath, err)
		return false
	}
	return kind == IndexKindText
}

// 实现 plugins.Plugin 接口
func (p *TextPlugin) GetMetadataExtractor() pluginInterfaces.MetadataExtractor {
	return &TextMetadataExtractor{}
}

// 实现 plugins.Plugin 接口
func (p *TextPlugin) GetContentPreprocessor() pluginInterfaces.ContentPreprocessor {
	return &TextContentPreprocessor{}
}

// 实现 plugins.Plugin 接口
func (p *TextPlugin) GetContentVirifier() pluginInterfaces.ContentVerifier {
	return nil
}

func (p *TextPlugin) GroupFiles(inputPaths []string, inputRootDir, outputDir string) ([]string, error) {
	return inputPaths, nil
}

func (p *TextPlugin) ContainerType() uint16 {
	return types.ContainerTypeText
}

func (p *TextPlugin) DefaultIsSeekable(inputPath string) bool {
	return false
}

func (p *TextPlugin) DisasterZones(inputPath string) []types.DisasterZone {
	return nil
}

func (p *TextPlugin) SupportedContainerVersions() []int {
	return types.SupportedVersions
}

func (p *TextPlugin) DefaultContainerVersion() int {
	return types.DefaultContainerVersion
}

func (p *TextPlugin) ValidateVersion(version int) error {
	if !types.IsValidVersion(version) {
		return fmt.Errorf("text plugin: unsupported container version: %d", version)
	}
	if types.IsDeprecatedVersion(version) {
		slog.Warn("text plugin: using deprecated container version", "version", version)
	}
	return nil
}

func (p *TextPlugin) GetTaskOptions() pluginInterfaces.TaskOptions {
	return pluginInterfaces.TaskOptions{
		PasswordStrategy:     pluginInterfaces.PasswordGlobal,
		SupportVersionSelect: false,
		ExtraFields: []pluginInterfaces.TaskField{
			{
				Key:          "encrypt_filename",
				Label:        "tasks.encryptFilename",
				Type:         "bool",
				Required:     false,
				DefaultValue: "false",
				Help:         "tasks.encryptFilenameHelp",
				Condition:    "encrypt",
			},
			{
				Key:          "fn_rounds",
				Label:        "tasks.fnRounds",
				Type:         "select",
				Required:     false,
				DefaultValue: "8",
				Help:         "tasks.fnRoundsHelp",
				Options:      []string{"4", "8", "12", "16"},
				OptionLabels: map[string]string{"4": "4", "8": "8 (Recommended)", "12": "12", "16": "16"},
				Condition:    "encrypt",
			},
			{
				Key:          "fn_charset",
				Label:        "tasks.fnCharset",
				Type:         "select",
				Required:     false,
				DefaultValue: "alnum",
				Help:         "tasks.fnCharsetHelp",
				Options:      []string{"alnum", "alnum_symbols", "full"},
				OptionLabels: map[string]string{"alnum": "Alphanumeric", "alnum_symbols": "Alnum + Symbols", "full": "Full (Alnum+Symbols+Hanzi+Emoji)"},
				Condition:    "encrypt",
			},
			{
				Key:          "fn_deconfuse",
				Label:        "tasks.fnDeconfuse",
				Type:         "bool",
				Required:     false,
				DefaultValue: "false",
				Help:         "tasks.fnDeconfuseHelp",
				Condition:    "encrypt",
			},
			{
				Key:          "fn_structured",
				Label:        "tasks.fnStructured",
				Type:         "bool",
				Required:     false,
				DefaultValue: "false",
				Help:         "tasks.fnStructuredHelp",
				Condition:    "encrypt",
			},
		},
	}
}

// --- 加密逻辑 ---

// Plugin 接口实现
// 在加密前处理，并更新 Index
func (p *TextPlugin) PreEncryptProcessor(index types.Index, inputPath, inputRootDir, outputDir string) error {
	vIndex, ok := index.(*TextIndex)
	if !ok {
		return fmt.Errorf("[%s] plugin received a non-%s index", p.Name(), p.Name())
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	p.index = *vIndex
	p.inputPath = inputPath
	p.inputRootDir = inputRootDir
	p.outputDir = outputDir
	return nil
}

// Plugin 接口实现
// 执行核心的加密工作
func (p *TextPlugin) Encrypt(dataReader io.Reader) (*crypto.EncryptionResult, error) {
	guardKey := fmt.Sprintf("%s|%s", p.inputPath, p.outputDir)

	var result *crypto.EncryptionResult
	err := utils.Do(guardKey, func() error {
		// 1. 执行加密
		// crypto.EncryptToTempFile_v2 会读取 dataReader 并生成带有 Salt/IV 头的临时文件
		var err error
		result, err = crypto.EncryptToTempFile_v2(dataReader, p.cfg.Password, p.outputDir)
		if err != nil {
			return fmt.Errorf("failed to encrypt to temp file: %w", err)
		}

		log.Printf("INFO: [%s] Encrypted to temporary file: %s (Payload: %d bytes)\n", p.Name(), result.TempPath, result.EncryptedPayloadSize)
		log.Printf("✅ [%s] Encrypted successfully.\n", p.Name())
		return nil
	})

	return result, err
}

// Plugin 接口实现
// 加密后处理器
func (p *TextPlugin) PostEncryptProcessor(result *crypto.EncryptionResult) (string, error) {
	logicalDataSize := result.EncryptedPayloadSize

	logicalFragments, err := fragment.CreateLogicalFragmentsFromSize(logicalDataSize, logicalDataSize, types.FragmentType_AtomicFile)
	if err != nil {
		return "", fmt.Errorf("failed to create logical fragments from size: %w", err)
	}
	log.Printf("-> [%s] Generated %d logical fragments.\n", p.Name(), len(logicalFragments))

	kvi := TextKVI_v2{
		KVI: types.KVI{
			SaltBase64: crypto.Base64Encode_v2(result.Salt),
			IVBase64:   crypto.Base64Encode_v2(result.IV),
		},
		TextIndex: &p.index,
	}
	manifest, err := types.NewManifest(kvi, logicalFragments)
	if err != nil {
		return "", fmt.Errorf("failed to create manifest: %w", err)
	}

	encryptedBaseName := p.baseNamer.GenerateEncryptedBaseName(p.index.OriginalFilename)
	finalFilename := encryptedBaseName + p.settings.Ext
	finalBaseName := strings.TrimSuffix(finalFilename, p.settings.Ext)

	packParams := &packer.PackParams{
		Manifest:       manifest,
		PhysicalPacker: p.physicalPacker,
		TempEncPath:    result.TempPath,

		Salt:                 result.Salt,
		IV:                   result.IV,
		SaltIVHeaderSize:     result.SaltIVHeaderSize,
		EncryptedPayloadSize: result.EncryptedPayloadSize,

		BaseName:      finalBaseName,
		OutputDir:     p.outputDir,
		Index:         &p.index,
		HeaderVersion: 4,
		ContainerType: p.ContainerType(),
		IsSeekable:    p.DefaultIsSeekable(p.inputPath),
		SpecialIDType: types.IDType_Raw,
		SpecialID:     nil,
		FinalFileName: finalFilename,
		WrappedDEK:    result.WrappedDEK,
	}

	outputPath, err := packer.StandardPostEncrypt(packParams)
	if err != nil {
		os.Remove(result.TempPath)
		return "", fmt.Errorf("packing failed: %w", err)
	}

	os.Remove(result.TempPath)

	log.Printf("✅ [%s] packed successfully.\n", p.Name())
	return outputPath, nil
}

// --- 解密逻辑 ---

// Plugin 接口实现
// 解密前无需额外操作
func (p *TextPlugin) PreDecryptProcessor(containerPath, outputDir string) error {
	return nil
}

// Plugin 接口实现
func (p *TextPlugin) Decrypt(containerPath, outputDir string) (string, error) {
	log.Printf("DEBUG: [%s] Starting decryption for: %s\n", p.Name(), containerPath)
	p.outputDir = outputDir

	readablePath, err := p.containerManager.GetReadablePath(containerPath, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get readable path from container manager: %w", err)
	}
	log.Printf("DEBUG: [%s] Using readable path: %s\n", p.Name(), readablePath)

	factory, err := reader.NewDecryptReaderFactory(readablePath, p.cfg.Password)
	if err != nil {
		return "", fmt.Errorf("failed to create reader factory for '%s': %w", readablePath, err)
	}
	defer factory.Close()
	log.Printf("DEBUG: [%s] Reader factory created successfully.\n", p.Name())

	decryptedReader, err := factory.NewDecryptReader()
	if err != nil {
		return "", fmt.Errorf("[%s] failed to create decrypt reader: %w", p.Name(), err)
	}
	defer decryptedReader.Close()
	_, isSeekable := decryptedReader.(io.Seeker)
	if isSeekable {
		log.Printf("INFO: [%s] Container is SEEKABLE. Decrypting full content.\n", p.Name())
	} else {
		log.Printf("INFO: [%s] Container is ATOMIC. Decrypting full content.\n", p.Name())
	}

	index := factory.GetIndex()
	vIndex, ok := index.(*TextIndex)
	if !ok {
		return "", fmt.Errorf("container is not a %s container", p.Name())
	}

	outputPath := filepath.Join(outputDir, vIndex.GetOriginalFilename())
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	if _, err := io.Copy(outputFile, decryptedReader); err != nil {
		return "", fmt.Errorf("failed to write decrypted %s stream: %w", p.Name(), err)
	}

	p.index = *vIndex

	log.Printf("✅ [%s] Decrypted to: %s\n", p.Name(), outputPath)
	return outputPath, nil
}

// Plugin 接口实现
// 在解密后处理
func (p *TextPlugin) PostDecryptProcessor(containerPath string) error {

	return nil
}
