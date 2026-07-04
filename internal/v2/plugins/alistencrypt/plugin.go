package alistencrypt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/namer"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/Soltus/encv-go/internal/v2/types"
)

type AlistEncryptPlugin struct {
	ctx             context.Context
	cfg             *config.Config
	settings        AlistEncryptPluginConfig
	outputDir       string
	inputPath       string
	taskExtraFields map[string]string
}

func (p *AlistEncryptPlugin) Name() string {
	return "alist_encrypt"
}

func (p *AlistEncryptPlugin) GetDefaultSettings() json.RawMessage {
	defaultCfg := AlistEncryptPluginConfig{
		Suffix:          ".bin",
		DefaultPassword: "",
		EncType:         "aesctr",
	}
	data, _ := json.Marshal(defaultCfg)
	return data
}

func (p *AlistEncryptPlugin) GetSettingsSchemaType() interface{} {
	return AlistEncryptPluginConfig{}
}

func (p *AlistEncryptPlugin) GetContainerExtension() string {
	return p.settings.Suffix
}

func (p *AlistEncryptPlugin) GetSettingFields() []pluginInterfaces.SettingField {
	return []pluginInterfaces.SettingField{
		{
			Key:          "suffix",
			Type:         "string",
			DefaultValue: ".bin",
			Help:         "Encrypted file suffix (e.g., '.bin'). Must be unique across all plugins.",
		},
		{
			Key:          "default_password",
			Type:         "string",
			DefaultValue: "",
			Help:         "Default password for encryption/decryption (optional, can be overridden per-operation).",
		},
		{
			Key:          "enc_type",
			Type:         "string",
			DefaultValue: "aesctr",
			Help:         "Encryption algorithm type. Currently only 'aesctr' is built-in.",
			Options:      []string{"aesctr"},
		},
	}
}

func (p *AlistEncryptPlugin) Initialize(ctx context.Context) error {
	if ctx == p.ctx {
		return nil
	}
	p.ctx = ctx
	p.cfg = config.FromContext(ctx)

	settings, err := config.GetPluginSettingsFor[AlistEncryptPluginConfig](p.cfg, p.Name())
	if err != nil {
		return fmt.Errorf("could not get settings for plugin %s: %w", p.Name(), err)
	}
	p.settings = *settings

	suffix := p.settings.Suffix
	if !strings.HasPrefix(suffix, ".") {
		slog.Warn("alist_encrypt: suffix does not start with '.', falling back to .bin",
			"suffix", suffix)
		p.settings.Suffix = ".bin"
	} else if len(suffix) > 16 {
		slog.Warn("alist_encrypt: suffix exceeds 16 chars, falling back to .bin",
			"suffix", suffix)
		p.settings.Suffix = ".bin"
	}
	// Note: ValidateExtensionUniqueness() in registry.go is responsible for
	// detecting cross-plugin collisions and reporting them. No suffix is
	// silently remapped here.

	if p.settings.EncType != "aesctr" {
		slog.Warn("alist_encrypt: unsupported enc_type, only aesctr is built-in",
			"enc_type", p.settings.EncType)
	}

	return nil
}

func (p *AlistEncryptPlugin) GetMetadataExtractor() pluginInterfaces.MetadataExtractor {
	return nil
}

func (p *AlistEncryptPlugin) GetContentPreprocessor() pluginInterfaces.ContentPreprocessor {
	return nil
}

func (p *AlistEncryptPlugin) GetContentVirifier() pluginInterfaces.ContentVerifier {
	return nil
}

func (p *AlistEncryptPlugin) GetChunkNamer() namer.ChunkNamer {
	return nil
}

func (p *AlistEncryptPlugin) SupportedMimePrefixes() []string {
	return nil
}

func (p *AlistEncryptPlugin) SupportedExtensions() []string {
	return nil
}

func (p *AlistEncryptPlugin) ShouldProcess(inputPath string) bool {
	return true
}

