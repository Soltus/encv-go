import re, pathlib, sys

ROOT = pathlib.Path("/workspace/app")
def log(m):
    print(m, flush=True)

def parse_blocks(path):
    text = pathlib.Path(path).read_text(encoding="utf-8")
    def block(pat):
        m = re.search(pat, text, re.S)
        d = {}
        if m:
            for mm in re.finditer(r'"((?:[^"\\]|\\.)*)"\s*:\s*"((?:[^"\\]|\\.)*)"', m.group(1)):
                d[mm.group(1)] = mm.group(2)
        return d
    zh = block(r'"zh-CN"\s*:\s*\{(.*?)\n\s*\},')
    en = block(r'\ben\s*:\s*\{(.*?)\n\s*\},')
    return zh, en

log("reading i18n log...")
logtext = (ROOT / "check-logs/i18n-lint-all.log").read_text(encoding="utf-8")
missing_keys = set(re.findall(r"^\s*key:\s*([\w.]+)", logtext, re.M))
log(f"missing_keys from log: {len(missing_keys)}")

pairs = [
    ("encv-mobile/src/i18n/tasks.ts", "packages/shared-components/src/i18n/tasks.ts", "tasks"),
    ("encv-mobile/src/i18n/files.ts", "packages/shared-components/src/i18n/files.ts", "files"),
]

for encv_rel, shared_rel, prefix in pairs:
    log(f"processing {shared_rel} ...")
    ezh, een = parse_blocks(ROOT / encv_rel)
    szh, sen = parse_blocks(ROOT / shared_rel)
    keys = [k for k in missing_keys if k.startswith(prefix + ".")]
    add_zh = {k: ezh[k] for k in keys if k in ezh and k not in szh}
    add_en = {k: een[k] for k in keys if k in een and k not in sen}
    if not add_zh and not add_en:
        log(f"[skip] {shared_rel}: nothing to add")
        continue
    text = (ROOT / shared_rel).read_text(encoding="utf-8")
    if add_zh:
        zh_part = "\n".join(f'    "{k}": "{add_zh[k]}",' for k in sorted(add_zh))
        text = text.replace("  },\n  en:", f"    {zh_part}\n  }},\n  en:", 1)
    if add_en:
        en_part = "\n".join(f'    "{k}": "{add_en[k]}",' for k in sorted(add_en))
        text = text.replace("  },\n};", f"    {en_part}\n  }},\n};", 1)
    (ROOT / shared_rel).write_text(text, encoding="utf-8")
    log(f"[ok] {shared_rel}: +zh {len(add_zh)}, +en {len(add_en)}")
