// cmd/ffmpeg-worker/ffmpeg_worker.c
//
// 完整可编译，无任何外部依赖，兼容 Go 1.25+ 所有问题
// 兼容 NDK clang（ndk-sysroot、setresuid musl 报错）+ glibc 桌面
//
// 协议（stdin JSON 收请求，stdout JSON 发响应）：
//
//   请求：
//     {
//       "args": ["-i", "in.mp4", "-c", "copy", "out.mp4"],
//       "lib_dir": "/data/data/com.encvgo/files",
//       "timeout_ms": 5000,
//       "mode": "ffmpeg"  // 或 "ffprobe"
//     }
//
//   响应：
//     {
//       "exit_code": 0,
//       "stdout": "<完整 stdout，含 ffprobe JSON>",
//       "stderr": "<完整 stderr，含 ffmpeg log/ffprobe stderr>",
//       "duration_ms": 1234,
//       "error": ""  // 仅失败时填
//     }
//
// 🆕 2026-06-15 重构：
//   - 加 mode 字段："ffmpeg"（默认，dlsym libffmpeg.so + ffmpeg_run）
//                  | "ffprobe"（dlsym libffprobe.so + ffprobe_run）
//   - 加 stdout 捕获（ffprobe JSON 输出在 stdout；之前只重定向 stderr 会丢数据）
//   - 错误信息完整保留（之前 line 220-221 跳过非 ASCII 字节，会丢诊断信息）
//   - JSON escape 完整（`\` `"` `\n` `\t` `\r` 都转义）
//   - 错误码与 utils.CallFFprobeNative 对齐：-1 ENGINE_LOAD_FAILED / -2 ENGINE_SYMBOL_MISSING
//   - argv[0] 跟 mode 联动：ffmpeg 模式 prepend "ffmpeg"；ffprobe 模式 prepend "ffprobe"
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <dlfcn.h>
#include <signal.h>
#include <time.h>
#include <sys/time.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <errno.h>

// ========== 配置 ==========
#define MAX_ARGS        128
#define MAX_ARG_LEN     4096
#define MAX_JSON_LEN    (1 << 20)  // 1MB：支持大 ffprobe JSON 输出
#define MAX_LIB_PATH    4096
#define MAX_OUTPUT_LEN  (256 * 1024)  // 256KB stdout/stderr 上限（截断防 OOM）
#define MAX_TMP_PATH    512  // 临时文件路径（Android 真机 /data/user/<uid>/<pkg>/cache 长 ~80 字节，留足余量）

// ========== FFmpeg/FFprobe 函数指针 ==========
typedef int (*run_fn_t)(int, char**);
typedef void (*reset_fn_t)(void);

// ========== 全局状态 ==========
static volatile sig_atomic_t g_timeout_triggered = 0;
static char g_lib_dir[MAX_LIB_PATH] = {0};

// ========== 超时处理（硬超时） ==========
static void timeout_sig_handler(int sig) {
    (void)sig;
    g_timeout_triggered = 1;
    // 直接 _exit，不刷盘：父进程已经能根据 timeout 杀掉 worker
    _exit(124);
}

static void setup_timeout(int timeout_ms) {
    if (timeout_ms <= 0) return;
    struct sigaction sa;
    memset(&sa, 0, sizeof(sa));
    sa.sa_handler = timeout_sig_handler;
    sigaction(SIGALRM, &sa, NULL);
    // Android NDK alarm() 只支持秒级
    unsigned int seconds = (timeout_ms + 999) / 1000;
    if (seconds == 0) seconds = 1;
    alarm(seconds);
}

// ========== 极简 JSON 解析（无依赖，专门针对我们的协议） ==========
// 注意：这是单行 JSON 解析（不支持嵌套对象里同名 key 后出现前一个 key 的情况）
// 我们的协议里所有 key 只出现一次，且 args 是 string[]，其他字段是 string / int
static int json_find_string(const char* json, const char* key, char* out, int out_len) {
    char search[256];
    snprintf(search, sizeof(search), "\"%s\":\"", key);
    const char* p = strstr(json, search);
    if (!p) return -1;
    p += strlen(search);
    const char* end = strchr(p, '"');
    if (!end) return -1;
    int len = (int)(end - p);
    if (len >= out_len) len = out_len - 1;
    memcpy(out, p, (size_t)len);
    out[len] = 0;
    return 0;
}

