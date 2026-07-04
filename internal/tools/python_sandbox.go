// Stage 9 (borrow-nuclear-boy-2026q2)：Python 沙箱 4 策略 + 白/黑名单。
//
// 借鉴自 /tmp/nuclear-boy/skills/src/main/java/com/nuclearboy/skills/SandboxPolicy.kt。
//
// 4 种 SandboxMode（对应 nuclear-boy SandboxPolicy.kt L40-100）：
//   - STRICT            严格：只允许 stdlib，禁网络/子进程/危险命令
//   - STANDARD          标准：允许常见第三方库，禁网络/子进程
//   - RELAXED           宽松：允许网络，禁止 rm -rf 等破坏性命令
//   - DOCUMENT_GENERATION 文档生成：允许 docx/xlsx/pptx
//
// 落地策略：
//   - BuildPreamble 返回注入到 Python 脚本开头的代码（重写 builtins.open / 拦截 subprocess）
//   - IsStdlibModule 判断模块是否为 stdlib（白名单）
//   - IsDangerousCommand 检查黑名单（rm -rf /, mkfs., dd if=, fork bomb, chmod 777 /, curl|sh）
package tools

import (
	"strings"
)

// SandboxMode 4 种沙箱模式（对应 nuclear-boy SandboxMode enum）。
type SandboxMode int

const (
	// SandboxStrict 严格：只允许 stdlib，禁网络/子进程/危险命令。
	SandboxStrict SandboxMode = iota
	// SandboxStandard 标准：允许常见第三方库，禁网络/子进程。
	SandboxStandard
	// SandboxRelaxed 宽松：允许网络，禁止破坏性命令。
	SandboxRelaxed
	// SandboxDocumentGeneration 文档生成：允许 docx/xlsx/pptx。
	SandboxDocumentGeneration
)

// String 返回模式名。
func (m SandboxMode) String() string {
	switch m {
	case SandboxStrict:
		return "STRICT"
	case SandboxStandard:
		return "STANDARD"
	case SandboxRelaxed:
		return "RELAXED"
	case SandboxDocumentGeneration:
		return "DOCUMENT_GENERATION"
	default:
		return "UNKNOWN"
	}
}

// AllowsNetwork 是否允许网络。
func (m SandboxMode) AllowsNetwork() bool {
	return m == SandboxRelaxed
}

// AllowsSubprocess 是否允许子进程。
func (m SandboxMode) AllowsSubprocess() bool {
	return m == SandboxRelaxed || m == SandboxDocumentGeneration
}

// AllowsThirdPartyImports 是否允许第三方库。
func (m SandboxMode) AllowsThirdPartyImports() bool {
	return m != SandboxStrict
}

// ─── Stdlib 白名单 ──────────────────────────────────────────

