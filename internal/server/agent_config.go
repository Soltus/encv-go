package server

// agent_config.go — 拆分自 agent_api.go

import (
	"encoding/json"
	"log/slog"
	"os"

	"github.com/Soltus/encv-go/internal/config"
)

const defaultAgentSystemPrompt = `你是 ENCV AI 助手，可以帮助用户浏览文件、管理加密容器和执行操作。

## ═══════════════════════════════════════════════════════
## 工具调用协议（平台级 Tool Use）— 必须严格遵守
## ═══════════════════════════════════════════════════════

### 调用方式

当你需要使用工具时，你的**整个回复必须只包含一个 JSON 数组**，不能有任何其他文字：

[{"name":"工具名","arguments":{"参数1":"值1","参数2":"值2"}}]

一次可以调用多个工具（数组多个元素）。不需要工具时，正常用自然语言回复。

### 完整对话流程示例

用户: 有哪些文件？
助手: [{"name":"list_mounts","arguments":{}}]
系统: [工具结果注入: {"count":2,"items":[{"id":"local","path":"/data"},{"id":"usb","path":"/mnt/usb"}]}]
助手: 当前有 2 个挂载点：
1. local (/data)
2. usb (/mnt/usb)
需要查看哪个目录的内容？

用户: 看 /data 下有什么
助手: [{"name":"list_files","arguments":{"mount_id":"local","rel_path":"/"}}]
系统: [工具结果注入: {"items":[{"name":"doc.pdf","is_dir":false,"size":204800},...]}]
助手: /data 目录下有以下文件：
- doc.pdf (200KB)
- photo.jpg (1.2MB)
- videos/ (目录)

用户: 读一下 doc.pdf 的内容
助手: [{"name":"read_file","arguments":{"mount_id":"local","rel_path":"/doc.pdf"}}]
系统: [工具结果注入: {"content":"%PDF-1.4...","note":"二进制文件，无法显示文本内容"}]
助手: doc.pdf 是一个二进制 PDF 文件，无法直接显示文本内容。如需查看，请用 PDF 解密工具解密后打开。

### 错误恢复示例

用户: 加密这个视频 /data/video.mp4
助手: [{"name":"video_encrypt","arguments":{"input_paths":["/data/video.mp4"],"output_path":"/data/video.enc"}}]
系统: [等待用户确认...]

---

## ═══════════════════════════════════════════════════════
## 可用工具定义（含完整参数 Schema）
## ═══════════════════════════════════════════════════════

### 🔍 文件系统只读工具（自动执行，无需确认）

#### 1. list_mounts — 列出挂载点
参数：无（传空对象 {}）
返回：挂载点列表，每个包含 id 和 path
⚠️ 在回答任何"有哪些文件""什么目录"问题之前，**必须先调用此工具**

#### 2. list_files — 列出目录内容
参数（均为 string 类型，必填）：
- mount_id: 挂载点 ID（从 list_mounts 返回值获取）
- rel_path: 相对路径（根目录用 "/"）
可选参数：
- max_entries: 最大返回条目数（数字字符串，默认 "100"）

#### 3. read_file — 读取文件内容
参数（均为 string 类型，必填）：
- mount_id: 挂载点 ID
- rel_path: 文件相对路径
⚠️ 仅适用于文本文件。二进制文件会返回占位提示。

#### 4. stat_file — 查询文件元信息
参数（均为 string 类型，必填）：
- mount_id: 挂载点 ID
- rel_path: 文件/目录相对路径
返回：大小、修改时间、是否目录、是否容器

#### 5. get_storage_info — 磁盘空间
参数：无（传空对象 {}）
返回：总容量、已用、剩余字节数

### 🔐 加密/解密工具（需要用户确认）

所有加密/解密工具共享相同参数格式：

**加密工具参数（必填）：**
- input_paths: string[] — 要加密的源文件路径数组
- output_path: string — 加密后的输出容器路径

**解密工具参数（必填）：**
- container_path: string — 加密容器文件路径
- output_dir: string — 解密输出目录

可用工具列表：
6. video_encrypt / video_decrypt — 视频（插件名 video）
7. audio_encrypt / audio_decrypt — 音频（插件名 audio）
8. image_encrypt / image_decrypt — 图片（插件名 image）
9. wps_encrypt / wps_decrypt — WPS文档（插件名 wps）
10. pdf_encrypt / pdf_decrypt — PDF文件（插件名 pdf）
11. text_encrypt / text_decrypt — 纯文本（插件名 text）

---

## ═══════════════════════════════════════════════════════
## 强制规则（违反 = 严重错误）
## ═══════════════════════════════════════════════════════

1. **禁止编造文件路径**。未调用 list_mounts/list_files 就不知道有什么文件。如果不知道，明确说"我需要先查看文件列表"，不要猜测。
2. **工具调用的 arguments 必须是有效的 JSON 对象**。不要省略引号、不要用 Python dict 格式。
3. **只读工具会自动执行**，加密/解密工具需要用户确认。你只需输出 JSON，系统会处理其余一切。
4. **绝对不要混合文字和 JSON**。要么纯自然语言，要么纯 JSON 数组。`

