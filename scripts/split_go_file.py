# scripts/split_go_file.py
# 通用 Go 文件拆分器：按顶层声明（func / type / const / var）切分到多个子文件
#
# 用法：
#   python3 scripts/split_go_file.py <src> <out1> <funcs1> [<out2> <funcs2> ...]
# 例：
#   python3 scripts/split_go_file.py agent_api.go agent_crypto.go "deriveKey,EncryptApiKey,..."
#
# 行为：
#   1. 解析 <src>，提取所有顶层声明（func/type/const/var）
#   2. 按 <funcsN> 列表分配到 <outN> 文件
#   3. 未在列表中的声明留在 <src>
#   4. 每个新文件继承原 imports
#   5. 运行后用 goimports 清理
import re
import sys
import os

if len(sys.argv) < 4 or (len(sys.argv) - 2) % 2 != 0:
    print("Usage: split_go_file.py <src> <out1> <funcs1_csv> [<out2> <funcs2_csv> ...]")
    sys.exit(1)

SRC = sys.argv[1]
groups = []
for i in range(2, len(sys.argv), 2):
    groups.append((sys.argv[i], set(sys.argv[i+1].split(","))))

with open(SRC, "r") as f:
    content = f.read()
lines = content.split("\n")

# 1) 提取 import 块
imports_match = re.search(r"^import\s*\((.*?)\n\)\n", content, re.DOTALL | re.MULTILINE)
imports_block = imports_match.group(0) if imports_match else ""
pkg_match = re.search(r"^package\s+(\w+)", content, re.MULTILINE)
pkg_line = pkg_match.group(0) if pkg_match else "package main"

# 2) 找顶层声明起始行
# 顶层声明：func / type / const / var + 缩进=0
decl_starts = []
for i, line in enumerate(lines):
    if re.match(r"^(func |type |const |var )", line):
        # 必须不在 import 块内
        if imports_match and imports_match.start() < i < imports_match.end():
            continue
        decl_starts.append(i)

# 3) 计算每个声明的结束行
def find_end(start):
    """从 start 找匹配的右花括号；处理 const/type/var 多行块（用括号深度）和 func"""
    line = lines[start]
    # func 用花括号匹配
    if line.startswith("func "):
        depth = 0
        saw_open = False
        for i in range(start, len(lines)):
            for ch in lines[i]:
                if ch == "{":
                    depth += 1
                    saw_open = True
                elif ch == "}":
                    depth -= 1
                    if saw_open and depth == 0:
                        return i
        return len(lines) - 1
    # type/const/var：用括号平衡（type ... { ... } 形式）
    # 但很多 type 是单行（如 type Foo struct { ... } 跨多行）
    if line.startswith("type "):
        # type X struct/interface { ... } 跨多行
        if "{" in line:
            depth = 0
            saw_open = False
            for i in range(start, len(lines)):
                for ch in lines[i]:
                    if ch == "{":
                        depth += 1
                        saw_open = True
                    elif ch == "}":
                        depth -= 1
                        if saw_open and depth == 0:
                            return i
            return len(lines) - 1
        # type X string/int — 单行结束
        return start
    if line.startswith("const "):
        # const (...) 或 const X = ...
        if "(" in line and ")" not in line:
            # 多行 const 块
            for i in range(start, len(lines)):
                if lines[i].strip().startswith(")"):
                    return i
            return len(lines) - 1
        # 单行 const
        return start
    if line.startswith("var "):
        if "(" in line and ")" not in line:
            for i in range(start, len(lines)):
                if lines[i].strip().startswith(")"):
                    return i
            return len(lines) - 1
        return start
    return start

decls = []
for i, start in enumerate(decl_starts):
    end = find_end(start)
    if i + 1 < len(decl_starts):
        end = min(end, decl_starts[i+1] - 1)
    name = re.match(r"^(func (\(.*?\) )?|type |const |var )(\w+)", lines[start])
    if name:
        decl_name = name.group(3)
        body = "\n".join(lines[start:end+1])
        decls.append((decl_name, start, end, body))

# 4) 按声明名路由到组
group_bodies = {out: [] for out, _ in groups}
kept = []
for name, s, e, body in decls:
    matched = False
    for out, names in groups:
        if name in names:
            group_bodies[out].append(body)
            matched = True
            break
    if not matched:
        kept.append(body)

# 5) 写每个新文件
for out, _ in groups:
    if not group_bodies[out]:
        continue
    header = pkg_line + "\n\n// " + out + " — 拆分自 " + os.path.basename(SRC) + "\n\n" + imports_block + "\n\n"
    body = "\n\n".join(group_bodies[out]) + "\n"
    with open(out, "w") as f:
        f.write(header + body)
    print(f"  {out:50s}  {len(group_bodies[out]):2d} decls  {(header+body).count(chr(10)):4d} lines")

# 6) 覆盖 <src>：只留 kept 声明
new_header = pkg_line + "\n\n// " + os.path.basename(SRC) + " — 拆分后保留\n\n" + imports_block + "\n\n"
new_body = "\n\n".join(kept) + "\n" if kept else ""
with open(SRC, "w") as f:
    f.write(new_header + new_body)
print(f"  {SRC:50s}  {len(kept):2d} decls (kept)  {(new_header+new_body).count(chr(10)):4d} lines")
