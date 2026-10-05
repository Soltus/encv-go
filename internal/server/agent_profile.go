package server

// agent_profile.go —— 运行 profile 与 mock/synthetic 门禁（vNext 规格 Phase 0）
//
// 背景（用户原话）：现有演示是"自欺欺人的垃圾剧本"。根因之一是 mock 剧本、
// 预设 chip 与真实 OpenAI/工具调用共用同一条生产路由，只要配置里写了
// mock_mode=builtin，整条链就短路成剧本回放，而 UI 与文档都曾把它当成"能力已实现"。
//
// 本文件把这条边界变成代码强制约束：
//   - production（默认）：mock 恒为 off，剧本目录不加载，mock_resume 直接 404
//   - test：仅在显式 ENCV_RUNTIME_PROFILE=test 时启用，供评估/演示/故障注入，
//           且其证据必须标记 synthetic，不得当作生产验收（见 internal/evidence）

import (
	"log/slog"
	"os"
	"strings"

	"github.com/Soltus/encv-go/internal/config"
)

// 运行 profile。
const (
	// ProfileProduction 生产 profile（默认，fail closed）。
	// 禁止一切 mock 短路与剧本；只允许真实 Provider、真实工具与真实设备回执。
	ProfileProduction = "production"
	// ProfileTest 测试 / 评估 profile。
	// 允许 mock 剧本、Replay 与故障注入，但产生的是 synthetic 证据。
	ProfileTest = "test"
)

// runtimeProfileFromEnv 从环境变量解析运行 profile。
//
// 未显式声明、或声明为未知值时一律 production —— 绝不因拼错而意外开启 mock。
func runtimeProfileFromEnv() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ENCV_RUNTIME_PROFILE"))) {
	case ProfileTest, "testing", "dev":
		return ProfileTest
	default:
		return ProfileProduction
	}
}

// RuntimeProfile 返回当前运行 profile；零值按 production 处理（fail closed）。
func (s *Server) RuntimeProfile() string {
	if s.runtimeProfile == "" {
		return ProfileProduction
	}
	return s.runtimeProfile
}

// mockEnabled 报告当前 profile 是否允许 mock/synthetic 演示路径。
func (s *Server) mockEnabled() bool { return s.RuntimeProfile() == ProfileTest }

// effectiveMockMode 返回实际生效的 mock 模式。
//
// production 下恒为 "off"：即便用户配置写了 builtin/custom，也只警告并忽略，
// 绝不让演示剧本冒充真实能力。这是 AGT-004 的第一道强制门禁。
func (s *Server) effectiveMockMode(cfg *config.Agent) string {
	mode := strings.ToLower(strings.TrimSpace(cfg.MockMode))
	if mode == "" {
		mode = "off"
	}
	if s.RuntimeProfile() == ProfileProduction {
		if mode != "off" {
			slog.Warn("agent: mock_mode ignored in production profile",
				"configured", mode, "profile", s.RuntimeProfile())
		}
		return "off"
	}
	return mode
}
