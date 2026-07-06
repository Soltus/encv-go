#!/usr/bin/env python3
"""i18n 系统边界/悲观测试 - 验证解析器和扫描器的鲁棒性"""
from __future__ import annotations

import sys
import json
import subprocess
import tempfile
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent))

FIXTURES_DIR = Path(__file__).parent / "test-fixtures"


def test_node_extractor_edge_cases():
    """测试 Node.js 提取器处理边界情况"""
    print("=" * 60)
    print("测试1: Node.js 提取器 - 边界情况")
    print("=" * 60)

    edge_file = FIXTURES_DIR / "edge-cases.ts"
    cmd = ["node", str(Path(__file__).parent / "extract-i18n.mjs"), str(edge_file)]
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=30, cwd=str(Path(__file__).parent.parent.parent))

    assert result.returncode == 0, f"提取失败: {result.stderr}"

    data = json.loads(result.stdout)
    zh = data.get("zh-CN", {})
    en = data.get("en", {})

    checks = [
        ("空字符串", "edge.emptyString", ""),
        ("单字符", "edge.singleChar", "a"),
        ("单引号转义", "quote.singleQuoteValue", "It's a test"),
        ("双引号转义", "quote.doubleQuoteValue", 'He said "hello" to me'),
        ("混合引号", "quote.mixedQuotes", 'He said "it\'s mine"'),
        ("反斜杠转义", "quote.escapedBackslash", "path\\to\\file"),
        ("中文", "unicode.chinese", "你好世界"),
        ("emoji", "unicode.emoji", "🎉 欢迎 🚀"),
        ("含虚线的key", "key.with-dashes", "dashed-key"),
        ("含下划线的key", "key.with_underscores", "underscore_key"),
        ("多点的key", "key.with.dots.multiple", "multiple.dots.in.key"),
        ("数字开头的key", "key.startsWithNumber", "123start"),
        ("非注释的value", "comment.notAComment", "// this is not a comment"),
        ("简单变量", "var.simple", "Hello {name}"),
        ("多变量", "var.multiple", "{count} items found in {folder}"),
        ("超长value存在", "long.superLongValue", None),
    ]

    passed = 0
    failed = 0
    for name, key, expected in checks:
        if key not in zh:
            print(f"  ❌ {name}: key 不存在")
            failed += 1
            continue
        actual = zh[key]
        if expected is not None and actual != expected:
            print(f"  ❌ {name}: 期望 {repr(expected)}, 实际 {repr(actual[:50])}")
            failed += 1
        else:
            if expected is None:
                if len(actual) > 1000:
                    print(f"  ✅ {name}: 存在且长度 {len(actual)}")
                    passed += 1
                else:
                    print(f"  ❌ {name}: 长度不足 {len(actual)}")
                    failed += 1
            else:
                print(f"  ✅ {name}")
                passed += 1

    print(f"\n结果: {passed} 通过, {failed} 失败")
    return failed == 0


def test_node_extractor_evil_cases():
    """测试 Node.js 提取器处理恶意/悲观情况"""
    print("\n" + "=" * 60)
    print("测试2: Node.js 提取器 - 恶意/悲观情况")
    print("=" * 60)

    evil_file = FIXTURES_DIR / "evil-cases.ts"
    cmd = ["node", str(Path(__file__).parent / "extract-i18n.mjs"), str(evil_file)]
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=30, cwd=str(Path(__file__).parent.parent.parent))

    assert result.returncode == 0, f"提取失败（崩溃）: {result.stderr}"

    data = json.loads(result.stdout)
    zh = data.get("zh-CN", {})

    evil_keys = [k for k in zh if k.startswith("evil.")]
    print(f"解析到 {len(evil_keys)} 个 evil key")

    # 验证几个关键的
    test_pairs = [
        ("evil.balancedBracesInString", "{{{{{{{{{{}}}}}}}}}}"),
        ("evil.onlyBraceOpen", "{"),
        ("evil.onlyBraceClose", "}"),
        ("evil.emptyObject", "{}"),
        ("evil.colonsEverywhere", "a:b:c:d:e:f:g"),
        ("evil.commasInValue", "a, b, c, d, e, f"),
    ]

    passed = 0
    failed = 0
    for key, expected in test_pairs:
        if key not in zh:
            print(f"  ❌ {key}: 不存在")
            failed += 1
        elif zh[key] != expected:
            print(f"  ❌ {key}: 期望 {repr(expected)}, 实际 {repr(zh[key][:60])}")
            failed += 1
        else:
            print(f"  ✅ {key}")
            passed += 1

    print(f"\n结果: {passed} 通过, {failed} 失败")
    return failed == 0


def test_scanner_edge_cases():
    """测试扫描器处理边界情况"""
    print("\n" + "=" * 60)
    print("测试3: 源码扫描器 - 边界情况")
    print("=" * 60)

    from i18n_lib.scanner import scan_file, is_dynamic_key

    # 动态 key 检测
    dynamic_cases = [
        ("tasks.status.${status}", True),
        ("settings.${key}", True),
        ("`tasks.${name}`", True),
        ("tasks.status.running", False),
        ("common.confirm", False),
        ("agent.title", False),
        ("a + b", True),
        ("normal.key", False),
    ]

    passed = 0
    failed = 0
    for key, expected in dynamic_cases:
        actual = is_dynamic_key(key)
        if actual == expected:
            print(f"  ✅ is_dynamic_key({repr(key)}) = {actual}")
            passed += 1
        else:
            print(f"  ❌ is_dynamic_key({repr(key)}) = {actual}, 期望 {expected}")
            failed += 1

    # 注释行过滤
    test_content = """// t('commented.out.key')
const x = t('real.key.1')
const y = $t('real.key.2') // t('commented.inline')
// tField('ignored')
const z = tField('field-key')
"""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".ts", delete=False) as f:
        f.write(test_content)
        tmp_path = f.name

    try:
        keys, _ = scan_file(tmp_path)
        direct_keys = set(keys.keys())

        expected_direct = {"real.key.1", "real.key.2"}
        unexpected = direct_keys - expected_direct
        missing = expected_direct - direct_keys

        if not unexpected and not missing:
            print(f"  ✅ 注释过滤正确，直接key: {direct_keys}")
            passed += 1
        else:
            if unexpected:
                print(f"  ❌ 不应存在的key: {unexpected}")
            if missing:
                print(f"  ❌ 缺失的key: {missing}")
            failed += 1
    finally:
        os.unlink(tmp_path)

    print(f"\n结果: {passed} 通过, {failed} 失败")
    return failed == 0


