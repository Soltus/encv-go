package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PluginAdapter is the agent-side mirror of encv-go's
// `internal/v2/plugins.Plugin`. Keeping the dependency local to
// the agent package lets encv-go's internals (config, container,
// crypto) evolve without forcing the agent module to be aware of
// them.
//
// The contract is intentionally narrow: only the methods the
// scanner + handler actually invoke. Plugins from
// `internal/v2/plugins/registry.go` are wrapped into this shape by
// a small adapter in `cmd/agent-demo` (see plugin_adapter.go
// there).
type PluginAdapter interface {
	// Name is the unique plugin identifier (e.g. "video",
	// "audio", "alist_encrypt"). It is used to derive the tool
	// name (`<name>_encrypt` / `<name>_decrypt`).
	Name() string

	// GetContainerExtension returns the plugin's container file
	// extension, including the leading dot (e.g. ".sccgv"). It is
	// embedded in the tool's description.
	GetContainerExtension() string

	// GetTaskOptions is the source of truth for the schema
	// fragments: PasswordStrategy, SupportVersionSelect,
	// SupportedVersions, DefaultVersion, and ExtraFields.
	GetTaskOptions() PluginTaskOptions

	// SetTaskExtraFields injects the user-supplied extra fields
	// (resolution, codec, …) into the plugin before encryption or
	// decryption begins. Plugins that do not need this method
	// may return without doing anything.
	SetTaskExtraFields(map[string]string)

	// PreEncryptProcessor runs once per input file before
	// encryption. The plugin uses it to extract metadata,
	// preprocess content, or build its index. Implementations
	// should be idempotent for the same inputPath.
	PreEncryptProcessor(ctx context.Context, inputPath, inputRootDir, outputDir string) error

	// Encrypt consumes the prepared reader and produces an
	// EncryptionResult. The agent opens the file and passes the
	// reader in; the plugin does not own the file lifecycle.
	Encrypt(reader io.Reader) (*EncryptionResult, error)

	// PostEncryptProcessor finalizes the encryption (writes
	// manifest, packs chunks, etc.) and returns the absolute
	// path of the produced container file.
	PostEncryptProcessor(ctx context.Context, result *EncryptionResult) (string, error)

	// CanDecrypt reports whether the plugin owns the given
	// container file. The agent uses this as a self-check
	// before invoking PreDecryptProcessor.
	CanDecrypt(containerPath string) bool

	// PreDecryptProcessor runs before decryption. Plugins use
	// it to validate the container header and set up their
	// internal state.
	PreDecryptProcessor(ctx context.Context, containerPath, outputDir string) error

	// Decrypt decrypts the container file into the given
	// outputDir and returns the absolute path of the
	// resulting plaintext file.
	Decrypt(ctx context.Context, containerPath, outputDir string) (string, error)

	// PostDecryptProcessor runs after successful decryption. It
	// gives the plugin a chance to clean up temporary files or
	// verify the output.
	PostDecryptProcessor(ctx context.Context, containerPath string) error
}

// EncryptionResult is the agent-side mirror of
// `internal/v2/crypto.EncryptionResult`. We only keep the fields
// the handler cares about; the rest is opaque to the agent.
type EncryptionResult struct {
	// TempPath is the on-disk path of the encrypted payload.
	TempPath string
	// Salt is the encryption salt. Plugins may use it to
	// derive the key; the agent itself does not.
	Salt []byte
	// IV is the initialization vector.
	IV []byte
	// EncryptedPayloadSize is the size of the encrypted
	// payload, excluding header.
	EncryptedPayloadSize int64
}

// PluginInput is the parsed argument shape shared by encrypt and
// decrypt tools. Fields are populated from the LLM's tool-call
// args JSON. The LLM is expected to provide a superset of the
// declared schema; unknown fields are tolerated.
type PluginInput struct {
	InputPaths   []string          `json:"input_paths"`
	OutputPath   string            `json:"output_path"`
	ExtraFields  map[string]string `json:"extra_fields,omitempty"`
	Password     string            `json:"password,omitempty"`
	Version      int               `json:"version,omitempty"`
}

// PluginOutput is the JSON shape returned to the LLM after a
// successful encrypt / decrypt call.
type PluginOutput struct {
	OutputPaths []string `json:"output_paths"`
	DurationMs  int64    `json:"duration_ms"`
	FileSize    int64    `json:"file_size,omitempty"`
	Plugin      string   `json:"plugin"`
	Operation   string   `json:"operation"`
}

