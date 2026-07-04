// Package testguard 在 Go test binary 启动时强制执行"必须经 scripts/test-go.sh 调用"的守卫。
//
// =====================================================
// 背景（test-orchestration-defense）：
//   - scripts/test-go.sh 已有 bash 层守卫（拦截多包 ./... 模式）
//   - 但 bash 守卫对"裸 go test ./internal/xxx"完全无效（直接绕过）
//   - 用户多次反馈："go 完整测试拦截似乎没有考虑到所有调用方式，没有拦截你刚才"
//   - 必须从 Go test binary 进程内拦截（init() 钩子）才能 100% 兜底
//
// 工作原理：
//   - Go test binary 启动时，import 的每个包的 init() 都会执行
//   - 如果某个测试包 import（blank import）了本包，本包的 init() 会强制检查：
//       1. ENCV_TEST_INVOKED_BY 是否有值（scripts/test-go.sh 设置）
//       2. 是否在 CI 环境（CI=true / GITHUB_ACTIONS=true）
//       3. 是否用户显式 bypass（ENCV_TEST_BYPASS_GUARD=1）
//   - 都不满足 → os.Exit(64)（与 test-go.sh 守卫的 EX_USAGE 一致）
//
// 接入方式：
//   - 在 internal/testutil（已被多个测试包 import）加 blank import：
//       import _ "github.com/Soltus/encv-go/internal/testguard"
//   - 这样任何 import testutil 的测试 binary 都会激活守卫
//   - 不 import testutil 的包暂时不在守卫覆盖范围（接受限制）
//
// bypass 方式（仅紧急情况）：
//   - ENCV_TEST_BYPASS_GUARD=1 go test ./...（绕过守卫，直接执行）
//   - 推荐改用：bash scripts/test-go.sh ./internal/<pkg>
//
// 创建：2026-06-17（test-orchestration-defense 第 3 层加固）
// =====================================================
package testguard

import (
	"fmt"
	"os"
	"strings"
)

const (
	// EnvKeyInvokedBy 是 scripts/test-go.sh 调用 go test 前 export 的标记。
	// 任何"经 test-go.sh 启动的 go test 进程"都会带这个 env var。
	EnvKeyInvokedBy = "ENCV_TEST_INVOKED_BY"

	// EnvKeyBypass 是紧急 bypass 开关（仅限调试）。
	// 设置为 "1" 可绕过本守卫直接执行（CLI 警告：仅限紧急调试）。
	EnvKeyBypass = "ENCV_TEST_BYPASS_GUARD"
)

func init() {
	// 0) 判断是否在 Go test binary 上下文中：
	//    - os.Args[0] 包含 ".test"（如 detector.test）→ go test 标准模式
	//    - os.Args 含 -test.* flag（如 -test.run / -test.v）→ 任何含 test flag 的进程
	//    - os.Args[0] 包含 ".test."（如 detector.test.new）→ `go test -c -o` 产物
	//    三者满足其一即视为 test binary
	isTestBinary := false
	if len(os.Args) > 0 {
		exe := os.Args[0]
		if strings.Contains(exe, ".test") {
			isTestBinary = true
		} else {
			for _, a := range os.Args[1:] {
				if strings.HasPrefix(a, "-test.") {
					isTestBinary = true
					break
				}
			}
		}
	}
	if !isTestBinary {
		return // production binary（如 go run / go build 产物）→ 放行
	}

	// 1) 用户显式 bypass
	if os.Getenv(EnvKeyBypass) == "1" {
		return
	}

	// 2) 守卫主逻辑：必须经 scripts/test-go.sh 调用
	//    注意：早期版本放过 CI=true / GITHUB_ACTIONS=true，但沙箱内 CI=true
	//    是开发环境而非真实 GitHub Actions，会被绕过 → 改为强制要求 ENCV_TEST_INVOKED_BY
	//    真实 CI workflow 也走 scripts/test-go.sh，会自动 export 该 env var
	invokedBy := os.Getenv(EnvKeyInvokedBy)
	if invokedBy == "" {
		fmt.Fprintf(os.Stderr, `
╔════════════════════════════════════════════════════════════════╗
║  [test-guard] ❌ 拦截裸 'go test' 调用！                       ║
╠════════════════════════════════════════════════════════════════╣
║                                                                ║
║  检测到 Go test binary 启动时缺少 ${%s} 环境变量。              ║
║  这意味着你正在裸调 go test，绕过了 scripts/test-go.sh 的       ║
║  守卫（HARD_TIMEOUT / pre-flight / fail-fast / log rotation）。  ║
║                                                                ║
║  ⚠️  风险：                                                    ║
║    - 沙箱内多包跑动辄 380s+，耗尽网络配额触发断网                ║
║    - server 包单跑就 376s，多包串起来可能 OOM kill              ║
║    - 缺少 log rotation / pre-flight 清理 / 失败早停             ║
║                                                                ║
║  ✅  正确调用方式：                                              ║
║    单包：bash scripts/test-go.sh ./internal/<pkg>               ║
║    全包：bash scripts/test-all-go.sh                            ║
║    紧急：ENCV_TEST_BYPASS_GUARD=1 go test ./internal/<pkg>      ║
║                                                                ║
╚════════════════════════════════════════════════════════════════╝
`, EnvKeyInvokedBy)
		os.Exit(64) // EX_USAGE — 与 test-go.sh bash 守卫一致
	}
}
