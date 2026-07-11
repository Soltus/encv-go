# .ide/Dockerfile
# ==========================================
# 0. 统一版本号管理
# ==========================================
ARG GO_VERSION=1.25.1
ARG G_VERSION=1.8.0
ARG PNPM_VERSION=11

FROM cnbcool/default-dev-env:latest

ARG GO_VERSION
ARG G_VERSION
ARG PNPM_VERSION

# ==========================================
# 1. 基础系统与镜像源配置
# ==========================================
USER root
RUN sed -i 's|http://deb.debian.org/debian|https://mirrors.cloud.tencent.com/debian|g' /etc/apt/sources.list && \
    sed -i 's|http://security.debian.org/debian-security|https://mirrors.cloud.tencent.com/debian-security|g' /etc/apt/sources.list || true

# build-essential + ca-certificates：cgo 编译（pkg/ 下含 .c/.h/.a）
# cmake：scripts/setup-sandbox-env.sh 前置检查强依赖（第 121 行）
# default-jdk：同上，脚本前置检查 java + 后续 setup-kotlinc.sh 需要 JRE
# Cypress：全局安装 cypress@15（含二进制）并保留在镜像中，供运行测试直接使用；
#   系统依赖用官方命令 `cypress install --dependencies` 按当前发行版自动解析安装
#   （xvfb + GTK/ATK 等），避免硬编码冷门包（跨 Debian 版本易漂移，如
#   libasound2 → libasound2t64）。
#   注意：cypress@13+ 已废弃 `--dependencies` 选项，该选项改为交由 OS 包管理器解析，
#   故此处去掉该参数，仅安装 Cypress 二进制本体（依赖项在运行端到端测试时按需自备）。
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      curl wget git build-essential ca-certificates \
      cmake default-jdk \
      python3 \
    && npm install -g cypress@15 \
    && cypress install \
    && rm -rf /var/lib/apt/lists/*

COPY .ide/settings.json /root/.vscode-server/data/Machine/settings.json
COPY .ide/settings.json /root/.local/share/code-server/Machine/settings.json

# ==========================================
# 2. 开发工具链安装
# ==========================================
# 2.1 g（Go 版本管理器）→ /usr/local/bin
RUN curl -sSL "https://github.com/voidint/g/releases/download/v${G_VERSION}/g${G_VERSION}.linux-amd64.tar.gz" | tar -xz -C /usr/local/bin/

# 2.2 npm / pnpm 镜像源 + pnpm 安装（前端 monorepo 用 pnpm workspace + catalog）
RUN npm config set registry https://mirrors.cloud.tencent.com/npm/ || true
RUN npm install -g pnpm@${PNPM_VERSION} && \
    pnpm config set registry https://mirrors.cloud.tencent.com/npm/ || true

# ==========================================
# 3. 目录预创建
# ==========================================
RUN mkdir -p /root/.g /root/go

# ==========================================
# 4. 以 root 初始化运行时环境
# ==========================================
# 4.1 Go
RUN g install ${GO_VERSION} && g use ${GO_VERSION}

# ==========================================
# 5. 容器级环境变量 (关键！CI 和非交互式 shell 也继承)
# ==========================================
ENV PATH="/root/.g/bin:/root/.g/go/bin:/root/go/bin:/usr/local/bin:$PATH" \
    G_MIRROR="https://golang.google.cn/dl/" \
    GOPATH="/root/go" \
    GOPROXY="https://goproxy.cn,direct"

# ==========================================
# 6. 注入 Shell 配置 (交互式终端兜底)
# ==========================================
RUN echo '' >> /etc/zsh/zshenv && \
    echo '# ===== Custom Dev Tools PATH =====' >> /etc/zsh/zshenv && \
    echo 'export PATH="/root/.g/bin:/root/.g/go/bin:/root/go/bin:/usr/local/bin:$PATH"' >> /etc/zsh/zshenv && \
    echo 'export G_MIRROR="https://golang.google.cn/dl/"' >> /etc/zsh/zshenv && \
    echo 'export GOPATH="/root/go"' >> /etc/zsh/zshenv && \
    echo 'export GOPROXY="https://goproxy.cn,direct"' >> /etc/zsh/zshenv
RUN echo '[ -f /etc/zsh/zshenv ] && source /etc/zsh/zshenv' >> /root/.zshrc
RUN echo '#!/bin/sh' > /etc/profile.d/custom_tools.sh && \
    echo 'export PATH="/root/.g/bin:/root/.g/go/bin:/root/go/bin:/usr/local/bin:$PATH"' >> /etc/profile.d/custom_tools.sh && \
    echo 'export GOPATH="/root/go"' >> /etc/profile.d/custom_tools.sh && \
    echo 'export GOPROXY="https://goproxy.cn,direct"' >> /etc/profile.d/custom_tools.sh && \
    chmod +x /etc/profile.d/custom_tools.sh
RUN echo '[ -f /etc/profile.d/custom_tools.sh ] && source /etc/profile.d/custom_tools.sh' >> /root/.bashrc

# ==========================================
# 7. 最终环境验收
# ==========================================
RUN echo "====================================" && \
    echo " 最终环境验证" && \
    echo "====================================" && \
    echo "PATH: $PATH" && \
    echo "node: $(node --version)" && \
    echo "pnpm: $(pnpm --version)" && \
    echo "py:   $(python3 --version)" && \
    echo "cmake:$(cmake --version | head -1)" && \
    echo "java: $(java -version 2>&1 | head -1)" && \
    echo "g:    $(g --version)" && \
    echo "go:   $(go version)" && \
    echo "cypress: $(cypress --version)" && \
    echo "====================================" && \
    echo "✅ 环境准备完成"