static int json_find_int(const char* json, const char* key, int* out) {
    char search[256];
    snprintf(search, sizeof(search), "\"%s\":", key);
    const char* p = strstr(json, search);
    if (!p) return -1;
    p += strlen(search);
    // 跳过空白
    while (*p == ' ' || *p == '\t' || *p == '\n') p++;
    *out = atoi(p);
    return 0;
}

static int json_parse_args(const char* json, char*** out_args, int* out_argc, const char* mode) {
    const char* p = strstr(json, "\"args\":");
    if (!p) return -1;
    p = strchr(p, '[');
    if (!p) return -1;
    p++;

    char** args = calloc((size_t)MAX_ARGS, sizeof(char*));
    if (!args) return -1;
    int argc = 0;
    // argv[0] 跟 mode 联动
    args[argc++] = strdup(mode);

    while (*p && *p != ']' && argc < MAX_ARGS) {
        while (*p == ' ' || *p == ',' || *p == '\n' || *p == '\t' || *p == '\r') p++;
        if (*p == ']' || *p == 0) break;
        if (*p != '"') { p++; continue; }
        p++;
        // 解析单引号内 string，支持 \\ \" \n \t \r 转义
        // 先估算长度（包含转义序列展开）
        const char* start = p;
        size_t out_len = 0;
        while (*p && *p != '"') {
            if (*p == '\\' && (*(p+1) == '"' || *(p+1) == '\\' || *(p+1) == 'n' || *(p+1) == 't' || *(p+1) == 'r')) {
                p += 2;
                out_len++;
            } else {
                p++;
                out_len++;
            }
        }
        if (*p != '"') break;
        int len = (int)(p - start);
        char* arg = malloc((size_t)out_len + 1);
        if (!arg) return -1;
        // 实际复制 + 展开
        size_t j = 0;
        for (int i = 0; i < len; i++) {
            char c = start[i];
            if (c == '\\' && i + 1 < len) {
                char n = start[i+1];
                if (n == 'n') { arg[j++] = '\n'; i++; continue; }
                if (n == 't') { arg[j++] = '\t'; i++; continue; }
                if (n == 'r') { arg[j++] = '\r'; i++; continue; }
                if (n == '"' || n == '\\') { arg[j++] = n; i++; continue; }
            }
            arg[j++] = c;
        }
        arg[j] = 0;
        args[argc++] = arg;
        if (*p == '"') p++;
    }

    *out_args = args;
    *out_argc = argc;
    return 0;
}

// ========== 重定向 stdout/stderr 到临时文件（捕获 ffmpeg/ffprobe 输出） ==========
typedef struct {
    int saved_stdout;
    int saved_stderr;
    char stdout_path[MAX_TMP_PATH];
    char stderr_path[MAX_TMP_PATH];
} output_redirect_t;

// ========== 🆕 2026-06-16：真机可写 temp dir 解析 ==========
//
// 旧实现硬编码 "/tmp/ffmpeg_stdout_XXXXXX" → Android 上 /tmp/ 不存在 / 不可写 → EACCES
//   症状：mock 生成 ffmpeg 转码全部失败 "failed to create output temp files: Permission denied"
//   静态文件生成（PDF/TXT/CSV/AE/SCCGV）正常，因为它们不调 ffmpeg，直接写 mount 路径
//
// 修复：worker 启动时按以下优先级选 temp dir：
//   1. JSON 请求里的 "tmp_dir" 字段（Go 父进程从 os.TempDir() 传过来，最准确）
//   2. TMPDIR 环境变量（Go 父进程 os.Environ() 继承，gomobile 设置为 context.cacheDir）
//   3. /data/local/tmp/（Android shell temp；app 进程可能不可写，仅兜底）
//   4. "/data/user/0/com.encvgo.app/cache"（已知 app cacheDir，兜底）
//   5. 当前 cwd（如果可写）
//
// 任一尝试失败时返回 -1，让 redirect_output_start 输出可读错误（不再静默 /tmp 失败）
static char g_tmp_dir[MAX_TMP_PATH] = {0};

