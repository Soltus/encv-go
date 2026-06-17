#!/usr/bin/env bash
# =============================================================================
# ci-check-no-encv.sh
# -----------------------------------------------------------------------------
# 硬性 lint：禁止在主源码树中硬编码 ".encv" 后缀名（2026-06-17 加）。
#
# 背景：
#   ① ENC v4 plugin 容器扩展名由 plugin.GetContainerExtension() 动态返回
#     （权威来源：internal/v2/plugins/<plugin>/plugin.go 的 GetContainerExtension()）：
#        video    → .sccgv
#        audio    → .sccga
#        image    → .sccgi
#        text     → .sccgt
#        pdf      → .sccgpdf
#        wps      → .sccgwps
#        alist_encrypt → .bin（默认；兼容 alist 历史 .encv v2 格式）
#   ② 严禁在任何 v4 plugin 上下文中硬编码 ".encv"。
#   ③ 历史上 agent_plugin_bridge.go / task_options_test.go / SKILL.md / 前端
#     schema.json / WorkflowDashboard.vue / PluginTestsDetail.vue 都曾误写
#     ".encv" 为"v4 加密容器"——已统一改回 plugin 权威。
#
# 禁止模式（仅检测作为"文件后缀"出现的 .encv）：
#   - *.encv（任意文件名 + .encv 后缀）
#   - ".encv" / '.encv' 字符串字面量
#   - "/.encv/" 路径片段（除白名单目录外）
#
# 合法白名单（不检测）：
#   - alistencrypt plugin 目录（alist 兼容；plugin.go 第 100 行明确保留 .encv 历史格式）
#   - 命名空间前缀：.encv-tasks.json、.encv-encrypt、.encv-automation 等
#   - 隐藏目录：.encv/mounts.json、.encv/agent/、.encv/skills/、.encv/config.user.json
#   - "com.encvgo.*" / "com.test.encv"（test stub 包名）
#   - "encv-encrypt" / "encv-decrypt" / "encv-server-url" 变量与路径
#   - "ENCV" magic byte 检测（agent 4 字节检测 .encv 容器）
#   - .pnpm-store/、node_modules/、app/openlist/、ios/、android/、app/encv-mobile/dist/、.git/
#   - .trae/documents/、.trae/specs/（历史 plan/spec 文档，不动）
#   - scripts/ci-check-no-encv.sh（lint 脚本自身）
#
# 违规后果：CI 红灯（exit 1）
# =============================================================================

set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

# 1) 排除目录（不查）
EXCLUDES=(
  -path './.pnpm-store' -prune -o
  -path './node_modules' -prune -o
  -path './app/openlist' -prune -o
  -path './app/encv-mobile/dist' -prune -o
  -path './ios' -prune -o
  -path './android' -prune -o
  -path './.git' -prune -o
  -path './.trae/documents' -prune -o
  -path './.trae/specs' -prune -o
)

# 2) 文件后缀白名单（.go / .ts / .vue / .js / .md / .json / .sh / .yaml / .yml）
INCLUDE=(
  -type f \( -name '*.go' -o -name '*.ts' -o -name '*.vue' -o -name '*.js' -o -name '*.md' -o -name '*.json' -o -name '*.sh' -o -name '*.yaml' -o -name '*.yml' \)
)

# 3) 文件路径白名单（整个路径或目录）
#    以下是**合法**的 .encv 出现位置（不要检测）：
#    - alistencrypt plugin 目录：plugin.go 明确支持 .encv 历史格式
#    - agent/ 整个目录：独立 go module，模拟 alist legacy 格式
#    - internal/server/agent_*_test.go：测试 ENCV magic bytes 检测
#    - internal/mount/ 测试：.encv/mounts.json 是 ENCV 的 config 目录
#    - internal/testutil/container_ext_helper.go：helper 自身注释
#    - 任何 .encv/mounts.json / .encv/agent / .encv/skills / .encv/config.user.json
#    - 任何 .encv/ 隐藏目录
#    - logcat.txt / test_v4.txt / ci-check-no-encv.sh / .trae/{documents,specs}/
PATH_ALLOWLIST=(
  -not -path '*/alistencrypt/*'
  -not -path '*/logcat.txt'
  -not -path '*/test_v4.txt*'
  -not -path '*/ci-check-no-encv.sh'
  -not -path '*/.trae/documents/*'
  -not -path '*/.trae/specs/*'
  -not -path '*/agent/*'
  -not -path '*/internal/testutil/container_ext_helper.go'
  -not -path '*/internal/server/agent_*'
  -not -path '*/internal/mount/*'
  -not -path '*/internal/server/server.go'
)