// scanPluginTools builds the slice of ToolDefinitions that wraps
// the encv-go plugin system as agent tools. Each plugin (other
// than alistencrypt) yields two tools: `<name>_encrypt` and
// `<name>_decrypt`. The alistencrypt plugin is intentionally
// skipped because the same functionality is exposed via
// OpenList's /api/ext/* endpoints (avoiding duplicate tools).
//
// The third return value is a name→plugin lookup map, which
// keeps the agent-demo wiring honest (the caller can verify
// every tool that was registered is one this function emitted).
func scanPluginTools(plugins []PluginAdapter) ([]ToolDefinition, map[string]PluginAdapter, error) {
	var tools []ToolDefinition
	lookup := make(map[string]PluginAdapter)

	for _, p := range plugins {
		name := p.Name()
		// Spec requirement: alistencrypt is exposed via OpenList
		// tools, not as an agent-side plugin tool.
		if name == "alistencrypt" || name == "alist_encrypt" {
			continue
		}

		opts := p.GetTaskOptions()
		encryptSchema := buildPluginSchema(name, "encrypt", opts, p.GetContainerExtension())
		decryptSchema := buildPluginSchema(name, "decrypt", opts, p.GetContainerExtension())

		tools = append(tools, ToolDefinition{
			Schema:      encryptSchema,
			Handler:     makePluginEncryptHandler(p),
			NeedConfirm: true,
			Kind:        KindFileChange,
		})
		tools = append(tools, ToolDefinition{
			Schema:      decryptSchema,
			Handler:     makePluginDecryptHandler(p),
			NeedConfirm: true,
			Kind:        KindFileChange,
		})
		lookup[name+"_encrypt"] = p
		lookup[name+"_decrypt"] = p
	}

	if len(lookup) == 0 {
		return nil, nil, fmt.Errorf("scanPluginTools: no plugins yielded tools (input had %d plugins)", len(plugins))
	}
	return tools, lookup, nil
}

// buildPluginSchema assembles the OpenAI function-calling JSON
// schema for one encrypt or decrypt tool.
//
// The schema is built from three sources:
//  1. Fixed fields: input_paths, output_path (always present).
//  2. Password: governed by PasswordStrategy (global /
//     independent / none).
//  3. Version: governed by SupportVersionSelect.
//  4. extra_fields: free-form object whose keys mirror the
//     ExtraFields from TaskOptions. The agent does not validate
//     individual extra fields — the plugin does that — but
//     declaring them in the schema helps the LLM reason about
//     what to send.
func buildPluginSchema(pluginName, op string, opts PluginTaskOptions, ext string) map[string]any {
	props := map[string]any{
		"input_paths": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "OpenList 端输入文件绝对路径（单文件请传单元素数组）",
		},
		"output_path": map[string]any{
			"type":        "string",
			"description": "输出容器路径（加密）或输出目录（解密）",
		},
		"extra_fields": map[string]any{
			"type":        "object",
			"description": "插件任务级额外字段（如分辨率、编码器、fn_rounds）",
			"properties":  extraFieldsSchema(opts.ExtraFields, op),
		},
	}

	required := []string{"input_paths", "output_path"}

	// Password strategy controls the password field.
	switch opts.PasswordStrategy {
	case PasswordGlobal:
		// Hidden from the LLM; the agent uses the global
		// password from AgentConfig.
	case PasswordIndependent:
		props["password"] = map[string]any{
			"type":        "string",
			"description": "本插件独立密码（alist_encrypt 必填）",
		}
		required = append(required, "password")
	case PasswordNone:
		// No password field at all.
	}

	// Container version selection.
	if opts.SupportVersionSelect {
		versionEnum := make([]any, 0, len(opts.SupportedVersions))
		for _, v := range opts.SupportedVersions {
			versionEnum = append(versionEnum, v)
		}
		versionProp := map[string]any{
			"type":        "integer",
			"enum":        versionEnum,
			"description": "容器版本（从插件支持的版本中选择）",
		}
		if opts.DefaultVersion > 0 {
			versionProp["default"] = opts.DefaultVersion
		}
		props["version"] = versionProp
		required = append(required, "version")
	}

	description := pluginDescription(pluginName, op, ext, opts)

	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        pluginName + "_" + op,
			"description": description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": props,
				requiredKey: required,
			},
		},
	}
}