static int try_set_tmp_dir(const char* candidate) {
    if (candidate == NULL || candidate[0] == '\0') return -1;
    // 路径太长直接拒
    if (strlen(candidate) >= MAX_TMP_PATH - 32) return -1;  // 留 32 字节给 "ffmpeg_stdout_XXXXXX\0"
    // 必须以 / 结尾
    char buf[MAX_TMP_PATH];
    snprintf(buf, sizeof(buf), "%s", candidate);
    size_t bl = strlen(buf);
    if (bl == 0 || buf[bl - 1] != '/') {
        if (bl + 1 >= sizeof(buf)) return -1;
        buf[bl] = '/';
        buf[bl + 1] = '\0';
    }
    // mkdir -p（EEXIST 视为成功）
    if (mkdir(buf, 0700) != 0 && errno != EEXIST) {
        return -1;
    }
    // 试写：写个 test 文件
    char test_path[MAX_TMP_PATH];
    snprintf(test_path, sizeof(test_path), "%s.encv_worker_writable", buf);
    int fd = open(test_path, O_CREAT | O_WRONLY | O_TRUNC, 0600);
    if (fd < 0) {
        return -1;
    }
    close(fd);
    unlink(test_path);
    // 成功：写回 g_tmp_dir
    snprintf(g_tmp_dir, sizeof(g_tmp_dir), "%s", buf);
    return 0;
}

static void resolve_tmp_dir(const char* json_tmp_dir) {
    // 1. JSON 优先
    if (json_tmp_dir != NULL && json_tmp_dir[0] != '\0') {
        if (try_set_tmp_dir(json_tmp_dir) == 0) return;
    }
    // 2. TMPDIR 环境
    const char* env_tmp = getenv("TMPDIR");
    if (env_tmp != NULL && env_tmp[0] != '\0') {
        if (try_set_tmp_dir(env_tmp) == 0) return;
    }
    // 3. /data/local/tmp/
    if (try_set_tmp_dir("/data/local/tmp/") == 0) return;
    // 4. 已知 app cacheDir（Android shared user 0 + pkg）
    if (try_set_tmp_dir("/data/user/0/com.encvgo.app/cache/") == 0) return;
    // 5. 全部失败 → g_tmp_dir 留空 → redirect_output_start 会返回明确错误
}

static int redirect_output_start(output_redirect_t* r) {
    memset(r, 0, sizeof(*r));
    r->saved_stdout = -1;
    r->saved_stderr = -1;

    if (g_tmp_dir[0] == '\0') {
        errno = EACCES;
        return -1;
    }

    // stdout 临时文件
    snprintf(r->stdout_path, sizeof(r->stdout_path), "%sffmpeg_stdout_XXXXXX", g_tmp_dir);
    int fd_out = mkstemp(r->stdout_path);
    if (fd_out < 0) return -1;
    close(fd_out);

    // stderr 临时文件
    snprintf(r->stderr_path, sizeof(r->stderr_path), "%sffmpeg_stderr_XXXXXX", g_tmp_dir);
    int fd_err = mkstemp(r->stderr_path);
    if (fd_err < 0) {
        unlink(r->stdout_path);
        return -1;
    }
    close(fd_err);

    fflush(stdout);
    fflush(stderr);

    r->saved_stdout = dup(STDOUT_FILENO);
    r->saved_stderr = dup(STDERR_FILENO);

    int fd1 = open(r->stdout_path, O_WRONLY | O_CREAT | O_TRUNC, 0644);
    if (fd1 >= 0) {
        dup2(fd1, STDOUT_FILENO);
        close(fd1);
    }
    int fd2 = open(r->stderr_path, O_WRONLY | O_CREAT | O_TRUNC, 0644);
    if (fd2 >= 0) {
        dup2(fd2, STDERR_FILENO);
        close(fd2);
    }
    return 0;
}

static void redirect_output_end(output_redirect_t* r) {
    if (r->saved_stdout >= 0) {
        fflush(stdout);
        dup2(r->saved_stdout, STDOUT_FILENO);
        close(r->saved_stdout);
    }
    if (r->saved_stderr >= 0) {
        fflush(stderr);
        dup2(r->saved_stderr, STDERR_FILENO);
        close(r->saved_stderr);
    }
}

