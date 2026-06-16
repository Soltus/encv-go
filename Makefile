.PHONY: encv copy-files build-all build-artifacts run clean dev-backend dev-mobile test-go test-diagnose test-self test-all-go

OUTPUT_DIR ?= dist

# 清理编译产物
clean:
	@echo "Cleaning up..."
	rm -rf $(OUTPUT_DIR)/

# 编译 encv
encv:
	@echo "Building encv..."
	go build -o $(OUTPUT_DIR)/encv ./cmd/encv

# 复制配置和文档文件
copy-files:
	@echo "Copying necessary files..."
	@mkdir -p $(OUTPUT_DIR)
	@cp config.user.json $(OUTPUT_DIR)/
	@cp README.md $(OUTPUT_DIR)/

# 编译所有程序并复制文件
build-all: encv copy-files
	@echo "All targets and files built successfully in ./$(OUTPUT_DIR)/"

# 启动后端（桌面端模式，server.dir 使用 config 中的原始值）
dev-backend:
	@echo "Starting backend (desktop mode)..."
	go run ./cmd/encv start

# 启动后端（移动端预览模式，mobile 配置段作为 overlay 自动生效）
# ENCV_MOBILE=1 或 ENCV_DEV_PREVIEW=1 均可触发 ApplyMobileOverlay
# 2026-06-10：不再预生成 mock 数据（Node CLI 脚本已删），由用户主动调后端 /api/mock/generate。
dev-mobile:
	@echo "Starting backend (mobile preview mode, no mock pre-generation)..."
	ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start

# Go 测试唯一入口（强超时链 + pre-flight 清理 + 崩溃落盘）
# 2026-06-15 创建（test-architecture-refactor-defense-awareness Sprint 1）
# 用法：
#   make test-go                                    # 跑全部 internal/...
#   make test-go PKGS=./internal/crypto/...         # 跑指定包
#   make test-go HARD_TIMEOUT=120                   # 自定义总超时
test-go:
	@bash scripts/test-go.sh

# Go 测试诊断（不修复任何代码，只收集 baseline 数据）
# 输出在 .test-runs/diagnose-<ts>/summary.txt
test-diagnose:
	@bash scripts/test-diagnose.sh

# testutil 自测（验证 SafeGo / Cleanup / Report 都正确）
test-self:
	@bash scripts/test-go.sh ./internal/testutil/... -run Test