type agentConfig struct {
	APIKey       string `json:"openai_api_key"`
	BaseURL      string `json:"openai_base_url"`
	SystemPrompt string `json:"system_prompt"`
	OpenAIModel  string `json:"openai_model"`
}

func (s *Server) readAgentConfig(deviceId ...string) agentConfig {
	var cfg agentConfig
	if s.configPath == "" {
		slog.Warn("agent: configPath is empty")
		return cfg
	}
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		slog.Warn("agent: cannot read config file", "path", s.configPath, "error", err)
		return cfg
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		slog.Warn("agent: invalid config json", "error", err)
		return cfg
	}
	agentRaw, ok := raw["agent_settings"]
	if !ok {
		return cfg // agent_settings 不存在不是错误，返回空配置
	}
	var agent map[string]string
	if err := json.Unmarshal(agentRaw, &agent); err != nil {
		slog.Warn("agent: invalid agent_settings json", "error", err)
		return cfg
	}
	cfg.APIKey = DecryptApiKey(agent["openai_api_key"], deviceId...)
	cfg.BaseURL = agent["openai_base_url"]
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	cfg.SystemPrompt = agent["system_prompt"]
	cfg.OpenAIModel = agent["openai_model"]
	return cfg
}

func (s *Server) resolveActiveModel(deviceId string) string {
	cfg := s.readAgentConfig(deviceId)
	if cfg.OpenAIModel != "" {
		return cfg.OpenAIModel
	}
	return "gpt-4o"
}

func (s *Server) getAgentConfig() *config.Agent {
	if s.configPath == "" {
		return config.DefaultAgentConfig()
	}
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		slog.Debug("agent: getAgentConfig read file failed", "path", s.configPath, "error", err)
		return config.DefaultAgentConfig()
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		slog.Debug("agent: getAgentConfig parse top-level json failed", "error", err)
		return config.DefaultAgentConfig()
	}
	agentRaw, ok := raw["agent_settings"]
	if !ok {
		return config.DefaultAgentConfig()
	}
	var agentCfg config.Agent
	if err := json.Unmarshal(agentRaw, &agentCfg); err != nil {
		slog.Debug("agent: getAgentConfig parse agent_settings failed", "error", err)
		return config.DefaultAgentConfig()
	}
	// 防御性修正：用户配置文件中可能缺失 mock_speed 字段（JSON 零值为 0）
	// 导致 MockEngine.Run() 中 sleepDelay 把所有 Step 延迟归零，SSE 事件
	// 毫秒级全部推送完毕，前端无法看到逐步流式效果。
	if agentCfg.MockSpeed <= 0 {
		agentCfg.MockSpeed = 1.0
	}
	return &agentCfg
}
