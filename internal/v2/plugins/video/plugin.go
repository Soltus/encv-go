package video

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
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

var lastVerifyWarnings []*pluginInterfaces.VerifyWarning

func LastVerifyWarnings() []*pluginInterfaces.VerifyWarning {
	return lastVerifyWarnings
}

type VideoPlugin struct {
	ctx          context.Context
	cfg          *config.Config
	settings     VideoPluginConfig
	index        VideoIndex
	outputDir    string
	inputPath    string
	inputRootDir string
	// 记录实际用于加密的源文件路径
	// 如果 Remuxing 发生（如 MKV -> MP4），这里是临时文件路径；
	// 否则，它与 inputPath 一致。
	encryptedSourcePath string
	baseNamer           namer.BaseNamer           // 注入容器命名器
	chunkNamer          namer.ChunkNamer          // 注入分片命名器
	containerManager    *service.ContainerManager // 注入 ContainerManager
	physicalPacker      physical.PhysicalPacker
	trackExtensionsList []string
	// 【新增】存储分片集信息，用于不合并直接加密模式
	splitSets [][]string
	// 【新增】存储分片文件路径集合，用于快速查找
	splitPartPaths      map[string]bool
	isPostEncryptVerify bool
}

func (p *VideoPlugin) Name() string {
	return "video" // 这个字符串必须与配置文件中的键对应
}

func (p *VideoPlugin) SetPostEncryptVerify(v bool) {
	p.isPostEncryptVerify = v
}

// Plugin 接口实现
func (p *VideoPlugin) GetContainerExtension() string {
	return p.settings.Ext
}

type VideoPluginConfig struct {
	Ext                            string `json:"ext"`
	ContainerChunkSizeMB           int    `json:"container_chunk_size_mb"`
	LightContainerMainChunkEnabled bool   `json:"light_container_main_chunk_enabled"`
	TrackExtensions                string `json:"track_extensions"`
	KeepMkvForMkvSource            bool   `json:"keep_mkv_for_mkv_source"`
	VerifyAfterPack                bool   `json:"verify_after_pack"`
	PluginCacheDir                 string `json:"plugin_cache_dir"`
	SkipMergeForSplitMKV           bool   `json:"skip_merge_for_split_mkv"`
	AllowNoReencode                bool   `json:"allow_no_reencode"`
	DefaultStreamPreset            string `json:"default_stream_preset"`
}

func (p *VideoPlugin) GetSettingsSchemaType() interface{} {
	return VideoPluginConfig{}
}

// 2. 实现接口方法，返回默认配置的 JSON
func (p *VideoPlugin) GetDefaultSettings() json.RawMessage {
	defaultCfg := VideoPluginConfig{
		Ext:                            ".sccgv",
		ContainerChunkSizeMB:           0,
		LightContainerMainChunkEnabled: false,
		TrackExtensions:                ".ass,.srt,.dm.ass",
		KeepMkvForMkvSource:            true,
		VerifyAfterPack:                false,
		AllowNoReencode:                false,
		DefaultStreamPreset:            "balanced",
	}
	data, _ := json.Marshal(defaultCfg) // 忽略错误，因为默认值是硬编码的，不会出错
	return data
}