// 读取文件内容到 buffer，限制大小 + 截断标记
static int read_file_capped(const char* path, char* buf, int buf_size) {
    FILE* f = fopen(path, "rb");
    if (!f) {
        buf[0] = 0;
        return 0;
    }
    int n = (int)fread(buf, 1, (size_t)buf_size - 1, f);
    fclose(f);
    buf[n] = 0;
    if (n == buf_size - 1) {
        // 截断：在末尾追加 "...(truncated)"
        const char* trunc = "...(truncated)";
        int tlen = (int)strlen(trunc);
        if (n + tlen < buf_size) {
            memcpy(buf + n, trunc, (size_t)tlen);
            n += tlen;
            buf[n] = 0;
        }
    }
    return n;
}

// JSON 字符串转义：保留所有字节（含 UTF-8），仅转义 `"` `\` 控制字符
static void json_emit_string(const char* s) {
    putchar('"');
    for (const unsigned char* p = (const unsigned char*)s; *p; p++) {
        unsigned char c = *p;
        switch (c) {
        case '"':  fputs("\\\"", stdout); break;
        case '\\': fputs("\\\\", stdout); break;
        case '\n': fputs("\\n", stdout); break;
        case '\r': fputs("\\r", stdout); break;
        case '\t': fputs("\\t", stdout); break;
        case '\b': fputs("\\b", stdout); break;
        case '\f': fputs("\\f", stdout); break;
        default:
            // 控制字符（< 0x20 但非上面已处理）+ DEL(0x7F) → \uXXXX
            if (c < 0x20 || c == 0x7F) {
                printf("\\u%04x", c);
            } else {
                // 保留原字节（UTF-8 多字节字符不拆分）
                putchar((char)c);
            }
        }
    }
    putchar('"');
}