const requiredKey = "required"

// extraFieldsSchema flattens the plugin's TaskField slice into a
// JSON-Schema properties map keyed by the field's Key. The agent
// deliberately keeps this generic — the plugin is the authority
// on what each field means.
func extraFieldsSchema(fields []PluginTaskField, op string) map[string]any {
	out := map[string]any{}
	for _, f := range fields {
		if f.Condition != "" && f.Condition != op && f.Condition != "both" {
			continue
		}
		entry := map[string]any{
			"type":        pluginFieldType(f.Type),
			"description": f.Help,
		}
		if f.DefaultValue != "" {
			entry["default"] = f.DefaultValue
		}
		if len(f.Options) > 0 {
			opts := make([]any, 0, len(f.Options))
			for _, o := range f.Options {
				opts = append(opts, o)
			}
			entry["enum"] = opts
		}
		if f.Type == "password" {
			entry["format"] = "password"
		}
		out[f.Key] = entry
	}
	return out
}

// pluginFieldType maps the plugin's free-form type string to a
// JSON-Schema primitive. Unknown values fall back to "string" —
// the agent never invents types the plugin did not declare.
func pluginFieldType(t string) string {
	switch strings.ToLower(t) {
	case "string", "password", "select", "multiline":
		return "string"
	case "int", "integer", "number":
		return "integer"
	case "bool", "boolean":
		return "boolean"
	case "array":
		return "array"
	case "object":
		return "object"
	default:
		return "string"
	}
}

// pluginDescription composes the tool description in the style
// required by the spec: Chinese, mentioning both the plugin name
// and its container extension.
func pluginDescription(pluginName, op, ext string, opts PluginTaskOptions) string {
	opCN := "加密"
	if op == "decrypt" {
		opCN = "解密"
	}
	ver := ""
	if opts.SupportVersionSelect {
		ver = "。支持容器版本选择"
	}
	pwd := ""
	switch opts.PasswordStrategy {
	case PasswordGlobal:
		pwd = "。使用全局密码"
	case PasswordIndependent:
		pwd = "。需要显式传入本插件独立密码"
	}
	return fmt.Sprintf(
		"使用 %s 插件将文件%s为 %s 容器%s%s。输入：input_paths + output_path + extra_fields%s。",
		pluginName, opCN, extOrPlaceholder(ext), ver, pwd, "",
	)
}

func extOrPlaceholder(ext string) string {
	if ext == "" {
		return "<unknown>"
	}
	return ext
}

// makePluginEncryptHandler returns a Handler that drives one
// encrypt call through the plugin. The handler is intentionally
// tolerant: a per-file error is reported inside the per-file
// sub-result so the LLM can decide to continue or stop.
func makePluginEncryptHandler(p PluginAdapter) func(string) (string, error) {
	return func(args string) (string, error) {
		var in PluginInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return errorJSON("invalid_args", err.Error()), nil
		}
		if len(in.InputPaths) == 0 {
			return errorJSON("missing_input_paths", "input_paths must not be empty"), nil
		}
		if in.OutputPath == "" {
			return errorJSON("missing_output_path", "output_path is required"), nil
		}
		if in.ExtraFields != nil {
			p.SetTaskExtraFields(in.ExtraFields)
		}
		// Ensure the output directory exists.
		outDir := filepath.Dir(in.OutputPath)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return errorJSON("mkdir_failed", err.Error()), nil
		}

		start := nowMillis()
		// For a single-file flow, outputPath is treated as the
		// container file. For multi-file inputs, the plugin's
		// GroupFiles / PostEncryptProcessor decides the layout.
		var outputs []string
		for _, ip := range in.InputPaths {
			ctx := context.Background()
			if err := p.PreEncryptProcessor(ctx, ip, filepath.Dir(ip), outDir); err != nil {
				return errorJSON("pre_encrypt_failed", err.Error()), nil
			}
			f, err := os.Open(ip)
			if err != nil {
				return errorJSON("open_input_failed", err.Error()), nil
			}
			res, err := p.Encrypt(f)
			_ = f.Close()
			if err != nil {
				return errorJSON("encrypt_failed", err.Error()), nil
			}
			out, err := p.PostEncryptProcessor(ctx, res)
			if err != nil {
				return errorJSON("post_encrypt_failed", err.Error()), nil
			}
			outputs = append(outputs, out)
		}
		out := PluginOutput{
			OutputPaths: outputs,
			DurationMs:  nowMillis() - start,
			Plugin:      p.Name(),
			Operation:   "encrypt",
		}
		return toJSON(out), nil
	}
}