func (p *VideoPlugin) GetSettingFields() []pluginInterfaces.SettingField {
	return []pluginInterfaces.SettingField{
		{
			Key:          "ext",
			Type:         "string",
			DefaultValue: ".sccgv",
			Help:         "The container file extension for encrypted video files (e.g., '.sccgv').",
		},
		{
			Key:          "container_chunk_size_mb",
			Type:         "number",
			DefaultValue: 0,
			Help:         "The chunk size in MB. Set to 0 to disable physical chunking. Minimum value is 30 if enabled.",
		},
		{
			Key:          "light_container_main_chunk_enabled",
			Type:         "bool",
			DefaultValue: false,
			Help:         "If enabled, the main chunk will only contain the manifest, not the source data.",
		},
		{
			Key:          "track_extensions",
			Type:         "text", // 使用 text 类型，让用户输入逗号分隔的字符串
			DefaultValue: ".ass,.srt,.dm.ass",
			Help:         "Subtitle/track file extensions, separated by commas (e.g., .ass,.srt,.dm.ass). These files are not packed into the container.",
		},
		{
			Key:          "keep_mkv_for_mkv_source",
			Type:         "bool",
			DefaultValue: true,
			Help:         "If the source is an MKV file, keep its MKV container (remuxed with mkvmerge). If disabled, all sources will be converted to fast-start MP4.",
		},
		{
			Key:          "verify_after_pack",
			Type:         "bool",
			DefaultValue: false,
			Help:         "If enabled, verifies the container integrity by decrypting it and checking MD5 after packing. This takes extra time.",
		},
		{
			Key:          "plugin_cache_dir",
			Type:         "string",
			DefaultValue: "",
			Help:         "Video plugin cache directory for storing merged MKV files and encryption temp files. Leave empty to use output directory. Setting this can cache intermediate products to avoid reprocessing.",
		},
		{
			Key:          "skip_merge_for_split_mkv",
			Type:         "bool",
			DefaultValue: false,
			Help:         "If enabled, split MKV files will NOT be merged before encryption. Instead, they will be encrypted sequentially as multiple streams. This is faster but may have compatibility issues with some players.",
		},
		{
			Key:          "allow_no_reencode",
			Type:         "bool",
			DefaultValue: false,
			Help:         "Whether to allow encrypting video without re-encoding.",
		},
		{
			Key:          "default_stream_preset",
			Type:         "string",
			DefaultValue: "balanced",
			Help:         "Default stream preset name (e.g., 'balanced').",
		},
	}
}

// init 在包被导入时自动执行，完成自注册
func init() {
	types.RegisterKVIProvider(IndexKindVideo, func(rawKVI json.RawMessage) (types.KVIProvider, error) {
		var kvi VideoKVI_v2
		if err := json.Unmarshal(rawKVI, &kvi); err != nil {
			return nil, fmt.Errorf("failed to unmarshal KVI: %w", err)
		}
		return kvi, nil
	})
}

// Plugin 接口实现
func (p *VideoPlugin) Initialize(ctx context.Context) error {
	if ctx == p.ctx {
		return nil // 避免重复初始化
	}
	p.ctx = ctx
	p.cfg = config.FromContext(ctx)
	// 2. 【关键】使用泛型辅助函数，安全地获取并解析本插件的配置
	settings, err := config.GetPluginSettingsFor[VideoPluginConfig](p.cfg, p.Name())
	if err != nil {
		return fmt.Errorf("could not get settings for plugin %s: %w", p.Name(), err)
	}
	p.settings = *settings

	var rawSettings json.RawMessage
	if p.cfg.Provider != nil {
		rawSettings, _ = p.cfg.Provider.GetPluginSettings(p.Name())
	} else {
		rawSettings = p.cfg.PluginSettings[p.Name()]
	}
	if len(rawSettings) > 0 {
		var raw map[string]interface{}
		if json.Unmarshal(rawSettings, &raw) == nil {
			if p.settings.ContainerChunkSizeMB == 0 {
				if v, ok := raw["chunk_size_mb"]; ok {
					if f, ok := v.(float64); ok && f > 0 {
						p.settings.ContainerChunkSizeMB = int(f)
					}
				}
			}
			if !p.settings.LightContainerMainChunkEnabled {
				if v, ok := raw["light_main_chunk_enabled"]; ok {
					if b, ok := v.(bool); ok && b {
						p.settings.LightContainerMainChunkEnabled = true
					}
				}
			}
		}
	}
	if p.settings.ContainerChunkSizeMB > 0 && p.settings.ContainerChunkSizeMB < 30 {
		p.settings.ContainerChunkSizeMB = 30 // 强制修改为 30
	}
	// 处理逗号分隔的字符串，并存储到 trackExtensionsList
	if p.settings.TrackExtensions != "" {
		parts := strings.Split(p.settings.TrackExtensions, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				p.trackExtensionsList = append(p.trackExtensionsList, trimmed)
			}
		}
	}
	p.containerManager = service.NewContainerManager()
	p.baseNamer = namer.NewDefaultBaseNamer()
	p.chunkNamer = namer.NewPaddedNamer(p.settings.Ext, p.baseNamer, 4) // 补零到4位
	// 初始化新增字段
	p.splitSets = make([][]string, 0)
	p.splitPartPaths = make(map[string]bool)
	if p.settings.ContainerChunkSizeMB > 0 {
		slog.Info("Physical chunking enabled", "plugin", p.Name(), "size_mb", p.settings.ContainerChunkSizeMB)
		p.physicalPacker = physical.NewFileChunkerPhysicalPacker(int64(p.settings.ContainerChunkSizeMB)*1024*1024, p.chunkNamer)
	} else {
		slog.Info("Physical chunking disabled", "plugin", p.Name())
		p.physicalPacker = physical.NewSinglePhysicalPacker()
	}
	return nil
}

