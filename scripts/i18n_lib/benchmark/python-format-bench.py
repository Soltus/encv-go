#!/usr/bin/env python3
"""
Python 下各格式解析性能测试（轻量版，只用标准库 + PyYAML）
"""
import json
import time
import re
import sys
from pathlib import Path

dir_path = sys.argv[1] if len(sys.argv) > 1 else '/tmp/i18n-formats-5k'

print()
print('🔬 Python 格式解析性能测试')
print('=' * 60)
print(f'环境: Python {sys.version.split()[0]}')
print(f'目录: {dir_path}')
print()

results = []

def bench(name, fn, warmup=2, rounds=5):
    for _ in range(warmup):
        fn()
    times = []
    sample = None
    for i in range(rounds):
        t0 = time.time()
        result = fn()
        t1 = time.time()
        times.append(t1 - t0)
        if i == 0 and result and isinstance(result, dict):
            sample = result
            first_lang = next(iter(result.keys()))
            key_count = len(result.get(first_lang, {}))
            lang_count = len(result)
            total = lang_count * key_count
            print(f'  {name}: {lang_count} 语言 × {key_count:,} keys = {total:,} 条目')
    times.sort()
    median_ms = times[len(times)//2] * 1000
    results.append((name, median_ms))
    print(f'  耗时: {median_ms:.0f}ms (中位)')
    print()

# ====== 1. JSON ======
json_path = Path(dir_path) / 'dict.json'
json_content = json_path.read_text(encoding='utf-8')
print(f'📄 JSON: {len(json_content.encode())/1024:.0f} KB')
bench('JSON', lambda: json.loads(json_content))

# ====== 2. JSONC (去注释 + 去尾逗号 + json) ======
jsonc_path = Path(dir_path) / 'dict.jsonc'
jsonc_content = jsonc_path.read_text(encoding='utf-8')
print(f'📄 JSONC: {len(jsonc_content.encode())/1024:.0f} KB')

def strip_jsonc(text):
    """去除 JSONC 中的注释和尾逗号"""
    result = []
    in_string = False
    string_char = ''
    i = 0
    while i < len(text):
        c = text[i]
        if in_string:
            result.append(c)
            if c == '\\' and i + 1 < len(text):
                result.append(text[i+1])
                i += 2
                continue
            if c == string_char:
                in_string = False
            i += 1
        else:
            if c in ('"', "'"):
                in_string = True
                string_char = c
                result.append(c)
                i += 1
            elif c == '/' and i + 1 < len(text) and text[i+1] == '/':
                while i < len(text) and text[i] != '\n':
                    i += 1
            elif c == '/' and i + 1 < len(text) and text[i+1] == '*':
                i += 2
                while i < len(text) - 1 and not (text[i] == '*' and text[i+1] == '/'):
                    i += 1
                i += 2
            else:
                result.append(c)
                i += 1
    # 去除尾逗号（, 后面跟 ] 或 }）
    result_str = ''.join(result)
    result_str = re.sub(r',\s*([}\]])', r'\1', result_str)
    return result_str

def parse_jsonc():
    stripped = strip_jsonc(jsonc_content)
    return json.loads(stripped)

bench('JSONC (去注释+json)', parse_jsonc)

# ====== 3. YAML ======
try:
    import yaml
    yaml_path = Path(dir_path) / 'dict.yaml'
    yaml_content = yaml_path.read_text(encoding='utf-8')
    print(f'📄 YAML: {len(yaml_content.encode())/1024:.0f} KB')
    bench('YAML (PyYAML)', lambda: yaml.safe_load(yaml_content))
except ImportError:
    print('  YAML: 跳过 (PyYAML 未安装)')

# ====== 4. TOML ======
try:
    import tomllib
    toml_path = Path(dir_path) / 'dict.toml'
    toml_bytes = toml_path.read_bytes()
    print(f'📄 TOML: {len(toml_bytes)/1024:.0f} KB')
    bench('TOML (tomllib)', lambda: tomllib.loads(toml_bytes.decode('utf-8')))
except ImportError:
    print('  TOML: 跳过 (tomllib 不可用)')

# ====== 汇总 ======
print('=' * 60)
print('📊 性能汇总（从快到慢）')
print('=' * 60)
results.sort(key=lambda x: x[1])
fastest = results[0][1]

print()
header = f'  {"格式":<24} {"耗时":>8} {"速度比":>10}   注释'
sep = '  ' + '-'*24 + '-'*8 + '-'*10 + '  ' + '-'*8
print(header)
print(sep)

comments = {
    'JSON': '❌ 无',
    'JSONC (去注释+json)': '✅ 有',
    'YAML (PyYAML)': '✅ 有',
    'TOML (tomllib)': '✅ 有',
}

for name, ms in results:
    ratio = f'{ms/fastest:.1f}x'
    ms_str = f'{ms:.0f}ms'
    c = comments.get(name, '?')
    print(f'  {name:<24} {ms_str:>8} {ratio:>10}   {c}')

print()
print(f'💡 最快: {results[0][0]} ({results[0][1]:.0f}ms)')
print(f'💡 最慢: {results[-1][0]} ({results[-1][1]:.0f}ms)')
print(f'💡 差距: {results[-1][1]/results[0][1]:.1f}x')

print()
print('=' * 60)
