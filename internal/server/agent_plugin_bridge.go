// internal/server/agent_plugin_bridge.go
//
// 把 encv-go 插件系统接入 AI agent 工具系统。
//
// 设计目标：
//   - 12 个工具：6 个插件（video/audio/image/wps/pdf/text）× 2 操作（encrypt/decrypt）
//   - 工具描述用中文，符合用户使用习惯
//   - schema 动态生成：input_paths / output_path / extra_fields（来自 plugin.GetTaskOptions().ExtraFields）/
//     password（按 PasswordStrategy 决定 required/optional/hidden）/ version（按 SupportVersionSelect）
//   - 复用 plugins.EncryptFileWithPlugin / DecryptContainerWithPlugin 高层 API
//   - 安全：所有 encrypt/decrypt 操作都标记为 NeedConfirm=true，前端必须弹 ApprovalCard
//
// 不做的事（明确范围）：
//   - 不集成 OpenList 工具（用户已砍掉）
//   - 不做 4 决策的复杂 confirm 流程（先 accept/decline 两个够用）
//   - 不做断点续传（先做核心工具调用闭环）
//   - 不做虚拟列表/分组渲染（后续 phase）
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/Soltus/encv-go/internal/tools"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	encvPlugins "github.com/Soltus/encv-go/pkg/encv/plugins"
)

// pluginToolDef 描述一个由插件支持的 agent 工具
type pluginToolDef struct {
	plugin      plugins.Plugin
	name        string
	description string
	op          string // "encrypt" | "decrypt"
}

// pluginOpsByName 建立 "video_encrypt" / "video_decrypt" → pluginName + op 的映射
var pluginOpsByName = func() map[string]pluginToolDef {
	m := make(map[string]pluginToolDef)
	for _, p := range encvPlugins.Plugins() {
		// 跳过 OpenList 相关插件（用户已砍掉 OpenList 集成）
		// 命名约定：以 "alist" 开头的插件都属于 OpenList 工具族
		if strings.HasPrefix(p.Name(), "alist") {
			continue
		}
		// 中文插件名映射（用户可见名）
		cnName := pluginNameCN(p.Name())
		m[p.Name()+"_encrypt"] = pluginToolDef{
			plugin:      p,
			name:        p.Name() + "_encrypt",
			description: fmt.Sprintf("使用 %s 插件加密文件为 %s 容器", cnName, p.GetContainerExtension()),
			op:          "encrypt",
		}
		m[p.Name()+"_decrypt"] = pluginToolDef{
			plugin:      p,
			name:        p.Name() + "_decrypt",
			description: fmt.Sprintf("使用 %s 插件解密 %s 容器为原始文件", cnName, p.GetContainerExtension()),
			op:          "decrypt",
		}
	}
	return m
}()

// pluginNameCN 把插件名翻译成中文（用户可读）
func pluginNameCN(name string) string {
	switch name {
	case "video":
		return "视频"
	case "audio":
		return "音频"
	case "image":
		return "图片"
	case "wps":
		return "WPS 文档"
	case "pdf":
		return "PDF"
	case "text":
		return "文本"
	default:
		return name
	}
}

// toolSchemaEncrypt / toolSchemaDecrypt 动态 schema 生成器
// 根据插件 GetTaskOptions() 返回的元数据，动态生成完整 schema：
//   - input_paths/output_path 基础字段
//   - extra_fields 来自 plugin.ExtraFields（每个 TaskField 转为 JSON Schema 属性）
//   - password 按 PasswordStrategy（global=optional, independent=optional, none=隐藏）
//   - version 按 SupportVersionSelect（true=必填 select, false=隐藏）

// buildTaskFieldSchema 把单个 TaskField 转成 JSON Schema property
func buildTaskFieldSchema(f pluginInterfaces.TaskField) map[string]interface{} {
	prop := map[string]interface{}{
		"type":        jsonSchemaType(f.Type),
		"description": f.Label,
	}
	if f.Help != "" {
		prop["description"] = f.Label + " — " + f.Help
	}
	if f.DefaultValue != "" {
		prop["default"] = f.DefaultValue
	}
	if len(f.Options) > 0 {
		prop["enum"] = f.Options
		if len(f.OptionLabels) > 0 {
			prop["enumNames"] = f.OptionLabels
		}
	}
	return prop
}