def test_cache_lock_mechanism():
    """测试缓存 lock 机制"""
    print("\n" + "=" * 60)
    print("测试4: 缓存 lock 机制")
    print("=" * 60)

    from i18n_lib.loader import _cache_path, _save_to_cache, _load_from_cache, _compute_files_hash

    test_app = "__test_lock_app__"
    json_path, lock_path = _cache_path(test_app)

    # 清理
    if json_path.exists():
        json_path.unlink()
    if lock_path.exists():
        lock_path.unlink()

    passed = 0
    failed = 0

    # 1. 正常保存和读取
    test_data = {"zh-CN": {"test.key": "test value"}, "en": {"test.key": "test value"}}
    test_hash = "testhash123"
    _save_to_cache(test_app, test_hash, test_data)

    loaded = _load_from_cache(test_app, test_hash)
    if loaded and loaded.get("zh-CN", {}).get("test.key") == "test value":
        print("  ✅ 正常保存和读取")
        passed += 1
    else:
        print("  ❌ 正常保存和读取失败")
        failed += 1

    # 2. hash 不匹配时缓存失效
    loaded_bad = _load_from_cache(test_app, "wronghash")
    if loaded_bad is None:
        print("  ✅ hash 不匹配时缓存失效")
        passed += 1
    else:
        print("  ❌ hash 不匹配时仍返回缓存")
        failed += 1

    # 3. lock 文件存在时返回 None（模拟并发写入）
    lock_path.touch()
    loaded_locked = _load_from_cache(test_app, test_hash)
    if loaded_locked is None:
        print("  ✅ lock 文件存在时返回 None")
        passed += 1
    else:
        print("  ❌ lock 文件存在时仍返回数据")
        failed += 1

    # 清理
    if json_path.exists():
        json_path.unlink()
    if lock_path.exists():
        lock_path.unlink()

    print(f"\n结果: {passed} 通过, {failed} 失败")
    return failed == 0


def test_full_integration():
    """完整集成测试 - 用真实项目数据验证"""
    print("\n" + "=" * 60)
    print("测试5: 完整集成测试（真实项目数据）")
    print("=" * 60)

    from i18n_lib.loader import load_all_dicts
    from i18n_lib.scanner import extract_used_keys
    from i18n_lib.lint import check_variable_consistency

    # 清理缓存
    import shutil
    from i18n_lib.config import CACHE_DIR
    if CACHE_DIR.exists():
        shutil.rmtree(CACHE_DIR)

    dicts = load_all_dicts("encv-mobile", use_cache=True)
    zh = dicts.get("zh-CN", {})
    en = dicts.get("en", {})

    print(f"  zh-CN: {len(zh)} keys")
    print(f"  en: {len(en)} keys")

    if len(zh) > 0 and len(zh) == len(en):
        print("  ✅ 字典加载成功，zh-CN 和 en key 数一致")
    else:
        print(f"  ❌ 字典加载异常: zh-CN={len(zh)}, en={len(en)}")
        return False

    keys = extract_used_keys("encv-mobile", use_cache=True)
    print(f"  使用中的key: {len(keys)}")

    if len(keys) > 1000:
        print("  ✅ 源码扫描成功，key 数合理")
    else:
        print(f"  ❌ 源码扫描异常: {len(keys)} keys")
        return False

    missing = sum(1 for k in keys if k not in zh)
    if missing == 0:
        print("  ✅ 0 个缺失 key")
    else:
        print(f"  ❌ {missing} 个缺失 key")
        return False

    issues = check_variable_consistency(dicts)
    if len(issues) == 0:
        print("  ✅ 变量一致性检查通过")
    else:
        print(f"  ❌ {len(issues)} 个变量不一致")
        return False

    print("\n  ✅ 完整集成测试全部通过")
    return True


def main():
    print("i18n 系统边界/悲观测试套件")
    print()

    tests = [
        ("边界情况提取", test_node_extractor_edge_cases),
        ("恶意情况提取", test_node_extractor_evil_cases),
        ("扫描器边界", test_scanner_edge_cases),
        ("缓存 lock 机制", test_cache_lock_mechanism),
        ("完整集成", test_full_integration),
    ]

    results = []
    for name, fn in tests:
        try:
            result = fn()
            results.append((name, result))
        except Exception as e:
            print(f"\n💥 测试 {name} 崩溃: {e}")
            import traceback
            traceback.print_exc()
            results.append((name, False))

    print("\n" + "=" * 60)
    print("测试总结")
    print("=" * 60)
    for name, passed in results:
        status = "✅ 通过" if passed else "❌ 失败"
        print(f"  {status}: {name}")

    all_passed = all(r for _, r in results)
    print(f"\n总计: {sum(1 for _, r in results if r)}/{len(results)} 通过")
    return 0 if all_passed else 1


if __name__ == "__main__":
    sys.exit(main())
