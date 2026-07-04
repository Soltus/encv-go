# CI Step 草案：ffmpeg-worker cross-compile for Android arm64

> **本文档不直接修改 .github/workflows/android.yml，仅作为 PR 草案。**
> 由用户 review 后再合入。

## 改动位置

文件：`.github/workflows/android.yml`
插入位置：L206（"Build Go binary for Android arm64" 步骤之后，L208 之前）

## 完整新增步骤

```yaml
      # 🆕 2026-06-11 Phase 2: ffmpeg-worker subprocess for cgo isolation
      #
      # Why:
      #   之前真机 in-process cgo dlopen libffmpeg.so 调用阻塞 OS thread，
      #   父进程 ctx cancel 不能打断 in-flight cgo call，gin handler 永远
      #   hang → 前端 spinner forever。
      #
      # Now:
      #   worker binary（libffmpeg-worker.so）作为独立 OS 进程启，
      #   内部 cgo dlopen libffmpeg.so。父进程 ctx cancel 时 Go
      #   exec.CommandContext 默认 SIGKILL worker（Go 1.20+），父进程
      #   立即 unblock。
      #
      # 跟 libopenlist.so / libencv-go.so 一个模式：cross-compile 出
      # ARM aarch64 ELF executable，AGP jniLibs 打包（只接 .so 后缀），
      # Kotlin EncvGoService.kt 启动时 ENCV_FFMPEG_WORKER 指向
      # nativeLibraryDir/libffmpeg-worker.so。
      - name: Build ffmpeg-worker for Android arm64
        if: matrix.variant == 'arm64'  # 跟 libencv-go.so 一样只编 arm64
        env:
          CGO_ENABLED: "1"
          GOOS: android
          GOARCH: arm64
          # 跟 L198 libencv-go.so 同一个 NDK clang
          CC: ${{ env.ANDROID_NDK_HOME }}/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android24-clang
          CXX: ${{ env.ANDROID_NDK_HOME }}/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android24-clang++
        run: |
          go build \
            -ldflags="-s -w" \
            -o ffmpeg-worker-arm64 \
            ./cmd/ffmpeg-worker
          # 改名 .so 是为了过 AGP jniLibs 打包（只接 .so 后缀）
          # 跟 libopenlist.so 模式完全一致
          mkdir -p app/encv-mobile/android/app/src/main/jniLibs/arm64-v8a
          cp ffmpeg-worker-arm64 \
             app/encv-mobile/android/app/src/main/jniLibs/arm64-v8a/libffmpeg-worker.so
          # 验证产物是 ARM aarch64 + executable（跟 libopenlist.so L617 一样）
          file app/encv-mobile/android/app/src/main/jniLibs/arm64-v8a/libffmpeg-worker.so
          # 应输出: ELF 64-bit LSB pie executable, ARM aarch64, ...
          # 如果是 shared object 报错：检查 CC 是否被 CGO 正确用上

      # 跟现有 libencv-go.so / libffmpeg.so / libopenlist.so 同步打到
      # jniLibs（jniLibs.srcDirs("src/main/jniLibs") 见 build.gradle.kts L77）。
      # 不需要额外步骤，AGP 自动 pack 进 APK。
```

## 注意事项

1. **AGP jniLibs 打包冲突**：如果 `libffmpeg-worker.so` 跟 `libffmpeg.so` / `libencv-go.so` 重名会冲突。命名按"`lib<name>.so`"模板已无冲突。

2. **APK 体积增加**：worker binary 是 Go runtime + cgo stub，arm64 strip 后 ~3.2 MB（沙箱实测），跟 libencv-go.so 同一个量级。release 模式可用 `-trimpath -ldflags="-s -w"` 进一步压缩。

3. **CI 验证强度**：
   - `file` 命令验证 ELF 类型（必须 executable 不是 shared object）
   - 可选加：`ldd libffmpeg-worker.so` 看依赖（应只依赖 libc/libdl/libstdc++）
   - 可选加：在 arm64 emulator 上 `./libffmpeg-worker` + 空 stdin → 应返回 `read request: invalid JSON`

4. **跟 libopenlist.so 模式一致性**：CI L597-L619 是 OpenList fork 的同模式 step，参考其完整模式（包括 `set -e` 严格、file 验证、备份等）。

5. **不要直接 adb push**：`adb push libffmpeg-worker.so /data/local/tmp && adb shell /data/local/tmp/libffmpeg-worker` 在 Android 10+ 设备 /data/local/tmp 通常 noexec。**必须走 APK 打包**（jniLibs 解包到 nativeLibraryDir 后可 exec，跟 libencv-go.so 实测一致）。

## 不在本文档范围

- ✅ 已实施：Go worker 代码（main_exec.go / main_android.go）
- ✅ 已实施：worker_runner.go / worker_client.go 去 build tag
- ✅ 已实施：native_runner.go init() WorkerRunner 优先
- ✅ 已实施：Kotlin EncvGoService.kt 加 ENCV_FFMPEG_WORKER env
- ✅ 已实施：前端 AutomationTestsDetail.vue 加 classifyMockError
- ⏳ 待 review + 合并：本文档的 CI step
- ⏳ 真机验证项：
  - adb logcat 抓 worker 启动 + SIGKILL 行为
  - 真机 10 并发 mock generate byte-exact
  - 真机 cgo hang 场景（如果有）→ 父进程 30s abort 是否生效