func (p *AlistEncryptPlugin) GroupFiles(inputPaths []string, inputRootDir, outputDir string) ([]string, error) {
	return inputPaths, nil
}

func (p *AlistEncryptPlugin) PreEncryptProcessor(index types.Index, inputPath, inputRootDir, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	p.inputPath = inputPath
	p.outputDir = outputDir
	return nil
}

func (p *AlistEncryptPlugin) Encrypt(dataReader io.Reader) (*crypto.EncryptionResult, error) {
	password := p.resolvePasswordFromTask()
	slog.Debug("alist_encrypt: Encrypt called", "hasPassword", password != "", "hasOutputDir", p.outputDir != "", "outputDir", p.outputDir, "hasSettings", p.settings.Suffix != "")

	result, err := EncryptToFile(dataReader, password, p.outputDir, &p.settings)
	if err != nil {
		return nil, fmt.Errorf("alist_encrypt encryption failed: %w", err)
	}

	return result, nil
}

func (p *AlistEncryptPlugin) PostEncryptProcessor(result *crypto.EncryptionResult) (string, error) {
	originalFilename := filepath.Base(p.inputPath)
	password := p.resolvePasswordFromTask()

	finalPath, err := RenameToFinalEncrypted(result.TempPath, originalFilename, p.outputDir, p.settings.Suffix, password, p.settings.EncType)
	if err != nil {
		os.Remove(result.TempPath)
		return "", fmt.Errorf("failed to rename encrypted file: %w", err)
	}

	slog.Info("alist_encrypt: encryption complete", "output", finalPath)
	return finalPath, nil
}

func (p *AlistEncryptPlugin) CanDecrypt(containerPath string) bool {
	ext := strings.ToLower(filepath.Ext(containerPath))
	if ext != p.settings.Suffix {
		return false
	}
	if info, err := os.Stat(containerPath); err != nil || info.Size() == 0 {
		return false
	}
	if PeekIsAECTR2(containerPath) {
		return true
	}
	f, err := os.Open(containerPath)
	if err != nil {
		return false
	}
	defer f.Close()
	sample := make([]byte, 64)
	n, _ := io.ReadFull(f, sample)
	if n < 8 {
		return false
	}
	// Heuristic: an alist-encrypted ciphertext is high-entropy; plain media
	// files expose recognisable magic headers.  Reject obvious media files so
	// we don't claim ownership of an MP4/MKV/etc. that just happens to end
	// with .bin (e.g. user-renamed uploads).
	if hasKnownMediaMagic(sample[:n]) {
		return false
	}
	return true
}

// hasKnownMediaMagic reports whether the given byte slice contains any of the
// common media / document magic markers.  The slice is the leading bytes of the
// file as it sits on disk — an MP4's ftyp box can sit at offset 4 (after the
// 4-byte size), so we search the whole window rather than only its start.
func hasKnownMediaMagic(b []byte) bool {
	checks := [][]byte{
		[]byte("ftyp"),                 // MP4 / MOV / HEIC (at offset 4 normally)
		[]byte{0x1A, 0x45, 0xDF, 0xA3}, // MKV / WebM (EBML)
		[]byte("RIFF"),                 // AVI / WAV
		[]byte("OggS"),                 // OGG
		[]byte("ID3"),                  // MP3
		[]byte("fLaC"),                 // FLAC
		[]byte("\xFF\xD8\xFF"),         // JPEG
		[]byte("\x89PNG"),              // PNG
		[]byte("GIF8"),                 // GIF
		[]byte("%PDF"),                 // PDF
		[]byte("<?xml"),                // XML
		[]byte("<!DOCTYPE html"),       // HTML
	}
	for _, c := range checks {
		if len(c) > len(b) {
			continue
		}
		if bytes.Contains(b, c) {
			return true
		}
	}
	return false
}

func (p *AlistEncryptPlugin) PreDecryptProcessor(containerPath, outputDir string) error {
	return nil
}