// jsonSchemaType 把 plugin field type 映射到 JSON Schema type
func jsonSchemaType(t string) string {
	switch t {
	case "bool":
		return "boolean"
	case "int", "integer":
		return "integer"
	case "float", "number":
		return "number"
	case "select", "string", "password", "text":
		return "string"
	case "array":
		return "array"
	default:
		return "string"
	}
}

// buildDynamicSchema 为指定插件+操作生成完整的 JSON Schema
func buildDynamicSchema(p plugins.Plugin, op string) map[string]interface{} {
	opts := p.GetTaskOptions()

	properties := map[string]interface{}{}
	required := []string{}

	if op == "encrypt" {
		properties["input_paths"] = map[string]interface{}{
			"type":        "array",
			"items":       map[string]interface{}{"type": "string"},
			"description": "要加密的源文件绝对路径列表（可批量）",
		}
		properties["output_path"] = map[string]interface{}{
			"type":        "string",
			"description": "加密产物容器输出路径（绝对路径，扩展名由 plugin.GetContainerExtension() 决定）",
		}
		required = append(required, "input_paths", "output_path")
	} else { // decrypt
		properties["container_path"] = map[string]interface{}{
			"type":        "string",
			"description": "加密容器文件绝对路径（扩展名由 plugin.GetContainerExtension() 决定）",
		}
		properties["output_dir"] = map[string]interface{}{
			"type":        "string",
			"description": "解密产物输出目录（绝对路径）",
		}
		required = append(required, "container_path", "output_dir")
	}

	// ② extra_fields：按 op 过滤（plugin 通过 Condition 字段声明 encrypt/decrypt）
	extraProps := map[string]interface{}{}
	for _, f := range opts.ExtraFields {
		if f.Condition != "" && f.Condition != op {
			continue
		}
		extraProps[f.Key] = buildTaskFieldSchema(f)
		if f.Required {
			required = append(required, f.Key)
		}
	}
	if len(extraProps) > 0 {
		properties["extra_fields"] = map[string]interface{}{
			"type":        "object",
			"description": "插件高级选项（动态生成自 " + p.Name() + " 插件）",
			"properties":  extraProps,
		}
		required = append(required, "extra_fields")
	}

	// ③ password：按 PasswordStrategy 决定
	switch opts.PasswordStrategy {
	case pluginInterfaces.PasswordGlobal:
		// 用全局密码（从配置读），不传给 LLM
	case pluginInterfaces.PasswordIndependent:
		// 插件独立密码，LLM 必须传
		properties["password"] = map[string]interface{}{
			"type":        "string",
			"description": "该插件的主密码（与全局密码独立）",
		}
		required = append(required, "password")
	case pluginInterfaces.PasswordNone:
		// 不需要密码，不暴露字段
	}

	// ④ version：按 SupportVersionSelect
	if opts.SupportVersionSelect {
		verProp := map[string]interface{}{
			"type":        "integer",
			"description": "容器版本号",
			"default":     opts.DefaultVersion,
		}
		if len(opts.SupportedVersions) > 0 {
			verProp["enum"] = opts.SupportedVersions
		}
		properties["version"] = verProp
		required = append(required, "version")
	}

	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

// ListPluginTools 返回所有插件工具的元信息（name + description + schema）。
// 前端可调用以渲染"已启用工具"列表。
// Schema 是动态生成的：根据每个 plugin 的 GetTaskOptions() 返回 ExtraFields / PasswordStrategy / SupportVersionSelect
func ListPluginTools() []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(pluginOpsByName))
	for _, def := range pluginOpsByName {
		out = append(out, map[string]interface{}{
			"name":        def.name,
			"description": def.description,
			"parameters":  buildDynamicSchema(def.plugin, def.op),
			"needConfirm": true, // 加密/解密均需用户确认（写入文件是高危操作）
		})
	}
	return out
}

// ListAgentTools 合并所有 agent 可用工具：插件工具（加密/解密）+ fs 工具（只读）。
// 这是真正发到 OpenAI /v1/chat/completions 的 "tools" 字段。
//
// 重要：之前 ListPluginTools() 从未被发给 LLM（handleAgentChat 的 reqBody 里没 "tools" 字段），
// 也就是说 agent 实际根本无法调用任何 tool，只能聊天。本函数是修复这个核心断点的入口。
func (s *Server) ListAgentTools() []map[string]interface{} {
	out := ListPluginTools()
	out = append(out, s.ListFSTools()...)
	return out
}