// ========== 主函数 ==========
int main(void) {
    // 1. 完整读取 stdin（JSON 请求）
    char json_buf[MAX_JSON_LEN];
    ssize_t total = 0;
    while ((size_t)total < sizeof(json_buf) - 1) {
        ssize_t n = read(STDIN_FILENO, json_buf + total, sizeof(json_buf) - 1 - (size_t)total);
        if (n <= 0) break;
        total += n;
    }
    if (total <= 0) {
        printf("{\"error\":\"empty stdin\",\"exit_code\":-1}\n");
        fflush(stdout);  // 🆕 2026-06-15：stdio 缓冲可能在 return 前丢失，加 fflush 防御
        return 1;
    }
    json_buf[total] = 0;

    // 2. 解析 mode（默认 "ffmpeg"）
    char mode[32] = "ffmpeg";
    json_find_string(json_buf, "mode", mode, sizeof(mode));

    // 3. 解析 args
    char** args = NULL;
    int argc = 0;
    if (json_parse_args(json_buf, &args, &argc, mode) != 0) {
        printf("{\"error\":\"parse args failed\",\"exit_code\":-1}\n");
        fflush(stdout);
        for (int i = 0; i < argc; i++) free(args[i]);
        free(args);
        return 1;
    }

    int timeout_ms = 0;
    char json_tmp_dir[MAX_TMP_PATH] = {0};
    json_find_string(json_buf, "lib_dir", g_lib_dir, sizeof(g_lib_dir));
    json_find_string(json_buf, "tmp_dir", json_tmp_dir, sizeof(json_tmp_dir));
    json_find_int(json_buf, "timeout_ms", &timeout_ms);

    // 3.5 🆕 2026-06-16：解析 tmp_dir（在 redirect_output_start 之前必须完成）
    //   优先级：JSON tmp_dir > TMPDIR env > /data/local/tmp/ > 已知 cacheDir
    //   旧实现硬编码 /tmp/ → Android 上 EACCES
    resolve_tmp_dir(json_tmp_dir);
    if (g_tmp_dir[0] == '\0') {
        printf("{\"error\":\"no writable temp dir (tried JSON tmp_dir, $TMPDIR, /data/local/tmp/, /data/user/0/com.encvgo.app/cache/)\",\"exit_code\":-1}\n");
        fflush(stdout);
        for (int i = 0; i < argc; i++) free(args[i]);
        free(args);
        return 1;
    }

    // 4. 设置超时
    setup_timeout(timeout_ms);

    // 5. 计时开始
    struct timeval start, end;
    gettimeofday(&start, NULL);

    // 6. dlopen 对应 .so
    char lib_path[MAX_LIB_PATH];
    const char* lib_name = (strcmp(mode, "ffprobe") == 0) ? "libffprobe.so" : "libffmpeg.so";
    if (g_lib_dir[0] == 0) {
        printf("{\"error\":\"lib_dir not set in request\",\"exit_code\":-1}\n");
        fflush(stdout);
        for (int i = 0; i < argc; i++) free(args[i]);
        free(args);
        return 1;
    }
    snprintf(lib_path, sizeof(lib_path), "%s/%s", g_lib_dir, lib_name);

    dlerror();
    void* handle = dlopen(lib_path, RTLD_NOW | RTLD_LOCAL);
    if (!handle) {
        const char* err = dlerror();
        printf("{\"error\":\"[ENGINE_LOAD_FAILED] dlopen %s: %s\",\"exit_code\":-1}\n", lib_path, err ? err : "unknown");
        fflush(stdout);
        for (int i = 0; i < argc; i++) free(args[i]);
        free(args);
        return 1;
    }

    // 7. 查找 symbol（ffmpeg_run 或 ffprobe_run）
    const char* run_sym = (strcmp(mode, "ffprobe") == 0) ? "ffprobe_run" : "ffmpeg_run";
    const char* reset_sym = (strcmp(mode, "ffprobe") == 0) ? "ffprobe_reset" : "ffmpeg_reset";

    reset_fn_t reset_fn = (reset_fn_t)dlsym(handle, reset_sym);
    if (reset_fn) reset_fn();

    run_fn_t run_fn = (run_fn_t)dlsym(handle, run_sym);
    if (!run_fn) {
        const char* err = dlerror();
        printf("{\"error\":\"[ENGINE_SYMBOL_MISSING] %s: %s\",\"exit_code\":-2}\n", run_sym, err ? err : "unknown");
        fflush(stdout);
        dlclose(handle);
        for (int i = 0; i < argc; i++) free(args[i]);
        free(args);
        return 1;
    }

    // 8. 重定向 stdout/stderr 到临时文件
    output_redirect_t redir;
    if (redirect_output_start(&redir) != 0) {
        printf("{\"error\":\"failed to create output temp files: %s\",\"exit_code\":-1}\n", strerror(errno));
        fflush(stdout);
        dlclose(handle);
        for (int i = 0; i < argc; i++) free(args[i]);
        free(args);
        return 1;
    }

    // 9. 执行（核心调用）
    int exit_code = run_fn(argc, args);

    // 10. 恢复 stdout/stderr + 读取内容
    redirect_output_end(&redir);

    char stdout_buf[MAX_OUTPUT_LEN] = {0};
    char stderr_buf[MAX_OUTPUT_LEN] = {0};
    read_file_capped(redir.stdout_path, stdout_buf, sizeof(stdout_buf));
    read_file_capped(redir.stderr_path, stderr_buf, sizeof(stderr_buf));
    unlink(redir.stdout_path);
    unlink(redir.stderr_path);

    // 11. 计算耗时
    gettimeofday(&end, NULL);
    long duration_ms = (end.tv_sec - start.tv_sec) * 1000L +
                       (end.tv_usec - start.tv_usec) / 1000L;

    // 12. 输出 JSON 响应
    // 顺序：exit_code, stdout, stderr, duration_ms, error
    // 字段顺序与 Go encode.go workerResponse 严格对齐（json.Unmarshal 按字段名匹配，顺序不严格，但便于调试）
    printf("{\"exit_code\":%d,\"stdout\":", exit_code);
    json_emit_string(stdout_buf);
    printf(",\"stderr\":");
    json_emit_string(stderr_buf);
    printf(",\"duration_ms\":%ld", duration_ms);
    if (g_timeout_triggered) {
        printf(",\"error\":\"timeout exceeded after %d ms\"", timeout_ms);
    } else {
        printf(",\"error\":\"\"");
    }
    printf("}\n");
    fflush(stdout);

    // 13. 清理
    for (int i = 0; i < argc; i++) free(args[i]);
    free(args);
    if (handle) dlclose(handle);

    return exit_code;
}
