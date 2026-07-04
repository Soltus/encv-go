# ffmpeg-builder Makefile
# 跨平台统一入口：make <target>
#
# 用法：
#   make host                    # 在宿主机（linux/macOS）编译（sanity check / 烟雾测试）
#   make android                 # 交叉编译到 Android arm64-v8a + API 24
#   make android ANDROID_ABI=x86_64
#   make clean                   # 清理 build/ffmpeg/
#   make verify                  # 验证已编译产物
#   make lint                    # bash -n 全套脚本
#   make help                    # 显示帮助
#
# 设计：
#   - 所有路径走 git rev-parse（不需要 /workspace 这种硬编码）
#   - 每个 target 是独立的 .PHONY 目标，make android 失败不会污染 host
#   - 强制并行的 j 数：$(nproc) （host）/ NDK 默认（android）
#
# 设计来源：vibe-encoder/ffmpeg-builder，2026-06-13 重构自单脚本 build-ffmpeg-android.sh

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# === 全局配置 ===
BUILDER_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
PROJECT_DIR := $(abspath $(BUILDER_DIR)/../..)
ROOT_DIR    := $(abspath $(PROJECT_DIR)/..)
MANIFEST    := $(BUILDER_DIR)/../ffmpeg-feature-manifest.json

# === Android 配置（可用环境变量覆盖） ===
TARGET ?= android
ANDROID_ABI ?= arm64-v8a
ANDROID_API ?= 24

# === 入口脚本（一个 .sh 串整条链） ===
ENTRY_SH := $(BUILDER_DIR)/.entry.sh

# === help ===
.PHONY: help
help:
	@echo "ffmpeg-builder - 跨平台 ffmpeg 编译工具集"
	@echo ""
	@echo "Targets:"
	@echo "  host                       编译到宿主机（sanity check）"
	@echo "  android                    交叉编译到 Android ($(ANDROID_ABI) + API $(ANDROID_API))"
	@echo "  clean                      清理所有 build/ffmpeg/"
	@echo "  verify                     验证已编译产物的导出符号"
	@echo "  lint                       bash -n 全套脚本"
	@echo "  help                       显示本帮助"
	@echo ""
	@echo "环境变量（覆盖默认值）:"
	@echo "  ANDROID_ABI=arm64-v8a|armeabi-v7a|x86_64|x86"
	@echo "  ANDROID_API=24|..."
	@echo "  ANDROID_NDK_HOME=/path/to/ndk/26.x"
	@echo "  ANDROID_HOME=/path/to/android-sdk"
	@echo "  NO_COLOR=1                 关闭彩色日志"
	@echo ""
	@echo "示例："
	@echo "  make host"
	@echo "  make android"
	@echo "  make android ANDROID_ABI=x86_64 ANDROID_API=24"
	@echo "  make clean"

# === host target ===
.PHONY: host
host:
	@echo ">>> Building for host ($(shell uname -s)-$(shell uname -m))"
	@TARGET=host OUT_DIR=$(OUT_DIR) \
	  bash -c 'source $(BUILDER_DIR)/common/paths.sh && ffmpeg_builder_init && source $(BUILDER_DIR)/targets/dispatcher.sh && target_setup && source $(BUILDER_DIR)/common/manifest.sh && load_manifest && source $(BUILDER_DIR)/deps/dispatcher.sh && build_all_deps && source $(BUILDER_DIR)/ffmpeg.sh && build_ffmpeg && source $(BUILDER_DIR)/postprocess.sh && build_fftools'

# === android target ===
.PHONY: android
android:
	@echo ">>> Cross-compiling for Android $(ANDROID_ABI) + API $(ANDROID_API)"
	@TARGET=android ANDROID_ABI=$(ANDROID_ABI) ANDROID_API=$(ANDROID_API) OUT_DIR=$(OUT_DIR) \
	  bash -c 'source $(BUILDER_DIR)/common/paths.sh && ffmpeg_builder_init && source $(BUILDER_DIR)/targets/dispatcher.sh && target_setup && source $(BUILDER_DIR)/common/manifest.sh && load_manifest && source $(BUILDER_DIR)/deps/dispatcher.sh && build_all_deps && source $(BUILDER_DIR)/ffmpeg.sh && build_ffmpeg && source $(BUILDER_DIR)/postprocess.sh && build_fftools && source $(BUILDER_DIR)/worker.sh && build_worker'

# === show-output-dir: 打印 ffmpeg-builder 内部的 OUTPUT_LIB_DIR（绝对路径）===
# 设计目的：caller (CI workflow) 不重新发明命名规则，直接查权威路径。
# 零硬编码：内部走 paths.sh 的 resolve_output_lib_dir，跟 targets/* 调的是同一个函数。
# 不调 target_setup → 不依赖 NDK 探测（host target 也支持）。
# 用法：
#   make show-output-dir                                # 默认 android / arm64-v8a
#   make show-output-dir ANDROID_ABI=arm64-v8a
#   make show-output-dir TARGET=host                    # host target 也支持
#   make show-output-dir OUT_DIR=/some/caller/path      # caller 覆盖时，返回的就是 OUT_DIR
.PHONY: show-output-dir
show-output-dir:
	@TARGET=$(TARGET) ANDROID_ABI=$(ANDROID_ABI) OUT_DIR=$(OUT_DIR) \
	  bash -c 'source $(BUILDER_DIR)/common/paths.sh && ffmpeg_builder_init >/dev/null && resolve_output_lib_dir'

# === ios (stub) ===
.PHONY: ios
ios:
	@echo ">>> iOS target not implemented yet"
	@exit 1

# === clean ===
.PHONY: clean
clean:
	@echo ">>> Cleaning $(PROJECT_DIR)/build/ffmpeg/"
	@rm -rf "$(PROJECT_DIR)/build/ffmpeg"
	@echo "✅ cleaned"

# === verify ===
.PHONY: verify
verify:
	@echo ">>> Verifying built artifacts in $(PROJECT_DIR)/build/ffmpeg/"
	@find "$(PROJECT_DIR)/build/ffmpeg" -name "libffmpeg.so" -o -name "libffprobe.so" 2>/dev/null | while read lib; do \
		echo "  $$lib: $$(ls -lh $$lib | awk '{print $$5}')"; \
		nm -D "$$lib" 2>/dev/null | grep -E " T (ffmpeg_run|ffprobe_run|ffmpeg_reset|ffprobe_reset)$$" | head -5; \
		echo ""; \
	done || echo "  (no artifacts found)"

# === lint（bash -n 全套） ===
.PHONY: lint
lint:
	@echo ">>> Linting all .sh scripts"
	@cd $(BUILDER_DIR) && for f in $$(find . -name "*.sh" -type f); do \
		if bash -n "$$f" 2>/dev/null; then \
			echo "  ✅ $$f"; \
		else \
			echo "  ❌ $$f"; \
			bash -n "$$f"; \
			exit 1; \
		fi; \
	done
	@echo "✅ all scripts pass bash -n"
