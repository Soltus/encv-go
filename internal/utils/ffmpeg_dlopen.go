//go:build android

package utils

/*
#cgo LDFLAGS: -ldl

#include <dlfcn.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <pthread.h>

typedef int (*run_func_t)(int argc, char **argv);
typedef void (*reset_func_t)(void);

static pthread_mutex_t g_mutex = PTHREAD_MUTEX_INITIALIZER;

static char g_dlerror[1024] = {0};

void *g_ffmpeg_handle = NULL;
void *g_ffprobe_handle = NULL;

static int call_native_run_cached(
    void **cached_handle,
    const char *lib_path,
    const char *run_sym,
    const char *reset_sym,
    int argc,
    char **argv,
    const char *stdout_file,
    const char *stderr_file
) {
    pthread_mutex_lock(&g_mutex);
    g_dlerror[0] = '\0';

    if (!*cached_handle) {
        dlerror();
        void *h = dlopen(lib_path, RTLD_NOW | RTLD_LOCAL);
        if (!h) {
            const char *err = dlerror();
            if (err) snprintf(g_dlerror, sizeof(g_dlerror), "%s", err);
            else snprintf(g_dlerror, sizeof(g_dlerror), "dlopen failed: unknown error");
            pthread_mutex_unlock(&g_mutex);
            return -1;
        }
        *cached_handle = h;
    }

    reset_func_t reset_fn = (reset_func_t)dlsym(*cached_handle, reset_sym);
    if (reset_fn) reset_fn();

    run_func_t run_fn = (run_func_t)dlsym(*cached_handle, run_sym);
    if (!run_fn) {
        const char *err = dlerror();
        if (err) snprintf(g_dlerror, sizeof(g_dlerror), "%s", err);
        else snprintf(g_dlerror, sizeof(g_dlerror), "symbol %s not found", run_sym);
        pthread_mutex_unlock(&g_mutex);
        return -2;
    }

    int saved_stdout = -1, saved_stderr = -1;

    if (stdout_file) {
        fflush(stdout);
        saved_stdout = dup(STDOUT_FILENO);
        int fd = open(stdout_file, O_WRONLY | O_CREAT | O_TRUNC, 0644);
        if (fd >= 0) {
            dup2(fd, STDOUT_FILENO);
            close(fd);
        }
    }
    if (stderr_file) {
        fflush(stderr);
        saved_stderr = dup(STDERR_FILENO);
        int fd = open(stderr_file, O_WRONLY | O_CREAT | O_TRUNC, 0644);
        if (fd >= 0) {
            dup2(fd, STDERR_FILENO);
            close(fd);
        }
    }

    int ret = run_fn(argc, argv);

    if (saved_stdout >= 0) {
        fflush(stdout);
        dup2(saved_stdout, STDOUT_FILENO);
        close(saved_stdout);
    }
    if (saved_stderr >= 0) {
        fflush(stderr);
        dup2(saved_stderr, STDERR_FILENO);
        close(saved_stderr);
    }

    pthread_mutex_unlock(&g_mutex);
    return ret;
}

static const char* get_dlerror(void) {
    return g_dlerror;
}
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"
)

// 平台无关的 libDir 状态（getLibDir/GetLibDir）在 libdir.go 定义（无 build tag，
// 沙箱编译时也可见——ffmpeg 包在 !android build 也 import utils）。

type NativeErrorType int

const (
	NativeErrorDlopen NativeErrorType = iota
	NativeErrorSymbol NativeErrorType = iota
	NativeErrorExit   NativeErrorType = iota
)

type NativeError struct {
	Type     NativeErrorType
	Detail   string
	ExitCode int
}

func (e *NativeError) Error() string {
	switch e.Type {
	case NativeErrorDlopen:
		return fmt.Sprintf("library load failed: %s", e.Detail)
	case NativeErrorSymbol:
		return fmt.Sprintf("symbol not found: %s", e.Detail)
	case NativeErrorExit:
		return fmt.Sprintf("exit code %d: %s", e.ExitCode, e.Detail)
	default:
		return e.Detail
	}
}

type NativeResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func CallFFmpegNative(args []string) (*NativeResult, error) {
	libDir := getLibDir()
	if libDir == "" {
		return nil, &NativeError{Type: NativeErrorDlopen, Detail: "ENCV_LIB_DIR not set"}
	}

	libPath := filepath.Join(libDir, "libffmpeg.so")

	cLibPath := C.CString(libPath)
	defer C.free(unsafe.Pointer(cLibPath))

	cRunSym := C.CString("ffmpeg_run")
	defer C.free(unsafe.Pointer(cRunSym))

	cResetSym := C.CString("ffmpeg_reset")
	defer C.free(unsafe.Pointer(cResetSym))

	fullArgs := make([]string, len(args)+1)
	fullArgs[0] = "ffmpeg"
	copy(fullArgs[1:], args)

	argc := C.int(len(fullArgs))
	argv := make([]*C.char, len(fullArgs)+1)
	for i, arg := range fullArgs {
		argv[i] = C.CString(arg)
		defer C.free(unsafe.Pointer(argv[i]))
	}
	argv[len(fullArgs)] = nil

	stderrFile, err := os.CreateTemp("", "ffmpeg_stderr_*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	stderrPath := stderrFile.Name()
	stderrFile.Close()
	defer os.Remove(stderrPath)

	cStderrPath := C.CString(stderrPath)
	defer C.free(unsafe.Pointer(cStderrPath))

	ret := C.call_native_run_cached(&C.g_ffmpeg_handle, cLibPath, cRunSym, cResetSym, argc, &argv[0], nil, cStderrPath)

	stderrData, _ := os.ReadFile(stderrPath)

	result := &NativeResult{
		ExitCode: int(ret),
		Stderr:   string(stderrData),
	}

	if ret == -1 {
		dlErr := C.GoString(C.get_dlerror())
		return result, &NativeError{Type: NativeErrorDlopen, Detail: fmt.Sprintf("%s: %s", libPath, dlErr)}
	}
	if ret == -2 {
		dlErr := C.GoString(C.get_dlerror())
		return result, &NativeError{Type: NativeErrorSymbol, Detail: fmt.Sprintf("ffmpeg_run in %s: %s", libPath, dlErr)}
	}

	return result, nil
}

func CallFFprobeNative(args []string) (*NativeResult, error) {
	libDir := getLibDir()
	if libDir == "" {
		return nil, &NativeError{Type: NativeErrorDlopen, Detail: "ENCV_LIB_DIR not set"}
	}

	libPath := filepath.Join(libDir, "libffprobe.so")

	cLibPath := C.CString(libPath)
	defer C.free(unsafe.Pointer(cLibPath))

	cRunSym := C.CString("ffprobe_run")
	defer C.free(unsafe.Pointer(cRunSym))

	cResetSym := C.CString("ffprobe_reset")
	defer C.free(unsafe.Pointer(cResetSym))

	fullArgs := make([]string, len(args)+1)
	fullArgs[0] = "ffprobe"
	copy(fullArgs[1:], args)

	argc := C.int(len(fullArgs))
	argv := make([]*C.char, len(fullArgs)+1)
	for i, arg := range fullArgs {
		argv[i] = C.CString(arg)
		defer C.free(unsafe.Pointer(argv[i]))
	}
	argv[len(fullArgs)] = nil

	stdoutFile, err := os.CreateTemp("", "ffprobe_stdout_*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	stdoutPath := stdoutFile.Name()
	stdoutFile.Close()
	defer os.Remove(stdoutPath)

	cStdoutPath := C.CString(stdoutPath)
	defer C.free(unsafe.Pointer(cStdoutPath))

	stderrFile, err := os.CreateTemp("", "ffprobe_stderr_*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	stderrPath := stderrFile.Name()
	stderrFile.Close()
	defer os.Remove(stderrPath)

	cStderrPath := C.CString(stderrPath)
	defer C.free(unsafe.Pointer(cStderrPath))

	ret := C.call_native_run_cached(&C.g_ffprobe_handle, cLibPath, cRunSym, cResetSym, argc, &argv[0], cStdoutPath, cStderrPath)

	stdoutData, _ := os.ReadFile(stdoutPath)
	stderrData, _ := os.ReadFile(stderrPath)

	result := &NativeResult{
		ExitCode: int(ret),
		Stdout:   string(stdoutData),
		Stderr:   string(stderrData),
	}

	if ret == -1 {
		dlErr := C.GoString(C.get_dlerror())
		return result, &NativeError{Type: NativeErrorDlopen, Detail: fmt.Sprintf("%s: %s", libPath, dlErr)}
	}
	if ret == -2 {
		dlErr := C.GoString(C.get_dlerror())
		return result, &NativeError{Type: NativeErrorSymbol, Detail: fmt.Sprintf("ffprobe_run in %s: %s", libPath, dlErr)}
	}

	return result, nil
}

func CheckFFmpegAvailable() (ffmpegOk bool, ffprobeOk bool, errMsg string, ffmpegDetail string, ffprobeDetail string) {
	libDir := getLibDir()
	if libDir == "" {
		return false, false, "ENCV_LIB_DIR not set", "", ""
	}

	ffmpegPath := filepath.Join(libDir, "libffmpeg.so")
	ffprobePath := filepath.Join(libDir, "libffprobe.so")

	var ffmpegErr, ffprobeErr string
	ffmpegOk, ffmpegErr = checkLibAvailable(ffmpegPath, "ffmpeg_run")
	ffprobeOk, ffprobeErr = checkLibAvailable(ffprobePath, "ffprobe_run")
	ffmpegDetail = ffmpegErr
	ffprobeDetail = ffprobeErr

	if !ffmpegOk && !ffprobeOk {
		errMsg = "ffmpeg and ffprobe libraries not available"
	} else if !ffmpegOk {
		errMsg = "ffmpeg library not available"
	} else if !ffprobeOk {
		errMsg = "ffprobe library not available"
	}

	return
}

func checkLibAvailable(libPath, symbol string) (bool, string) {
	cLibPath := C.CString(libPath)
	defer C.free(unsafe.Pointer(cLibPath))

	C.dlerror()
	handle := C.dlopen(cLibPath, C.RTLD_NOW|C.RTLD_LOCAL)
	if handle == nil {
		err := C.GoString(C.dlerror())
		return false, fmt.Sprintf("dlopen failed: %s", err)
	}

	cSymbol := C.CString(symbol)
	defer C.free(unsafe.Pointer(cSymbol))

	sym := C.dlsym(handle, cSymbol)
	if sym == nil {
		err := C.GoString(C.dlerror())
		C.dlclose(handle)
		return false, fmt.Sprintf("symbol '%s' not found: %s", symbol, err)
	}
	C.dlclose(handle)
	return true, ""
}