// stdlibModules Python 3 stdlib 模块名（截至 3.12）。
// 借鉴 nuclear-boy SandboxPolicy.kt L520-555。
var stdlibModules = map[string]bool{
	"abc": true, "aifc": true, "argparse": true, "array": true,
	"ast": true, "asynchat": true, "asyncio": true, "asyncore": true,
	"atexit": true, "audioop": true, "base64": true, "bdb": true,
	"binascii": true, "binhex": true, "bisect": true, "builtins": true,
	"bz2": true, "calendar": true, "cgi": true, "cgitb": true,
	"chunk": true, "cmath": true, "cmd": true, "code": true,
	"codecs": true, "codeop": true, "collections": true, "colorsys": true,
	"compileall": true, "concurrent": true, "configparser": true, "contextlib": true,
	"contextvars": true, "copy": true, "copyreg": true, "cProfile": true,
	"crypt": true, "csv": true, "ctypes": true, "curses": true,
	"dataclasses": true, "datetime": true, "dbm": true, "decimal": true,
	"difflib": true, "dis": true, "distutils": true, "doctest": true,
	"email": true, "encodings": true, "enum": true, "errno": true,
	"faulthandler": true, "fcntl": true, "filecmp": true, "fileinput": true,
	"fnmatch": true, "formatter": true, "fractions": true, "ftplib": true,
	"functools": true, "gc": true, "getopt": true, "getpass": true,
	"gettext": true, "glob": true, "grp": true, "gzip": true,
	"hashlib": true, "heapq": true, "hmac": true, "html": true,
	"http": true, "idlelib": true, "imaplib": true, "imghdr": true,
	"imp": true, "importlib": true, "inspect": true, "io": true,
	"ipaddress": true, "itertools": true, "json": true, "keyword": true,
	"lib2to3": true, "linecache": true, "locale": true, "logging": true,
	"lzma": true, "mailbox": true, "mailcap": true, "marshal": true,
	"math": true, "mimetypes": true, "mmap": true, "modulefinder": true,
	"multiprocessing": true, "netrc": true, "nis": true, "nntplib": true,
	"numbers": true, "operator": true, "optparse": true, "os": true,
	"ossaudiodev": true, "parser": true, "pathlib": true, "pdb": true,
	"pickle": true, "pickletools": true, "pipes": true, "pkgutil": true,
	"platform": true, "plistlib": true, "poplib": true, "posix": true,
	"posixpath": true, "pprint": true, "profile": true, "pstats": true,
	"pty": true, "pwd": true, "py_compile": true, "pyclbr": true,
	"pydoc": true, "queue": true, "quopri": true, "random": true,
	"re": true, "readline": true, "reprlib": true, "resource": true,
	"rlcompleter": true, "runpy": true, "sched": true, "secrets": true,
	"select": true, "selectors": true, "shelve": true, "shlex": true,
	"shutil": true, "signal": true, "site": true, "smtpd": true,
	"smtplib": true, "sndhdr": true, "socket": true, "socketserver": true,
	"spwd": true, "sqlite3": true, "ssl": true, "stat": true,
	"statistics": true, "string": true, "stringprep": true, "struct": true,
	"subprocess": true, "sunau": true, "symtable": true, "sys": true,
	"sysconfig": true, "syslog": true, "tabnanny": true, "tarfile": true,
	"telnetlib": true, "tempfile": true, "termios": true, "test": true,
	"textwrap": true, "threading": true, "time": true, "timeit": true,
	"tkinter": true, "token": true, "tokenize": true, "trace": true,
	"traceback": true, "tracemalloc": true, "tty": true, "turtle": true,
	"turtledemo": true, "types": true, "typing": true, "unicodedata": true,
	"unittest": true, "urllib": true, "uu": true, "uuid": true,
	"venv": true, "warnings": true, "wave": true, "weakref": true,
	"webbrowser": true, "winreg": true, "winsound": true, "wsgiref": true,
	"xdrlib": true, "xml": true, "xmlrpc": true, "zipapp": true,
	"zipfile": true, "zipimport": true, "zlib": true, "_thread": true,
}

// IsStdlibModule 是否为 Python stdlib 模块。
// 借鉴 nuclear-boy SandboxPolicy.kt L520-555。
func IsStdlibModule(name string) bool {
	// 去掉包前缀（foo.bar.baz → 找最顶层 'foo'）
	top := name
	if idx := strings.Index(name, "."); idx >= 0 {
		top = name[:idx]
	}
	return stdlibModules[top]
}

// ─── 危险命令黑名单 ─────────────────────────────────────────

// dangerousPatterns 危险命令 pattern（借鉴 nuclear-boy L429-435）。
var dangerousPatterns = []string{
	// 删除根目录
	"rm -rf /",
	"rm -rf /*",
	"rm -fr /",
	// 格式化磁盘
	"mkfs.",
	"mkfs ",
	// 写裸设备
	"dd if=",
	"> /dev/sda",
	">/dev/sda",
	// fork bomb
	":(){ :|:& };:",
	":(){:|:&};:",
	// 危险权限
	"chmod 777 /",
	"chmod -R 777 /",
	// 远程脚本执行
	"curl | sh",
	"curl|sh",
	"wget | sh",
	"wget|sh",
	"curl | bash",
	"curl|bash",
	"wget | bash",
	"wget|bash",
}