// Plugin 接口实现
//
//	返回在 Initialize 阶段已经配置好的 chunkNamer
func (p *VideoPlugin) GetChunkNamer() namer.ChunkNamer {
	return p.chunkNamer
}

// Plugin 接口实现
func (p *VideoPlugin) SupportedMimePrefixes() []string {
	return []string{"video/", "application/vnd.rn-realmedia-vbr", "application/vnd.apple.mpegurl"}
}

// Plugin 接口实现
func (p *VideoPlugin) SupportedExtensions() []string {
	// 当 MIME 类型无法识别时，通过这些扩展名进行兜底匹配
	return []string{
		"mp4",
		"mkv",
		"avi",
		"mov",
		"rmvb",
		"webm",
		"flv",
		"m3u8",
	}
}

// Plugin 接口实现
func (p *VideoPlugin) ShouldProcess(inputPath string) bool {
	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext == ".srt" || ext == ".ass" || ext == ".vtt" {
		return false
	}
	return true
}

//	Plugin 接口实现
//
// 判断此插件是否能解密给定的容器文件。
// 【关键修复】不再依赖不可靠的文件扩展名，而是通过读取容器元数据来判断其内容类型。
func (p *VideoPlugin) CanDecrypt(containerPath string) bool {
	kind, err := detector.DetectIndexKind(containerPath)
	if err != nil {
		// 如果无法判断类型（例如，文件损坏或不是 ENCV 容器），则认为不能解密
		// 这里的日志可以帮助调试
		// log.Printf("DEBUG: [VideoPlugin.CanDecrypt] Failed to detect kind for '%s': %v\n", containerPath, err)
		return false
	}
	return kind == IndexKindVideo
}

// 实现 plugins.Plugin 接口
func (p *VideoPlugin) GetMetadataExtractor() pluginInterfaces.MetadataExtractor {
	return &VideoMetadataExtractor{
		settings: p.settings,
		index:    &p.index,
	}
}

// 实现 plugins.Plugin 接口
func (p *VideoPlugin) GetContentPreprocessor() pluginInterfaces.ContentPreprocessor {
	return &VideoContentPreprocessor{
		settings:       p.settings,
		index:          &p.index,
		outputDir:      p.outputDir,
		splitPartPaths: p.splitPartPaths,
		ctx:            p.ctx,
	}
}

// 实现 plugins.Plugin 接口
func (p *VideoPlugin) GetContentVirifier() pluginInterfaces.ContentVerifier {
	return &VideoContentVerifier{}
}

// 实现 plugins.Plugin 接口
func (p *VideoPlugin) GetPhysicalPacker() physical.PhysicalPacker {
	return p.physicalPacker
}

func (p *VideoPlugin) BuildFragments(logicalFileSize int64) ([]types.Fragment, error) {
	return p.createGOPAlignedFragments(logicalFileSize)
}