func (p *AlistEncryptPlugin) Decrypt(containerPath, outputDir string) (string, error) {
	ext := strings.ToLower(filepath.Ext(containerPath))
	if ext != p.settings.Suffix {
		return "", &DecryptionError{Reason: "invalid format: extension mismatch", Err: ErrInvalidFormat}
	}

	password := p.resolvePasswordFromTask()

	outputPath, err := DecryptFile(containerPath, outputDir, password, p.settings.EncType)
	if err != nil {
		return "", fmt.Errorf("alist_encrypt decryption failed for '%s': %w", containerPath, err)
	}

	slog.Info("alist_encrypt: decryption complete", "source", containerPath, "output_path", outputPath)
	return outputPath, nil
}

func (p *AlistEncryptPlugin) PostDecryptProcessor(containerPath string) error {
	return nil
}

const containerTypeAlistEncrypt uint16 = 0x000A

func (p *AlistEncryptPlugin) ContainerType() uint16 {
	return containerTypeAlistEncrypt
}

func (p *AlistEncryptPlugin) DefaultIsSeekable(inputPath string) bool {
	return true
}

func (p *AlistEncryptPlugin) DisasterZones(inputPath string) []types.DisasterZone {
	return nil
}

func (p *AlistEncryptPlugin) SupportedContainerVersions() []int {
	return nil
}

func (p *AlistEncryptPlugin) DefaultContainerVersion() int {
	return 0
}

func (p *AlistEncryptPlugin) ValidateVersion(version int) error {
	return fmt.Errorf("alist_encrypt plugin does not use ENCV container versions")
}

func (p *AlistEncryptPlugin) GetTaskOptions() pluginInterfaces.TaskOptions {
	return pluginInterfaces.TaskOptions{
		PasswordStrategy:     pluginInterfaces.PasswordIndependent,
		SupportVersionSelect: false,
		ExtraFields: []pluginInterfaces.TaskField{
			{
				Key:       "plugin_password",
				Label:     "tasks.pluginPassword",
				Type:      "password",
				Required:  false,
				Help:      "tasks.pluginPasswordHelp",
				Condition: "",
			},
			{
				Key:          "encode_filename",
				Label:        "tasks.encodeFilename",
				Type:         "bool",
				Required:     false,
				DefaultValue: "false",
				Help:         "tasks.encodeFilenameHelp",
				Condition:    "encrypt",
			},
			{
				Key:          "enc_type",
				Label:        "tasks.encType",
				Type:         "select",
				Required:     false,
				DefaultValue: "aesctr",
				Help:         "tasks.encTypeHelp",
				Options:      []string{"aesctr"},
				OptionLabels: map[string]string{"aesctr": "AES-CTR-128"},
				Condition:    "encrypt",
			},
		},
	}
}

func (p *AlistEncryptPlugin) resolvePassword() string {
	if p.settings.DefaultPassword != "" {
		return p.settings.DefaultPassword
	}
	return ""
}

func (p *AlistEncryptPlugin) resolvePasswordFromTask() string {
	if p.taskExtraFields != nil {
		if pw := p.taskExtraFields["plugin_password"]; pw != "" {
			return pw
		}
	}
	return p.resolvePassword()
}

func (p *AlistEncryptPlugin) ResolveTaskPassword(taskPassword string, extraFields map[string]string) string {
	return p.resolvePasswordWithTaskExtras(extraFields)
}

func (p *AlistEncryptPlugin) resolvePasswordWithTaskExtras(extraFields map[string]string) string {
	if pw := extraFields["plugin_password"]; pw != "" {
		return pw
	}
	return p.resolvePassword()
}

func (p *AlistEncryptPlugin) SetTaskExtraFields(fields map[string]string) {
	p.taskExtraFields = fields
}

func (p *AlistEncryptPlugin) ResetTaskState() {
	p.taskExtraFields = nil
	p.outputDir = ""
	p.inputPath = ""
}

// (reservedSuffixes / isReservedSuffix removed in Phase 19 — they were dead code.
// The ValidateExtensionUniqueness() function in registry.go is the
// authoritative source of truth for extension conflict detection.)
