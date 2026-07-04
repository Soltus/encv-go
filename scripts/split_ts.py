#!/usr/bin/env python3
"""
按顶层 export 声明拆分 TS/Vue 文件。
用法: split_ts.py <src> <out1> <names1> [<out2> <names2> ...]
names 是顶层 export 标识符列表（逗号分隔）。
"""
import sys
import re
import os

def main():
    if len(sys.argv) < 4 or (len(sys.argv) - 2) % 2 != 0:
        print("Usage: split_ts.py <src> <out1> <names1> [<out2> <names2> ...]", file=sys.stderr)
        sys.exit(1)
    src = sys.argv[1]
    with open(src) as f:
        content = f.read()
    out_dir = os.path.dirname(src) or "."
    src_basename = os.path.basename(src)

    groups = []
    for i in range(2, len(sys.argv), 2):
        out = os.path.join(out_dir, sys.argv[i])
        names = set(n.strip() for n in sys.argv[i+1].split(","))
        groups.append((out, names))

    import_block = extract_imports(content)

    decls = split_top_level(content)
    print(f"  Found {len(decls)} top-level decls", file=sys.stderr)

    group_bodies = {out: [] for out, _ in groups}
    kept_bodies = []
    for name, body in decls:
        matched = False
        for out, names in groups:
            if name in names:
                group_bodies[out].append(body)
                matched = True
                break
        if not matched:
            kept_bodies.append(body)

    for (out, _), bodies in zip(groups, [group_bodies[o] for o, _ in groups]):
        if not bodies:
            continue
        header = f"// {os.path.basename(out)} - 拆分自 {src_basename}\n\n{import_block}\n\n"
        body = "\n\n".join(bodies) + "\n"
        with open(out, "w") as f:
            f.write(header + body)
        print(f"  {os.path.basename(out):50s}  {len(bodies):2d} decls  {body.count(chr(10)):4d} lines", file=sys.stderr)

    if kept_bodies:
        header = f"// {src_basename} - 拆分后保留\n\n{import_block}\n\n"
        body = "\n\n".join(kept_bodies) + "\n"
        with open(src, "w") as f:
            f.write(header + body)
        print(f"  {src_basename:50s}  {len(kept_bodies):2d} decls (kept)  {body.count(chr(10)):4d} lines", file=sys.stderr)


def extract_imports(content):
    lines = content.split("\n")
    out = []
    in_import = False
    for line in lines:
        s = line.lstrip()
        if s.startswith("import ") or s.startswith("import{"):
            in_import = True
            out.append(line)
        elif in_import:
            if s.startswith("from ") or s.startswith('"') or s.startswith("'"):
                out.append(line)
                in_import = False
            elif line.startswith(" ") or line.startswith("\t") or s == "" or s.startswith("type "):
                out.append(line)
            else:
                in_import = False
    return "\n".join(out)


def split_top_level(content):
    lines = content.split("\n")
    n = len(lines)
    decl_starts = []
    for i, line in enumerate(lines):
        stripped = line.lstrip()
        if not stripped.startswith("export "):
            continue
        m = re.match(r"^export\s+(?:async\s+)?(?:const|let|var|function|class|interface|type|enum)\s+(\w+)", stripped)
        if m:
            decl_starts.append((i, m.group(1)))
    decls = []
    for idx, (start, name) in enumerate(decl_starts):
        end = decl_starts[idx + 1][0] if idx + 1 < len(decl_starts) else n
        body = "\n".join(lines[start:end]).rstrip()
        decls.append((name, body))
    return decls


if __name__ == "__main__":
    main()