func (p *VideoPlugin) GroupFiles(inputPaths []string, inputRootDir, outputDir string) ([]string, error) {

	var mkvPaths []string
	for _, path := range inputPaths {
		if strings.ToLower(filepath.Ext(path)) == ".mkv" {
			mkvPaths = append(mkvPaths, path)
		}
	}
	if len(mkvPaths) == 0 {
		return inputPaths, nil
	}

	fmt.Println("-> [VIDEO_PLUGIN] Pre-scanning all MKV files in the directory for split parts...")
	mkvInfos, err := batchGetMkvInfos(mkvPaths)
	if err != nil {
		return nil, fmt.Errorf("failed to batch scan MKV files: %w", err)
	}

	splitSets := groupSplitParts(mkvInfos)

	p.splitSets = splitSets
	p.splitPartPaths = make(map[string]bool)
	for _, set := range splitSets {
		for _, path := range set {
			p.splitPartPaths[path] = true
		}
	}

	if p.settings.SkipMergeForSplitMKV && len(splitSets) > 0 {
		slog.Info("SkipMergeForSplitMKV enabled, processing split sets without merging", "component", "VIDEO_PLUGIN", "split_sets", len(splitSets))
		return inputPaths, nil
	}

	finalPaths := make([]string, 0)

	for _, path := range inputPaths {
		if strings.ToLower(filepath.Ext(path)) != ".mkv" {
			finalPaths = append(finalPaths, path)
		}
	}

	cacheDir := p.settings.PluginCacheDir
	if cacheDir == "" {
		cacheDir = outputDir
		slog.Info("PluginCacheDir not set, using outputDir as cache", "component", "VIDEO_PLUGIN", "cache_dir", cacheDir)
	} else {
		slog.Info("Using configured PluginCacheDir", "component", "VIDEO_PLUGIN", "cache_dir", cacheDir)
	}
	for _, set := range splitSets {
		slog.Info("Found a split set, merging", "component", "VIDEO_PLUGIN", "parts", len(set))
		mergedPath, err := mergeSplitPartsFromSet(set, outputDir, cacheDir)
		if err != nil {
			return nil, fmt.Errorf("failed to merge a split set: %w", err)
		}
		finalPaths = append(finalPaths, mergedPath)
	}

	for _, info := range mkvInfos {
		if !info.IsSplitPart {
			finalPaths = append(finalPaths, info.Path)
		}
	}

	return finalPaths, nil
}

func (p *VideoPlugin) ContainerType() uint16 {
	return types.ContainerTypeVideo
}

func (p *VideoPlugin) DefaultIsSeekable(inputPath string) bool {
	ext := strings.ToLower(filepath.Ext(inputPath))
	return ext == ".mp4" || ext == ".mkv"
}

func (p *VideoPlugin) DisasterZones(inputPath string) []types.DisasterZone {
	return []types.DisasterZone{
		{Name: "video_header", Offset: 0, Size: 4096},
	}
}

func (p *VideoPlugin) SupportedContainerVersions() []int {
	return types.SupportedVersions
}

func (p *VideoPlugin) DefaultContainerVersion() int {
	return types.DefaultContainerVersion
}

func (p *VideoPlugin) ValidateVersion(version int) error {
	if !types.IsValidVersion(version) {
		return fmt.Errorf("video plugin: unsupported container version: %d", version)
	}
	if types.IsDeprecatedVersion(version) {
		slog.Warn("video plugin: using deprecated container version", "version", version)
	}
	return nil
}

