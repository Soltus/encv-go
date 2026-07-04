#!/usr/bin/env bash
# targets/ios.sh - iOS target STUB（未实现）
#
# 用途：占位 + 明确报错。苹果工具链 (xcode-select / xcrun --sdk iphoneos) 与
# Android NDK 差异大（不允许 LGPL 动态链接 / 强制 bitcode 时代已结束但仍要
# embed bitcode=false / 需要 --extra-cflags 加 -arch arm64 等），独立 PR 再做。
#
# 进度跟踪：见 /workspace/.trae/documents/phase4-ffmpeg-hang-fix-and-codec-completion.md

set -euo pipefail

target_ios_setup() {
    die "iOS target not implemented yet. Use TARGET=host or TARGET=android for now."
}