// executeAgentTool 统一派发所有 agent 工具调用。
//
// 派发顺序（v2 spec）：
//  1. 工具注册表（tools.GlobalRegistry）—— 新工具（search_files / get_metadata /
//     read_file_v2 / command_run / edit_metadata / batch_rename）
//  2. 旧插件工具表（pluginOpsByName）—— 兼容 encrypt_video 等插件
//  3. fs 工具（list_mounts / list_files / read_file 等）—— 兼容 v1
//
// 不存在的工具名 → 报错。
//
// 这是把 fs 工具接入 agent 系统的入口。
// executePluginTool 保留为旧入口（向后兼容 + 测试）；新代码应调 executeAgentTool。
func (s *Server) executeAgentTool(ctx context.Context, toolName, argsJSON string) (string, error) {
	// v2 工具注册表优先（search_files / get_metadata / command_run / edit_metadata）
	if s.toolDeps != nil && tools.GlobalRegistry.Has(toolName) {
		res, err := tools.GlobalRegistry.Dispatch(ctx, toolName, argsJSON, s.toolDeps)
		if err != nil {
			slog.Warn("tool dispatch error", "tool", toolName, "error", err)
			return res.Result, err
		}
		return res.Result, nil
	}
	// 旧插件工具（encrypt_video / decrypt_video 等）
	if _, ok := pluginOpsByName[toolName]; ok {
		return executePluginTool(ctx, toolName, argsJSON)
	}
	// fs 工具（兼容 v1）
	return s.executeFSTool(ctx, toolName, argsJSON)
}

// executePluginTool 执行一个插件工具调用。
// 返回 (outputJSON, error)。outputJSON 描述执行结果（output_path / error / 耗时等）。
//
// 调用方负责决策 confirm / decline / cancel。
func executePluginTool(ctx context.Context, toolName, argsJSON string) (string, error) {
	def, ok := pluginOpsByName[toolName]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}

	switch def.op {
	case "encrypt":
		return runPluginEncrypt(ctx, def, argsJSON)
	case "decrypt":
		return runPluginDecrypt(ctx, def, argsJSON)
	default:
		return "", fmt.Errorf("unknown op: %s", def.op)
	}
}