func (p *VideoPlugin) GetTaskOptions() pluginInterfaces.TaskOptions {
	return pluginInterfaces.TaskOptions{
		PasswordStrategy:     pluginInterfaces.PasswordGlobal,
		SupportVersionSelect: true,
		SupportedVersions:    p.SupportedContainerVersions(),
		DefaultVersion:       p.DefaultContainerVersion(),
		ExtraFields: []pluginInterfaces.TaskField{
			{
				Key:          "stream_preset",
				Label:        "tasks.streamPreset",
				Type:         "select",
				Required:     false,
				DefaultValue: "balanced",
				Help:         "tasks.streamPresetHelp",
				Options:      []string{"balanced", "quality", "high_quality"},
				OptionLabels: map[string]string{"balanced": "Balanced", "quality": "Quality", "high_quality": "High Quality"},
				Condition:    "encrypt",
			},
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
func (p *VideoPlugin) PreEncryptProcessor(index types.Index, inputPath, inputRootDir, outputDir string) error {
	vIndex, ok := index.(*VideoIndex)
	if !ok {
		return fmt.Errorf("%s plugin received a non-%s index", p.Name(), p.Name())
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	p.index = *vIndex
	p.inputPath = inputPath
	p.inputRootDir = inputRootDir
	p.outputDir = outputDir

	// 【关键修复】如果处理的是合并后的缓存文件，使用第一个分片的原始文件名
	originalFilename := p.index.OriginalFilename
	if p.settings.PluginCacheDir != "" && len(p.splitSets) > 0 {
		cacheDir := filepath.Clean(p.settings.PluginCacheDir)
		inputDir := filepath.Clean(filepath.Dir(inputPath))
		if cacheDir == inputDir {
			// 这是合并后的文件，使用第一个分片的文件名
			firstPartPath := p.splitSets[0][0]
			originalFilename = filepath.Base(firstPartPath)
			slog.Info("Using original filename from first split part", "component", "VIDEO_PLUGIN", "filename", originalFilename)
			// 更新 index 中的原始文件名
			p.index.OriginalFilename = originalFilename
		}
	}

	encryptedBaseName := p.baseNamer.GenerateEncryptedBaseName(originalFilename)

	// 调用字幕处理逻辑，它会修改 p.index.SubtitleTrack
	return p.HandleSubtitlesForEncryption(p.cfg, &p.index, outputDir, encryptedBaseName)
}

// Plugin 接口实现
// 执行核心的加密工作，并调用 Packer
func (p *VideoPlugin) Encrypt(dataReader io.Reader) (*crypto.EncryptionResult, error) {
	p.isPostEncryptVerify = false
	guardKey := fmt.Sprintf("%s|%s", p.inputPath, p.outputDir)

	var result *crypto.EncryptionResult
	err := utils.Do(guardKey, func() error {
		// 【关键修复】尝试捕获实际的源文件路径
		// 框架传递的 dataReader 可能是 os.Open(original)，
		// 也可能是 Preprocess 生成的 os.Open(temp_remuxed) 或 TempFileReadCloser。
		// 通过类型断言，我们可以获取底层文件句柄，从而获取真实路径。
		if file, ok := dataReader.(*os.File); ok {
			p.encryptedSourcePath = file.Name()
		} else if tempFile, ok := dataReader.(*reader.TempFileReadCloser); ok {
			p.encryptedSourcePath = tempFile.Name()
		} else {
			// 如果不是文件（如内存 Reader），回退到 inputPath
			p.encryptedSourcePath = p.inputPath
		}

		// 1. 执行加密
		// crypto.EncryptToTempFile 会读取 dataReader
		// 如果 dataReader 指向 Remux 后的文件，加密的也就是 Remux 后的内容
		var err error
		// 【修改】使用配置的缓存目录作为加密临时文件目录，未配置时使用 outputDir
		encryptTempDir := p.settings.PluginCacheDir
		if encryptTempDir == "" {
			encryptTempDir = p.outputDir
			slog.Info("PluginCacheDir not set, using outputDir for encryption temp files", "component", "VIDEO_PLUGIN")
		} else {
			// 确保缓存目录存在
			if mkdirErr := os.MkdirAll(encryptTempDir, 0755); mkdirErr != nil {
				slog.Warn("Failed to create cache dir, falling back to outputDir", "component", "VIDEO_PLUGIN", "error", mkdirErr)
				encryptTempDir = p.outputDir
			} else {
				slog.Info("Using PluginCacheDir for encryption temp files", "component", "VIDEO_PLUGIN", "dir", encryptTempDir)
			}
		}
		result, err = crypto.EncryptToTempFile_v2(dataReader, p.cfg.Password, encryptTempDir)
		if err != nil {
			return fmt.Errorf("failed to encrypt to temp file: %w", err)
		}

		slog.Info("Encrypted to temporary file", "plugin", p.Name(), "temp_path", result.TempPath)
		slog.Info("Actual encrypted source path", "plugin", p.Name(), "source_path", p.encryptedSourcePath)
		slog.Info("Encrypted successfully", "plugin", p.Name())
		return nil
	})

	return result, err
}

// Plugin 接口实现
// 加密后处理器
func (p *VideoPlugin) PostEncryptProcessor(result *crypto.EncryptionResult) (string, error) {
	logicalDataSize := result.EncryptedPayloadSize

	var logicalFragments []types.Fragment
	var err error

	logicalFragments, err = p.createGOPAlignedFragments(logicalDataSize)
	if err != nil {
		slog.Warn("Failed to align fragments to GOP, falling back to size-based fragments", "error", err)
		baseSize := fragment.CalculateFragmentSize(logicalDataSize, int64(p.settings.ContainerChunkSizeMB)*1024*1024)
		logicalFragments, err = fragment.CreateLogicalFragmentsFromSize(logicalDataSize, baseSize, types.FragmentType_SeekableStream)
		if err != nil {
			return "", fmt.Errorf("fallback fragment creation failed: %w", err)
		}
	}

	if stat, err := os.Stat(p.inputPath); err == nil {
		p.index.OriginalFileSize = stat.Size()
	}

	kvi := VideoKVI_v2{
		KVI: types.KVI{
			SaltBase64: crypto.Base64Encode_v2(result.Salt),
			IVBase64:   crypto.Base64Encode_v2(result.IV),
		},
		VideoIndex: &p.index,
	}
	manifest, err := types.NewManifest(kvi, logicalFragments)
	if err != nil {
		return "", fmt.Errorf("failed to create manifest: %w", err)
	}

	finalFilename := p.chunkNamer.GenerateMainChunkName(p.baseNamer.GenerateEncryptedBaseName(p.index.OriginalFilename))
	finalBaseName := strings.TrimSuffix(finalFilename, p.settings.Ext)

	startIdx := 1
	if !p.settings.LightContainerMainChunkEnabled {
		startIdx = 0
	}

	packParams := &packer.PackParams{
		Manifest:       manifest,
		PhysicalPacker: p.physicalPacker,
		TempEncPath:    result.TempPath,

		Salt:                 result.Salt,
		IV:                   result.IV,
		SaltIVHeaderSize:     result.SaltIVHeaderSize,
		EncryptedPayloadSize: result.EncryptedPayloadSize,

		BaseName:              finalBaseName,
		OutputDir:             p.outputDir,
		Index:                 &p.index,
		Namer:                 p.chunkNamer,
		StartIdx:              startIdx,
		LightMainChunkEnabled: p.settings.LightContainerMainChunkEnabled,
		HeaderVersion:         4,
		ContainerType:         p.ContainerType(),
		IsSeekable:            p.DefaultIsSeekable(p.inputPath),
		SpecialIDType:         types.IDType_Raw,
		SpecialID:             nil,
	}

	if p.settings.ContainerChunkSizeMB == 0 {
		packParams.FinalFileName = finalFilename
	}

	passwordHint, err := crypto.CalculatePasswordHint(p.cfg.Password, result.Salt)
	if err != nil {
		slog.Warn("Failed to calculate password hint, using empty hint", "error", err)
	}
	packParams.PasswordHint = passwordHint
	packParams.WrappedDEK = result.WrappedDEK

	outputPath, err := packer.StandardPostEncrypt(packParams)
	if err != nil {
		os.Remove(result.TempPath)
		return "", fmt.Errorf("packing failed: %w", err)
	}

	os.Remove(result.TempPath)

	if p.settings.VerifyAfterPack {
		if verifyErr := p.verifyContainer(); verifyErr != nil {
			return "", verifyErr
		}
	}

	slog.Info("Packed successfully", "plugin", p.Name())
	return outputPath, nil
}

// createGOPAlignedFragments 基于关键帧位置生成逻辑分片
// 【通用性】适用于所有视频，依赖 p.index.KeyFrameOffsets
func (p *VideoPlugin) createGOPAlignedFragments(fileSize int64) ([]types.Fragment, error) {
	// 如果没有提取到关键帧（例如 ffprobe 失败），则无法进行 GOP 对齐
	if len(p.index.KeyFrameOffsets) == 0 {
		return nil, fmt.Errorf("no keyframe offsets available for GOP alignment")
	}

	// 计算目标逻辑分片大小（例如 4MB）
	targetLogicalSize := fragment.CalculateFragmentSize(fileSize, int64(p.settings.ContainerChunkSizeMB)*1024*1024)
	minLogicalSize := int64(2 * 1024 * 1024) // 2MB

	var fragments []types.Fragment
	currentOffset := uint64(0)
	fragIndex := 0

	for currentOffset < uint64(fileSize) {
		targetEndOffset := currentOffset + uint64(targetLogicalSize)
		actualEndOffset := targetEndOffset

		// 1. 在 KeyFrameOffsets 中查找第一个 >= targetEndOffset 的关键帧
		k := sort.Search(len(p.index.KeyFrameOffsets), func(i int) bool {
			return p.index.KeyFrameOffsets[i] >= targetEndOffset
		})

		// 2. 尝试使用找到的关键帧（或其前一个关键帧）作为分片结束点
		if k < len(p.index.KeyFrameOffsets) {
			candidateKF := p.index.KeyFrameOffsets[k]
			// 检查这个关键帧是否在合理范围内
			// 策略：优先向后对齐到 I 帧
			if candidateKF > currentOffset {
				// 确保 resulting fragment >= minLogicalSize
				if int64(candidateKF-currentOffset) >= minLogicalSize {
					actualEndOffset = candidateKF
				} else if k+1 < len(p.index.KeyFrameOffsets) {
					// 如果使用这个 KF 太小，尝试用下一个 KF
					nextKF := p.index.KeyFrameOffsets[k+1]
					if int64(nextKF-currentOffset) < int64(targetLogicalSize)*2 { // 不超过目标的两倍
						actualEndOffset = nextKF
					}
				}
			}
		}

		// 3. 边界保护：不能超过文件末尾
		if actualEndOffset > uint64(fileSize) {
			actualEndOffset = uint64(fileSize)
		}

		// 4. 创建 Fragment
		frag := types.Fragment{
			ID:                fmt.Sprintf("logical_fragment_%d", fragIndex),
			Type:              types.FragmentType_SeekableStream,
			GlobalStartOffset: currentOffset,
			Length:            actualEndOffset - currentOffset,
		}
		fragments = append(fragments, frag)

		// 记录详细信息用于调试
		// log.Printf("DEBUG: [GOP Frag] ID=%d, Start=%d, End=%d, Length=%d", fragIndex, currentOffset, actualEndOffset, frag.Length)

		currentOffset = actualEndOffset
		fragIndex++
	}

	// 对比计算出的总大小与文件大小
	var totalCalculatedLen uint64 = 0
	for _, f := range fragments {
		totalCalculatedLen += f.Length
	}
	slog.Info("GOP fragment calculation", "file_size", fileSize, "calculated_total_len", totalCalculatedLen, "diff", int64(fileSize)-int64(totalCalculatedLen))

	// 【修复】如果计算出的总大小与文件大小不一致，说明 GOP 对齐导致最后一块丢失，这里强制截断最后一个 Fragment
	if totalCalculatedLen > uint64(fileSize) {
		missingBytes := totalCalculatedLen - uint64(fileSize)
		slog.Warn("Calculated total length exceeds file size, trimming last fragment", "missing_bytes", missingBytes)
		lastFrag := &fragments[len(fragments)-1]
		lastFrag.Length = lastFrag.Length - uint64(missingBytes)
		totalCalculatedLen -= uint64(missingBytes)
	}

	if totalCalculatedLen != uint64(fileSize) {
		return nil, fmt.Errorf("GOP Fragments total length (%d) still mismatches file size (%d)", totalCalculatedLen, fileSize)
	}

	return fragments, nil
}

// verifyContainer 将验证逻辑提取为单独方法，保持 PostEncryptProcessor 简洁
func (p *VideoPlugin) verifyContainer() error {
	finalFilename := p.chunkNamer.GenerateMainChunkName(p.baseNamer.GenerateEncryptedBaseName(p.index.OriginalFilename))
	mainChunkFullPath := filepath.Join(p.outputDir, finalFilename)

	verifyTempDir := filepath.Join(p.outputDir, ".encv_verify_"+utils.RandomString(8))
	if err := os.MkdirAll(verifyTempDir, 0755); err != nil {
		return fmt.Errorf("failed to create verification temp dir: %w", err)
	}

	if _, err := p.Decrypt(mainChunkFullPath, verifyTempDir); err != nil {
		os.RemoveAll(verifyTempDir)
		return fmt.Errorf("verification failed: decryption error occurred: %w", err)
	}

	decrypedFilePath := filepath.Join(verifyTempDir, p.index.OriginalFilename)
	verifier := p.GetContentVirifier().(*VideoContentVerifier)

	sourcePath := p.encryptedSourcePath
	if sourcePath == "" {
		sourcePath = p.inputPath
	}

	slog.Info("verifyContainer path resolution",
		"encrypted_source_path", p.encryptedSourcePath,
		"resolved_source_path", sourcePath,
		"input_path", p.inputPath,
		"is_preprocessed", sourcePath != p.inputPath)

	var verifyOpts *pluginInterfaces.VerifyOptions
	if sourcePath != p.inputPath || p.isPostEncryptVerify {
		slog.Info("Detected preprocessed/re-encoded source or post-encrypt verification, using lenient verification",
			"source_path", sourcePath, "original_input", p.inputPath)
		verifyOpts = &pluginInterfaces.VerifyOptions{SkipSizeCheck: true, SkipStructCheck: true, SkipDeepCheck: true, CollectWarnings: true}
	} else {
		verifyOpts = &pluginInterfaces.VerifyOptions{CollectWarnings: true}
	}

	err, warnings := verifier.Verify(sourcePath, decrypedFilePath, verifyOpts)
	lastVerifyWarnings = warnings
	if err != nil {
		os.RemoveAll(verifyTempDir)
		return fmt.Errorf("container verification failed: %w", err)
	}

	if len(warnings) > 0 {
		slog.Warn("Verification completed with warnings",
			"warnings_count", len(warnings),
			"warnings", warnings)
	}

	os.RemoveAll(verifyTempDir)
	slog.Info("Container verified successfully", "plugin", p.Name())
	return nil
}

// --- 解密逻辑 ---

// SetOutputDir 设置输出目录（供 EncryptFileWithPlugin 在预处理前调用）
func (p *VideoPlugin) SetOutputDir(dir string) {
	p.outputDir = dir
}

// EncryptedSourcePath 返回实际被加密的源文件路径
// 可能是原始 inputPath，也可能是预处理后的临时文件路径（remux/transcode 场景）
func (p *VideoPlugin) EncryptedSourcePath() string {
	return p.encryptedSourcePath
}

// Plugin 接口实现
func (p *VideoPlugin) PreDecryptProcessor(containerPath, outputDir string) error {
	p.outputDir = outputDir
	return nil
}

// Plugin 接口实现
func (p *VideoPlugin) Decrypt(containerPath, outputDir string) (string, error) {
	slog.Info("Starting decryption", "plugin", p.Name(), "container_path", containerPath)
	p.outputDir = outputDir

	readablePath, err := p.containerManager.GetReadablePath(containerPath, p.chunkNamer)
	if err != nil {
		return "", fmt.Errorf("failed to get readable path from container manager: %w", err)
	}
	slog.Info("Using readable path", "plugin", p.Name(), "readable_path", readablePath)

	factory, err := reader.NewDecryptReaderFactory(readablePath, p.cfg.Password)
	if err != nil {
		return "", fmt.Errorf("failed to create reader factory for '%s': %w", readablePath, err)
	}
	defer factory.Close()
	slog.Info("Reader factory created successfully", "plugin", p.Name())

	decryptedReader, err := factory.NewDecryptReader()
	if err != nil {
		return "", fmt.Errorf("[%s] failed to create decrypt reader: %w", p.Name(), err)
	}
	defer decryptedReader.Close()

	index := factory.GetIndex()
	vIndex, ok := index.(*VideoIndex)
	if !ok {
		return "", fmt.Errorf("container is not a video container")
	}

	outputPath := filepath.Join(outputDir, vIndex.GetOriginalFilename())
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	if _, err := io.Copy(outputFile, decryptedReader); err != nil {
		return "", fmt.Errorf("failed to write decrypted video stream: %w", err)
	}

	p.index = *vIndex

	slog.Info("Decrypted successfully", "plugin", p.Name(), "output_path", outputPath)
	return outputPath, nil
}

// Plugin 接口实现
// 在解密后处理
func (p *VideoPlugin) PostDecryptProcessor(containerPath string) error {

	containerDir := filepath.Dir(containerPath)
	// 注意：这里的 vIndex.SubtitleTrack 应该在 Decrypt 方法中被正确设置
	if err := RestoreSubtitlesForDecryption(&p.index, containerDir, p.outputDir); err != nil {
		return fmt.Errorf("failed to restore subtitles: %w", err)
	}

	return nil
}
