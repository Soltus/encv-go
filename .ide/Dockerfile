# .ide/Dockerfile
# ==========================================
# 0. 统一版本号管理
# ==========================================
ARG GO_VERSION=1.25.1
ARG G_VERSION=1.8.0
ARG PNPM_VERSION=11
ARG ANDROID_SDK_TOOLS_VERSION=13114758
ARG RUST_VERSION=stable
# Android 模拟器（无头 AVD，供 agent 做自动化"真机"测试）
ARG ANDROID_SYSIMG="system-images;android-36;google_apis;x86_64"
ARG ANDROID_AVD_NAME=encv36
ARG ANDROID_EMU_DEVICE=pixel_7

FROM soltus/default-dev-env:dev

ARG GO_VERSION
ARG G_VERSION
ARG PNPM_VERSION
ARG ANDROID_SDK_TOOLS_VERSION
ARG RUST_VERSION
ARG ANDROID_SYSIMG
ARG ANDROID_AVD_NAME
ARG ANDROID_EMU_DEVICE

# Android SDK 开发环境（encv-mobile 安卓构建所需）
# compileSdk/targetSdk=36, AGP=8.13.0, Gradle=9.0.0, Kotlin=2.3.21
ENV ANDROID_HOME="/opt/android-sdk" \
    ANDROID_SDK_ROOT="/opt/android-sdk" \
    ANDROID_SDK_MANAGER="/opt/android-sdk/cmdline-tools/latest/bin/sdkmanager" \
    ANDROID_NDK="/opt/android-sdk/ndk/latest" \
    ANDROID_NDK_ROOT="/opt/android-sdk/ndk/latest" \
    ANDROID_EMULATOR_HOME="/root/.android" \
    ANDROID_AVD_HOME="/root/.android/avd" \
    ANDROID_AVD_NAME="${ANDROID_AVD_NAME}" \
    ANDROID_SYSIMG="${ANDROID_SYSIMG}" \
    ANDROID_EMU_DEVICE="${ANDROID_EMU_DEVICE}" \
    JAVA_HOME="/usr/lib/jvm/java-21-openjdk-amd64"

# ==========================================
# 1. 基础系统与镜像源配置
# ==========================================
USER root
# 兼容 deb822（trixie/bookworm）与传统 sources.list 两种格式
RUN if [ -f /etc/apt/sources.list.d/debian.sources ]; then \
        sed -i 's|deb.debian.org|mirrors.cloud.tencent.com|g' /etc/apt/sources.list.d/debian.sources; \
        sed -i 's|security.debian.org|mirrors.cloud.tencent.com|g' /etc/apt/sources.list.d/debian.sources; \
    fi; \
    if [ -f /etc/apt/sources.list ]; then \
        sed -i 's|deb.debian.org|mirrors.cloud.tencent.com|g' /etc/apt/sources.list; \
        sed -i 's|security.debian.org|mirrors.cloud.tencent.com|g' /etc/apt/sources.list; \
    fi