func runPluginEncrypt(ctx context.Context, def pluginToolDef, argsJSON string) (string, error) {
	var args struct {
		InputPaths  []string               `json:"input_paths"`
		OutputPath  string                 `json:"output_path"`
		ExtraFields map[string]interface{} `json:"extra_fields"`
		Password    string                 `json:"password"`
		Version     int                    `json:"version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errJSON("invalid_args", err.Error()), nil
	}
	if len(args.InputPaths) == 0 || args.OutputPath == "" {
		return errJSON("missing_args", "input_paths 和 output_path 必填"), nil
	}

	// 推断插件（按文件类型）—— 单文件路径（多文件取第一个判断）
	pluginName := strings.TrimSuffix(def.name, "_encrypt")
	inputPath := args.InputPaths[0]
	p, err := plugins.FindEncryptingPlugin(inputPath)
	if err != nil {
		// 兜底：尝试用 toolName 找 plugin
		var err2 error
		p, err2 = findPluginByName(pluginName)
		if err2 != nil {
			return errJSON("plugin_not_found", err.Error()), nil
		}
	}

	// 验证插件名匹配（防止 AI 用 video 工具处理 pdf 文件）
	if p.Name() != pluginName {
		return errJSON("plugin_mismatch",
			fmt.Sprintf("文件类型需要 %s 插件，但你调用的是 %s 工具", p.Name(), pluginName),
		), nil
	}

	// 注入 extra_fields（如果插件实现 SetTaskExtraFields）
	injectExtraFields(p, args.ExtraFields)

	inputRootDir := filepath.Dir(inputPath)
	outputPath, err := plugins.EncryptFileWithPlugin(ctx, p, inputPath, inputRootDir, args.OutputPath, nil)
	if err != nil {
		slog.Warn("agent: plugin encrypt failed", "plugin", p.Name(), "input", inputPath, "error", err)
		return errJSON("encrypt_failed", err.Error()), nil
	}

	return okJSON(map[string]interface{}{
		"plugin":  p.Name(),
		"op":      "encrypt",
		"input":   inputPath,
		"output":  outputPath,
		"version": args.Version,
	}), nil
}

func runPluginDecrypt(ctx context.Context, def pluginToolDef, argsJSON string) (string, error) {
	var args struct {
		ContainerPath string                 `json:"container_path"`
		OutputDir     string                 `json:"output_dir"`
		ExtraFields   map[string]interface{} `json:"extra_fields"`
		Password      string                 `json:"password"`
		Version       int                    `json:"version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errJSON("invalid_args", err.Error()), nil
	}
	if args.ContainerPath == "" || args.OutputDir == "" {
		return errJSON("missing_args", "container_path 和 output_dir 必填"), nil
	}

	// 推断插件
	pluginName := strings.TrimSuffix(def.name, "_decrypt")
	p, err := plugins.FindDecryptingPlugin(args.ContainerPath)
	if err != nil {
		var err2 error
		p, err2 = findPluginByName(pluginName)
		if err2 != nil {
			return errJSON("plugin_not_found", err.Error()), nil
		}
	}

	if p.Name() != pluginName {
		return errJSON("plugin_mismatch",
			fmt.Sprintf("容器类型需要 %s 插件，但你调用的是 %s 工具", p.Name(), pluginName),
		), nil
	}

	// 注入 extra_fields
	injectExtraFields(p, args.ExtraFields)

	outputPath, err := plugins.DecryptContainerWithPlugin(ctx, p, args.ContainerPath, args.OutputDir, nil)
	if err != nil {
		slog.Warn("agent: plugin decrypt failed", "plugin", p.Name(), "input", args.ContainerPath, "error", err)
		return errJSON("decrypt_failed", err.Error()), nil
	}

	return okJSON(map[string]interface{}{
		"plugin":  p.Name(),
		"op":      "decrypt",
		"input":   args.ContainerPath,
		"output":  outputPath,
		"version": args.Version,
	}), nil
}

// injectExtraFields 通过 type assertion 调用插件的 SetTaskExtraFields
// 如果插件未实现（6 个核心插件目前都没实现），则 no-op。
func injectExtraFields(p plugins.Plugin, fields map[string]interface{}) {
	if len(fields) == 0 {
		return
	}
	setter, ok := p.(pluginInterfaces.TaskExtraFieldsSetter)
	if !ok {
		slog.Debug("agent: plugin doesn't implement SetTaskExtraFields, skipping", "plugin", p.Name())
		return
	}
	// 把 map[string]interface{} 转 map[string]string（plugin API 要求）
	strMap := make(map[string]string, len(fields))
	for k, v := range fields {
		strMap[k] = fmt.Sprintf("%v", v)
	}
	setter.SetTaskExtraFields(strMap)
}

// findPluginByName 在 plugins 列表中按 name 查找插件
func findPluginByName(name string) (plugins.Plugin, error) {
	for _, p := range plugins.Plugins {
		if p.Name() == name {
			return p, nil
		}
	}
	return nil, fmt.Errorf("plugin %q not found in registry", name)
}

// okJSON 把 map 序列化为成功 JSON 字符串
func okJSON(m map[string]interface{}) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// errJSON 把错误包装为 {"error": code, "message": msg} JSON
func errJSON(code, msg string) string {
	return okJSON(map[string]interface{}{"error": code, "message": msg})
}

// agentToolsToOpenAITools 把 agent 内部 tool 元数据转换为 OpenAI /v1/chat/completions
// "tools" 字段要求的格式：
//
//	[
//	  { "type": "function", "function": { "name": ..., "description": ..., "parameters": ... } },
//	  ...
//	]
//
// 注意：OpenAI 不认 "needConfirm" 字段（那是前端用的），这里主动剔除；
// 保留 "kind" 字段也无害（OpenAI 忽略未知字段）。
func agentToolsToOpenAITools(tools []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(tools))
	for _, t := range tools {
		fn := map[string]interface{}{
			"name": t["name"],
		}
		if d, ok := t["description"].(string); ok {
			fn["description"] = d
		}
		if p, ok := t["parameters"]; ok {
			fn["parameters"] = p
		}
		out = append(out, map[string]interface{}{
			"type":     "function",
			"function": fn,
		})
	}
	return out
}