# 4) 扫描：找包含 ".encv" 字符串字面量作为后缀的文件
#    用单个 grep -v -e chain 排除白名单
violations=0
output=$(find . "${EXCLUDES[@]}" "${INCLUDE[@]}" "${PATH_ALLOWLIST[@]}" -print0 2>/dev/null \
  | xargs -0 grep -HnE '\.encv' 2>/dev/null \
  | grep -E '(\.encv["'"'"']|\.encv[/ ]|\.encv$|\.encv$|\.encv\)|file\.encv|test\.encv|sample\..*\.encv|/[^/ ]*\.encv[^a-zA-Z0-9])' \
  | grep -v -e '.encv-' -e '.encv_' -e '.encv/' -e 'com.encvgo' -e 'com.test.encv' -e 'encv-automation' -e 'encv-server-url' -e 'encv-encrypt' -e 'encv-decrypt' -e 'encv-tasks.json' -e '.encv_heartbeat' -e '.encv-diagnose-write-test' -e '.encv_verify_' -e '.encv_tmp' -e '.encv/mounts.json' -e '.encv/agent' -e '.encv/skills' -e '.encv/config.user.json' -e '.encv 目录' -e '.encv dir' -e 'ENCVPackageName' -e 'ENCV_PACKAGE_NAME' -e 'ENCV_APP_FILES_DIR' -e 'ENCV_HEARTBEAT_PATH' -e 'EncryptConfig' -e 'EncryptService' -e 'EncryptPathV2' -e 'is_encrypted' -e 'AlistEncrypt' -e 'alist_encrypt' -e 'alistencrypt' -e 'ENCV magic' -e 'ENCVxxxx' -e 'ENCVfake' -e 'ENCV 魔数' -e 'ENCV 容器' \
  || true)

if [[ -n "$output" ]]; then
  echo "❌ ci-check-no-encv: 发现 .encv 后缀硬编码违规"
  echo "=========================================="
  echo "$output" | head -40
  echo "=========================================="
  total=$(echo "$output" | wc -l | tr -d ' ')
  echo "Total: $total violations"
  echo ""
  echo "违规说明：.encv 是 alistencrypt plugin 的历史保留扩展名。"
  echo "ENC v4 plugin 容器扩展名由 plugin.GetContainerExtension() 动态返回（plugin 源码是权威）："
  echo "  video→.sccgv, audio→.sccga, image→.sccgi, text→.sccgt, pdf→.sccgpdf, wps→.sccgwps"
  echo "  alist_encrypt 默认 .bin（兼容 alist 历史 .encv v2 格式）"
  echo ""
  echo "修复指引："
  echo "  1. 找到 plugin 真实扩展名（看 internal/v2/plugins/<plugin>/plugin.go 的 GetContainerExtension()）"
  echo "  2. 把硬编码 .encv 改成 testutil.GetTest<Plugin>ContainerExt() 调用结果"
  echo "  3. 前端从 /api/webdav/manifest 接口的 registered_container_exts 字段取"
  echo "  4. 命名空间前缀（.encv-tasks.json / .encv_heartbeat / .encv_tmp 等）合法"
  echo "  5. 隐藏目录（.encv/mounts.json / .encv/agent / .encv/skills）合法"
  echo "  6. alistencrypt plugin 目录（alist 兼容）合法"
  exit 1
fi

echo "✅ ci-check-no-encv: 0 violations"
exit 0