# 引入 Adoptium 仓库：提供 temurin-17-jdk，规避基础镜像源缺失 openjdk-17-jdk 的问题
RUN apt-get update && \
    apt-get install -y --no-install-recommends gnupg && \
    mkdir -p /etc/apt/keyrings && \
    wget -qO - https://packages.adoptium.net/artifactory/api/gpg/key/public \
      | gpg --dearmor -o /etc/apt/keyrings/adoptium.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/adoptium.gpg] https://packages.adoptium.net/artifactory/deb $(awk -F= '/^VERSION_CODENAME/{print $2}' /etc/os-release) main" \
      > /etc/apt/sources.list.d/adoptium.list && \
    rm -rf /var/lib/apt/lists/*

# build-essential + ca-certificates：cgo 编译（pkg/ 下含 .c/.h/.a）
# cmake：scripts/setup-sandbox-env.sh 前置检查强依赖（第 121 行））
# pkg-config + libssl-dev：Rust/cgo 依赖（如 libsql 经 CGO 编译时需要的 openssl-sys）常用
# 注意：端到端测试（Cypress/Playwright）不再全局装进镜像，按需在运行端自备。
# procps：emuctl 用 pgrep 判定模拟器进程；其余 lib*：无头 Android 模拟器
# （emulator 自带 Qt/GL/swiftshader，这些是仍需系统提供的运行时库/X11 依赖）
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      curl wget git build-essential ca-certificates \
      pkg-config libssl-dev \
      cmake temurin-17-jdk \
      unzip openjdk-21-jdk jq \
      python3 \
      procps \
      libgl1 libglx-mesa0 libgl1-mesa-dri libegl1 libgbm1 \
      libnss3 libnspr4 libasound2 libpulse0 libdbus-1-3 \
      libxkbcommon-x11-0 libx11-xcb1 libxcb-cursor0 \
      libxcomposite1 libxdamage1 libxrandr2 libxtst6 libxi6 libxrender1 \
      libcups2 libfontconfig1 libfreetype6 \
    && rm -rf /var/lib/apt/lists/*
# Cypress 不在镜像安装

# 安装 code-server(VSCode WebIDE ）插件
RUN code-server --install-extension itv3.kilo-code-plus \
  && code-server --install-extension AMiner.codegeex \
  && code-server --install-extension usernamehw.errorlens \
  && code-server --install-extension shardulm94.trailing-spaces \
  && code-server --install-extension IBM.output-colorizer \
  && code-server --install-extension ruban-s.fast-import-cost \
  && code-server --install-extension Vue.volar \
  && code-server --install-extension golang.go

COPY .ide/settings.json /root/.vscode-server/data/Machine/settings.json
COPY .ide/settings.json /root/.local/share/code-server/Machine/settings.json

# ==========================================
# 1.5 Android SDK 开发环境（encv-mobile 安卓构建）
# ==========================================
# 下载并安装 commandline-tools（优先腾讯云镜像，回退 Google 官方）
RUN set -eux; \
    mkdir -p ${ANDROID_HOME}/cmdline-tools /tmp/androidsdk; \
    cd /tmp/androidsdk; \
    ( curl -fsSL "https://mirrors.cloud.tencent.com/AndroidSDK/commandlinetools-linux-${ANDROID_SDK_TOOLS_VERSION}_latest.zip" -o cmdtools.zip || \
      curl -fsSL "https://dl.google.com/android/repository/commandlinetools-linux-${ANDROID_SDK_TOOLS_VERSION}_latest.zip" -o cmdtools.zip ); \
    unzip -q cmdtools.zip -d ${ANDROID_HOME}/cmdline-tools; \
    mv ${ANDROID_HOME}/cmdline-tools/cmdline-tools ${ANDROID_HOME}/cmdline-tools/latest; \
    rm -f /tmp/androidsdk/cmdtools.zip

# JDK 17 与 21 共存：两者均安装，默认 21（AGP 8.13 + Gradle 9.0 要求 JDK 17+，本工程 jvmTarget=21），
# 17 保留供其他工具使用；用 `use-jdk 17|21` 可随时切换默认 java。
RUN set -eux; \
    update-alternatives --install /usr/bin/java java /usr/lib/jvm/temurin-17-jdk-amd64/bin/java 1711; \
    update-alternatives --install /usr/bin/javac javac /usr/lib/jvm/temurin-17-jdk-amd64/bin/javac 1711; \
    update-alternatives --install /usr/bin/java java ${JAVA_HOME}/bin/java 2111; \
    update-alternatives --install /usr/bin/javac javac ${JAVA_HOME}/bin/javac 2111; \
    update-alternatives --set java ${JAVA_HOME}/bin/java; \
    update-alternatives --set javac ${JAVA_HOME}/bin/javac; \
    printf '%s\n' \
      '#!/bin/sh' \
      'case "$1" in' \
      '  17) J=/usr/lib/jvm/temurin-17-jdk-amd64 ;;' \
      '  21) J=/usr/lib/jvm/java-21-openjdk-amd64 ;;' \
      '  *) echo "usage: use-jdk 17|21" >&2; exit 1 ;;' \
      'esac' \
      'update-alternatives --set java "$J/bin/java" >/dev/null' \
      'update-alternatives --set javac "$J/bin/javac" >/dev/null' \
      'echo "JAVA_HOME=$J"' \
      > /usr/local/bin/use-jdk; \
    chmod +x /usr/local/bin/use-jdk; \
    java -version


# 接受许可 + 安装本项目所需 SDK 包
#   platform-tools        : adb / 设备调试
#   platforms;android-36  : compileSdk/targetSdk = 36
#   build-tools;36.0.0    : AGP 8.13 构建工具
#   ndk;28.1.13356709     : gomobile 编译 libgojni.so 等原生代码所需
RUN set -eux; \
    mkdir -p ${ANDROID_HOME}/.android && \
    echo '### User Sources for Android SDK Manager' > ${ANDROID_HOME}/.android/repositories.cfg && \
    yes | ${ANDROID_SDK_MANAGER} --licenses > /dev/null || true; \
    yes | ${ANDROID_SDK_MANAGER} "platform-tools" > /dev/null; \
    yes | ${ANDROID_SDK_MANAGER} "platforms;android-36" > /dev/null; \
    yes | ${ANDROID_SDK_MANAGER} "build-tools;36.0.0" > /dev/null; \
    yes | ${ANDROID_SDK_MANAGER} "ndk;28.1.13356709" > /dev/null; \
    ln -sv ${ANDROID_HOME}/ndk/28.1.13356709 ${ANDROID_NDK}; \
    chmod -R 775 ${ANDROID_HOME}; \
    rm -rf /tmp/androidsdk

# ==========================================
# 1.6 无头 Android 模拟器（供 agent 控制做自动化"真机"测试）
# ==========================================
# 关键事实（2026-09-30 在本云原生开发容器内实测）：
#   - 容器内没有 /dev/kvm（emulator -accel-check 报
#     "KVM requires a CPU that supports vmx or svm"），因此模拟器只能软件渲染：
#     `-accel off -gpu swiftshader_indirect`（QEMU TCG）。冷启动实测：首次 ~12 分钟
#     （初始化 data 分区 + dexopt），二次 ~5 分钟；交互偏慢，但 adb 安装/点击/
#     截屏/日志全链路可用（建议会话内保持常开，别反复冷启动）。
#   - 系统镜像取 x86_64 + google_apis（API 36 = 本项目 compileSdk/targetSdk）：
#     其 build.prop 含 ro.dalvik.vm.native.bridge=libndk_translation.so，
#     能跑本项目 arm64-v8a-only 的 APK（x86_64 宿主 + TCG 比整台 arm64 模拟更快）。
#   - 若将来宿主提供 /dev/kvm，emuctl 会自动切 `-accel on -gpu auto`，无需改镜像。
#   - 换镜像：改 ANDROID_SYSIMG（如 aosp_atd / arm64-v8a）后重建，或运行期
#     `emuctl create` 重建 AVD。
# 安装 emulator + 系统镜像
RUN set -eux; \
    yes | ${ANDROID_SDK_MANAGER} --licenses > /dev/null || true; \
    yes | ${ANDROID_SDK_MANAGER} "emulator" "${ANDROID_SYSIMG}" > /dev/null; \
    ${ANDROID_HOME}/emulator/emulator -version | head -1; \
    du -sh ${ANDROID_HOME}/system-images/*/*/*

# 装好管控脚本后预创建 AVD（镜像构建期只建配置，不启动模拟器）——
# 运行期 agent 直接 `emuctl start` 即可，无需再下载镜像/建 AVD。
COPY .ide/bin/emuctl /usr/local/bin/emuctl
RUN set -eux; \
    chmod +x /usr/local/bin/emuctl; \
    mkdir -p ${ANDROID_AVD_HOME}; \
    emuctl create; \
    emuctl doctor

# ==========================================
# 2. 开发工具链安装
# ==========================================
# 2.1 g（Go 版本管理器）→ /usr/local/bin
RUN curl -sSL "https://github.com/voidint/g/releases/download/v${G_VERSION}/g${G_VERSION}.linux-amd64.tar.gz" | tar -xz -C /usr/local/bin/

# 2.2 npm / pnpm 镜像源 + pnpm 安装（前端 monorepo 用 pnpm workspace + catalog）
RUN npm config set registry https://mirrors.cloud.tencent.com/npm/ || true
RUN npm install -g pnpm@${PNPM_VERSION} && \
    pnpm config set registry https://mirrors.cloud.tencent.com/npm/ || true

# 2.3 bun（ESM-only 工具链）：可直接运行 .ts，基本可无感替代 nodejs。
#   项目脚本统一 .ts（顶部注释「用 bun 运行」），不再有 CJS/裸 node 跑脚本。
RUN npm install -g bun && bun --version

# 2.4 Rust 工具链：libsql 等 Rust 源码依赖经 CGO 编译为 Go 绑定所需。
#   走 rsproxy.cn 镜像加速 rustup 与 crates.io（国内源），并启用 sparse 协议。
ENV RUSTUP_DIST_SERVER="https://rsproxy.cn" \
    RUSTUP_UPDATE_ROOT="https://rsproxy.cn/rustup" \
    CARGO_HOME="/root/.cargo" \
    RUSTUP_HOME="/root/.rustup"

RUN set -eux; \
    ( curl --proto '=https' --tlsv1.2 -sSf https://rsproxy.cn/rustup-init.sh -o /tmp/rustup-init.sh || \
      curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs -o /tmp/rustup-init.sh ); \
    sh /tmp/rustup-init.sh -y --default-toolchain ${RUST_VERSION} --profile minimal --no-modify-path; \
    rm -f /tmp/rustup-init.sh; \
    export PATH="$CARGO_HOME/bin:$PATH"; \
    rustc --version && cargo --version

# crates.io 镜像（sparse 协议，rsproxy.cn）
RUN set -eux; \
    mkdir -p ${CARGO_HOME}; \
    printf '%s\n' \
      '[source.crates-io]' \
      'replace-with = "rsproxy-sparse"' \
      '[source.rsproxy-sparse]' \
      'registry = "sparse+https://rsproxy.cn/index/"' \
      '[registries.rsproxy]' \
      'index = "https://rsproxy.cn/crates.io-index"' \
      '[net]' \
      'git-fetch-with-cli = true' \
      > ${CARGO_HOME}/config.toml


# ==========================================
# 3. 目录预创建与外部参考仓库
# ==========================================
# deepseek-harness 仅作为长期文档/架构参考，放在临时目录，不污染协同工作区。
# 每次进入交互式 Bash/Zsh 时同步默认分支；首次为浅克隆，后续强制对齐远端 HEAD。
ENV DEEPSEEK_HARNESS_REPO="https://github.com/deepseek-ai/deepseek-harness" \
    DEEPSEEK_HARNESS_DIR="/tmp/deepseek-harness"

# 安卓真机路径：config.user.json 的 mobile.server.dir / mobile.output.path 默认指向
# /storage/emulated/0（真机存储根）。桌面开发容器里这个路径**不存在**，
# 而后端会忠实地按配置用它作为 serving_dir ⇒ 必须预创建，否则文件/媒体库为空。
#
# 纪律：**建目录，而不是改配置里的路径** —— 配置保持与真机一致，
# 避免"桌面能跑、真机路径不对"的偏差（2026-10-06 真机联调教训）。
RUN mkdir -p /storage/emulated/0/encv-output && \
    chmod 755 /storage/emulated/0

RUN mkdir -p /root/.g /root/go && \
    printf '%s\n' \
      '#!/bin/sh' \
      'set -u' \
      'repo="${DEEPSEEK_HARNESS_REPO:-https://github.com/deepseek-ai/deepseek-harness}"' \
      'target="${DEEPSEEK_HARNESS_DIR:-/tmp/deepseek-harness}"' \
      'lock="${target}.sync.lock"' \
      'waited=0' \
      'while ! mkdir "$lock" 2>/dev/null; do' \
      '  waited=$((waited + 1))' \
      '  if [ "$waited" -ge 120 ]; then' \
      '    echo "[deepseek-harness] 同步锁等待超时：$lock" >&2' \
      '    exit 0' \
      '  fi' \
      '  sleep 1' \
      'done' \
      'trap '\''rmdir "$lock" 2>/dev/null || true'\'' EXIT HUP INT TERM' \
      'if [ -d "$target/.git" ]; then' \
      '  if ! git -C "$target" fetch --depth=1 origin HEAD >/dev/null 2>&1 || ! git -C "$target" reset --hard FETCH_HEAD >/dev/null 2>&1; then' \
      '    echo "[deepseek-harness] 更新失败，保留已有副本：$target" >&2' \
      '  fi' \
      '  exit 0' \
      'fi' \
      'if [ -e "$target" ]; then' \
      '  echo "[deepseek-harness] 目标路径已存在且不是 Git 仓库：$target" >&2' \
      '  exit 0' \
      'fi' \
      'if ! git clone --depth=1 "$repo" "$target" >/dev/null 2>&1; then' \
      '  echo "[deepseek-harness] 克隆失败：$repo -> $target" >&2' \
      'fi' \
      > /usr/local/bin/sync-deepseek-harness-docs && \
    chmod +x /usr/local/bin/sync-deepseek-harness-docs

# ==========================================
# 4. 以 root 初始化运行时环境
# ==========================================
# 4.1 Go
RUN g install ${GO_VERSION} && g use ${GO_VERSION}

# ==========================================
# 5. 容器级环境变量 (关键！CI 和非交互式 shell 也继承)
# ==========================================
ENV PATH="${CARGO_HOME}/bin:${ANDROID_HOME}/cmdline-tools/latest/bin:${ANDROID_HOME}/emulator:${ANDROID_HOME}/platform-tools:${ANDROID_HOME}/tools:${ANDROID_NDK}:${JAVA_HOME}/bin:/root/.g/bin:/root/.g/go/bin:/root/go/bin:/usr/local/bin:$PATH" \
    CARGO_HOME="/root/.cargo" \
    RUSTUP_HOME="/root/.rustup" \
    G_MIRROR="https://golang.google.cn/dl/" \
    GOPATH="/root/go" \
    GOPROXY="https://goproxy.cn,direct"

# ==========================================
# 6. 注入 Shell 配置 (交互式终端兜底)
# ==========================================
RUN echo '' >> /etc/zsh/zshenv && \
    echo '# ===== Custom Dev Tools PATH =====' >> /etc/zsh/zshenv && \
    echo 'export PATH="/root/.cargo/bin:/root/.g/bin:/root/.g/go/bin:/root/go/bin:/usr/local/bin:$PATH"' >> /etc/zsh/zshenv && \
    echo 'export CARGO_HOME="/root/.cargo"' >> /etc/zsh/zshenv && \
    echo 'export RUSTUP_HOME="/root/.rustup"' >> /etc/zsh/zshenv && \
    echo 'export G_MIRROR="https://golang.google.cn/dl/"' >> /etc/zsh/zshenv && \
    echo 'export GOPATH="/root/go"' >> /etc/zsh/zshenv && \
    echo 'export GOPROXY="https://goproxy.cn,direct"' >> /etc/zsh/zshenv
RUN echo '[ -f /etc/zsh/zshenv ] && source /etc/zsh/zshenv' >> /root/.zshrc && \
    echo '/usr/local/bin/sync-deepseek-harness-docs' >> /root/.zshrc
RUN echo '#!/bin/sh' > /etc/profile.d/custom_tools.sh && \
    echo 'export PATH="/root/.cargo/bin:/root/.g/bin:/root/.g/go/bin:/root/go/bin:/usr/local/bin:$PATH"' >> /etc/profile.d/custom_tools.sh && \
    echo 'export CARGO_HOME="/root/.cargo"' >> /etc/profile.d/custom_tools.sh && \
    echo 'export RUSTUP_HOME="/root/.rustup"' >> /etc/profile.d/custom_tools.sh && \
    echo 'export GOPATH="/root/go"' >> /etc/profile.d/custom_tools.sh && \
    echo 'export GOPROXY="https://goproxy.cn,direct"' >> /etc/profile.d/custom_tools.sh && \
    chmod +x /etc/profile.d/custom_tools.sh
RUN echo '[ -f /etc/profile.d/custom_tools.sh ] && source /etc/profile.d/custom_tools.sh' >> /root/.bashrc && \
    echo '/usr/local/bin/sync-deepseek-harness-docs' >> /root/.bashrc

# ==========================================
# 7. 最终环境验收
# ==========================================
RUN echo "====================================" && \
    echo " 最终环境验证" && \
    echo "====================================" && \
    echo "PATH: $PATH" && \
    echo "node: $(node --version)" && \
    echo "bun:  $(bun --version)" && \
    echo "pnpm: $(pnpm --version)" && \
    echo "py:   $(python3 --version)" && \
    echo "cmake:$(cmake --version | head -1)" && \
    echo "java: $(java -version 2>&1 | head -1)" && \
    echo "android sdk: $ANDROID_HOME" && \
    echo "adb:  $(adb --version 2>&1 | head -1)" && \
    echo "sdkmanager: $(${ANDROID_SDK_MANAGER} --version 2>&1 | head -1)" && \
    echo "ndk:  $(readlink -f ${ANDROID_NDK})" && \
    echo "emulator: $(emulator -version 2>&1 | head -1)" && \
    echo "avd:  ${ANDROID_AVD_NAME} ($(cat ${ANDROID_AVD_HOME}/${ANDROID_AVD_NAME}.avd/config.ini 2>/dev/null | grep -m1 '^abi.type='))" && \
    echo "kvm:  $([ -c /dev/kvm ] && echo 'yes（硬件加速可用）' || echo 'no（软件渲染 TCG，启动慢）')" && \
    echo "g:    $(g --version)" && \
    echo "go:   $(go version)" && \
    echo "rustc:$(rustc --version)" && \
    echo "cargo:$(cargo --version)" && \
    echo "====================================" && \
    echo "✅ 环境准备完成"