// IsDangerousCommandRemoteScript 检查是否 curl/wget | sh/bash 模式。
// 单独的 " | sh" 太宽泛（会误捕 echo a | sh），单独检查更准。
func IsDangerousCommandRemoteScript(cmd string) bool {
	lower := strings.ToLower(cmd)
	remoteExec := []string{"curl", "wget"}
	shells := []string{"sh", "bash", "zsh"}
	for _, r := range remoteExec {
		for _, s := range shells {
			// 模式 1: "curl ... | sh" 形式
			if strings.Contains(lower, r) && strings.Contains(lower, "| "+s) {
				return true
			}
			// 模式 2: "curl|sh" 紧密形式
			if strings.Contains(lower, r+"|"+s) {
				return true
			}
		}
	}
	return false
}

// IsDangerousCommand 检查命令是否含黑名单 pattern。
//
// 借鉴 nuclear-boy SandboxPolicy.kt L429-435。
func IsDangerousCommand(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, p := range dangerousPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	if IsDangerousCommandRemoteScript(cmd) {
		return true
	}
	return false
}

// ─── Preamble 生成 ──────────────────────────────────────────

// BuildPreamble 返回注入到 Python 脚本开头的代码。
//
// 借鉴 nuclear-boy SandboxPolicy.kt L60-100 buildPolicyPreamble。
//
// 根据 mode 注入：
//   - STRICT: 重写 builtins.open 为空 / 禁 subprocess / 禁 socket
//   - STANDARD: 同 STRICT 但允许常见第三方库
//   - RELAXED: 保留网络/子进程，只检查黑名单
//   - DOCUMENT_GENERATION: 允许 docx/xlsx/pptx
func BuildPreamble(mode SandboxMode) string {
	switch mode {
	case SandboxStrict:
		return `import builtins
import sys

# SandboxPolicy(STRICT): 禁网络/子进程/危险文件操作
_orig_open = builtins.open
def _safe_open(file, mode='r', *args, **kwargs):
    if any(p in str(file) for p in ('/etc/', '/proc/', '/sys/', '/dev/')):
        raise PermissionError(f"SandboxPolicy: file access denied: {file}")
    return _orig_open(file, mode, *args, **kwargs)
builtins.open = _safe_open

# 禁用危险模块
sys.modules['subprocess'] = type(sys)('subprocess')
sys.modules['socket'] = type(sys)('socket')
sys.modules['urllib.request'] = type(sys)('urllib.request')

print("[SandboxPolicy] STRICT mode active", file=sys.stderr)
`
	case SandboxStandard:
		return `import builtins
import sys

# SandboxPolicy(STANDARD): 禁网络/子进程
sys.modules['subprocess'] = type(sys)('subprocess')
sys.modules['socket'] = type(sys)('socket')

print("[SandboxPolicy] STANDARD mode active", file=sys.stderr)
`
	case SandboxRelaxed:
		return `# SandboxPolicy(RELAXED): 允许网络/子进程
import sys
print("[SandboxPolicy] RELAXED mode active", file=sys.stderr)
`
	case SandboxDocumentGeneration:
		return `import sys
# SandboxPolicy(DOCUMENT_GENERATION): 允许 docx/xlsx/pptx
print("[SandboxPolicy] DOCUMENT_GENERATION mode active", file=sys.stderr)
`
	default:
		return ""
	}
}

// ValidateImport 检查 import 是否允许（mode 维度 + stdlib 白名单）。
func ValidateImport(moduleName string, mode SandboxMode) error {
	if IsStdlibModule(moduleName) {
		return nil // stdlib 永远允许
	}
	if !mode.AllowsThirdPartyImports() {
		return &SandboxError{Message: "imports not allowed in STRICT mode: " + moduleName}
	}
	return nil
}

// ValidateCommand 检查 shell 命令（mode 维度 + 危险黑名单）。
func ValidateCommand(cmd string, mode SandboxMode) error {
	if !mode.AllowsSubprocess() {
		return &SandboxError{Message: "subprocess not allowed in " + mode.String() + " mode"}
	}
	if IsDangerousCommand(cmd) {
		return &SandboxError{Message: "dangerous command blocked: " + cmd}
	}
	return nil
}

// SandboxError 沙箱拒绝错误。
type SandboxError struct {
	Message string
}

func (e *SandboxError) Error() string { return e.Message }