// makePluginDecryptHandler returns a Handler for one decrypt call.
// It performs the spec-mandated CanDecrypt self-check before
// dispatching to the plugin.
func makePluginDecryptHandler(p PluginAdapter) func(string) (string, error) {
	return func(args string) (string, error) {
		var in PluginInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return errorJSON("invalid_args", err.Error()), nil
		}
		if len(in.InputPaths) == 0 {
			return errorJSON("missing_input_paths", "input_paths must not be empty"), nil
		}
		if in.OutputPath == "" {
			return errorJSON("missing_output_path", "output_path is required"), nil
		}
		if in.ExtraFields != nil {
			p.SetTaskExtraFields(in.ExtraFields)
		}
		if err := os.MkdirAll(in.OutputPath, 0o755); err != nil {
			return errorJSON("mkdir_failed", err.Error()), nil
		}

		start := nowMillis()
		var outputs []string
		for _, containerPath := range in.InputPaths {
			if !p.CanDecrypt(containerPath) {
				// Spec §3.4: self-check failure yields a
				// structured error suggesting a better-suited
				// plugin. We don't have a registry here, so the
				// suggestion is "check the extension" — the LLM
				// is expected to know which plugin owns which
				// container.
				suggest := suggestDecryptTool(containerPath)
				payload := map[string]any{
					"error":               "container_format_mismatch",
					"path":                containerPath,
					"suggested_tool":      suggest,
					"current_plugin":      p.Name(),
				}
				return toJSON(payload), nil
			}
			ctx := context.Background()
			if err := p.PreDecryptProcessor(ctx, containerPath, in.OutputPath); err != nil {
				return errorJSON("pre_decrypt_failed", err.Error()), nil
			}
			out, err := p.Decrypt(ctx, containerPath, in.OutputPath)
			if err != nil {
				return errorJSON("decrypt_failed", err.Error()), nil
			}
			if err := p.PostDecryptProcessor(ctx, containerPath); err != nil {
				return errorJSON("post_decrypt_failed", err.Error()), nil
			}
			outputs = append(outputs, out)
		}
		out := PluginOutput{
			OutputPaths: outputs,
			DurationMs:  nowMillis() - start,
			Plugin:      p.Name(),
			Operation:   "decrypt",
		}
		return toJSON(out), nil
	}
}

// suggestDecryptTool looks at the container's extension and
// suggests which plugin's decrypt tool is the right one. The
// mapping is conservative: we only have the extension, so the
// suggestion is a hint, not a guarantee.
func suggestDecryptTool(containerPath string) string {
	ext := strings.ToLower(filepath.Ext(containerPath))
	switch ext {
	case ".sccgv":
		return "video_decrypt"
	case ".sccga":
		return "audio_decrypt"
	case ".sccgi":
		return "image_decrypt"
	case ".sccgwps":
		return "wps_decrypt"
	case ".sccgpdf":
		return "pdf_decrypt"
	case ".sccgt":
		return "text_decrypt"
	case ".bin":
		return "(use OpenList alist tools, not a plugin decrypt tool)"
	default:
		return ""
	}
}

// errorJSON is a small helper that wraps a structured error into
// the tool_result JSON shape. The agent treats the return value
// as a plain JSON string, so we keep this as compact as possible.
func errorJSON(code, message string) string {
	return toJSON(map[string]any{"error": code, "message": message})
}

// PluginErrorJSON is the public alias of errorJSON. Callers
// outside the scanner use it to build consistent error
// payloads for tool results (e.g. the OpenList tool
// registrations in cmd/agent-demo).
func PluginErrorJSON(code, message string) string {
	return errorJSON(code, message)
}

// toJSON is a thin wrapper around json.Marshal that never panics
// (we own every payload here).
func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":"marshal_failed","message":%q}`, err.Error())
	}
	return string(b)
}
