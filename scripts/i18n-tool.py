#!/usr/bin/env python3
"""
i18n 工具箱 - 跨项目的 i18n 管理工具（模块化 v2.0）

针对中文与英文特性深度优化，模块化架构：
- i18n_lib.scanner    源码扫描
- i18n_lib.loader     字典加载与解析
- i18n_lib.lint       Lint 检查（缺失key/变量一致性/近重复）
- i18n_lib.search     搜索与近重复检测（MinHash+LSH+n-gram）
- i18n_lib.db         SQLite 数据库集成
- i18n_lib.typegen    TypeScript 类型生成
- i18n_lib.stats      统计信息
- i18n_lib.benchmark  性能基准测试
- i18n_lib.tokenizer  中英文分词
- i18n_lib.perf       性能追踪
- i18n_lib.config     配置管理
"""
import sys
import argparse


def main():
    parser = argparse.ArgumentParser(
        description="i18n 工具箱 - 跨项目的 i18n 管理工具",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  python3 scripts/i18n-tool.py scan --app encv-mobile
  python3 scripts/i18n-tool.py lint --app encv-mobile
  python3 scripts/i18n-tool.py var-check --app encv-mobile
  python3 scripts/i18n-tool.py dup --app encv-mobile
  python3 scripts/i18n-tool.py stats --app encv-mobile
  python3 scripts/i18n-tool.py gen-types --app encv-mobile
  python3 scripts/i18n-tool.py db-init --app encv-mobile
  python3 scripts/i18n-tool.py find-key "搜索关键词"
  python3 scripts/i18n-tool.py benchmark
""",
    )

    subparsers = parser.add_subparsers(dest="command", help="可用命令")

    p_scan = subparsers.add_parser("scan", help="扫描源码中使用的 i18n key 并检查完整性")
    p_scan.add_argument("--app", help="应用名称 (默认 encv-mobile)")
    p_scan.add_argument("--no-cache", action="store_true", help="禁用缓存")

    p_lint = subparsers.add_parser("lint", help="运行所有 i18n lint 检查")
    p_lint.add_argument("--app", help="应用名称")
    p_lint.add_argument("--unused", action="store_true", help="包含未使用 key 检查")
    p_lint.add_argument("--dup", action="store_true", help="包含近重复检测")
    p_lint.add_argument("--dup-value", action="store_true", help="包含重复 value 检测")

    p_varcheck = subparsers.add_parser("var-check", help="检查翻译变量/参数一致性")
    p_varcheck.add_argument("--app", help="应用名称")

    p_dup = subparsers.add_parser("dup", help="检测近重复翻译")
    p_dup.add_argument("--app", help="应用名称")
    p_dup.add_argument("--threshold", type=float, default=0.85, help="相似度阈值 (默认 0.85)")
    p_dup.add_argument("--locale", default="zh-CN", help="语言 (默认 zh-CN)")

    p_stats = subparsers.add_parser("stats", help="显示 i18n 字典统计")
    p_stats.add_argument("--app", help="应用名称")

    p_types = subparsers.add_parser("gen-types", help="生成 TypeScript 类型定义")
    p_types.add_argument("--app", help="应用名称")

    p_dbinit = subparsers.add_parser("db-init", help="初始化 SQLite 数据库")
    p_dbinit.add_argument("--app", help="应用名称")

    p_dbquery = subparsers.add_parser("db-query", help="执行数据库查询")
    p_dbquery.add_argument("sql", help="SQL 查询语句")

    p_find = subparsers.add_parser("find-key", help="搜索翻译 key/value")
    p_find.add_argument("query", help="搜索关键词")
    p_find.add_argument("--app", help="应用名称")
    p_find.add_argument("--locale", help="语言过滤")
    p_find.add_argument("--limit", type=int, default=50, help="结果数量限制")

    p_bench = subparsers.add_parser("benchmark", help="运行性能基准测试")
    p_bench.add_argument("--app", help="应用名称")

    p_compile = subparsers.add_parser("compile-json", help="将 TS 字典编译为 JSON（供 Go/Python 等复用）")
    p_compile.add_argument("--app", help="应用名称")
    p_compile.add_argument("--output", help="输出目录")

    p_compile_android = subparsers.add_parser("compile-android", help="将 TS 字典编译为 Android strings.xml（供 Kotlin 复用）")
    p_compile_android.add_argument("--app", help="应用名称")
    p_compile_android.add_argument("--output", help="输出目录")

    p_addkey = subparsers.add_parser("add-key", help="添加翻译 key 到 TS 字典")
    p_addkey.add_argument("key", help="翻译 key")
    p_addkey.add_argument("--zh", required=True, help="中文翻译")
    p_addkey.add_argument("--en", required=True, help="英文翻译")
    p_addkey.add_argument("--app", help="应用名称")
    p_addkey.add_argument("--file", help="目标字典文件（不指定则自动猜测）")

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        return 1

    try:
        if args.command == "scan":
            from i18n_lib.scanner import extract_used_keys
            from i18n_lib.loader import load_all_dicts
            from i18n_lib.config import get_app_config

            used_keys = extract_used_keys(args.app, use_cache=not args.no_cache)
            dicts = load_all_dicts(args.app, use_cache=not args.no_cache)

            try:
                from i18n_lib.scanner_go import extract_go_i18n_keys
                app_cfg = get_app_config(args.app)
                if app_cfg.go_dirs:
                    go_keys = extract_go_i18n_keys(app_cfg.go_dirs, use_cache=not args.no_cache)
                    for key, files in go_keys.items():
                        if key in used_keys:
                            used_keys[key].extend(files)
                        else:
                            used_keys[key] = files
            except Exception:
                pass

            try:
                from i18n_lib.scanner_kotlin import extract_kotlin_i18n_keys
                app_cfg = get_app_config(args.app)
                if app_cfg.kotlin_dirs:
                    kt_keys = extract_kotlin_i18n_keys(app_cfg.kotlin_dirs)
                    for key, files in kt_keys.items():
                        if key in used_keys:
                            used_keys[key].extend(files)
                        else:
                            used_keys[key] = files
            except Exception:
                pass

            zh_dict = dicts.get("zh-CN", {})
            missing = 0
            for key in sorted(used_keys.keys()):
                if key not in zh_dict:
                    missing += 1
                    files = used_keys[key][:3]
                    print(f"❌ MISSING: {key}")
                    for f in files:
                        print(f"   ↳ {f}")

            print()
            print(f"📊 结果: {len(used_keys)} 个使用中的 key, {missing} 个缺失")

            if missing > 0:
                return 1
            return 0

        elif args.command == "lint":
            from i18n_lib.lint import run_all_checks

            result = run_all_checks(
                args.app,
                include_unused=args.unused,
                include_dup=args.dup,
                include_dup_value=args.dup_value,
            )

            for issue in result["issues"]:
                print(issue.format())
                print()

            print("=" * 60)
            print(f"📊 检查结果: 共 {result['total']} 个问题")
            print(f"   ❌ 错误: {len(result['errors'])}")
            print(f"   ⚠️  警告: {len(result['warnings'])}")
            print(f"   ℹ️  信息: {len(result['infos'])}")
            print()
            print(f"   使用中的 key: {result['used_keys_count']}")
            print(f"   字典中的 key: {result['dict_keys_count']}")

            if result["errors"]:
                return 1
            return 0

        elif args.command == "var-check":
            from i18n_lib.lint import check_variable_consistency
            from i18n_lib.loader import load_all_dicts

            print("🔍 检查翻译变量/参数一致性...")
            print()

            dicts = load_all_dicts(args.app)
            issues = check_variable_consistency(dicts)

            for issue in issues:
                print(issue.format())
                print()

            print(f"📊 变量一致性检查结果:")
            print(f"   检查的 key 总数: {len(dicts.get('zh-CN', {}))}")
            print(f"   发现问题: {len(issues)} 个")

            if issues:
                print()
                print("❌ 存在变量不一致问题")
                return 1
            else:
                print()
                print("✅ 所有翻译的变量完全一致！")
                return 0

        elif args.command == "dup":
            from i18n_lib.lint import check_near_duplicates
            from i18n_lib.loader import load_all_dicts

            print(f"🔍 检测近重复翻译 (阈值: {args.threshold}, 语言: {args.locale})...")
            print()

            dicts = load_all_dicts(args.app)
            issues = check_near_duplicates(dicts, args.locale, args.threshold)

            for issue in issues:
                print(issue.format())
                print()

            print(f"📊 近重复检测结果:")
            print(f"   找到 {len(issues)} 组近重复翻译")
            return 0

        elif args.command == "stats":
            from i18n_lib.stats import cmd_stats
            cmd_stats(args.app)
            return 0

        elif args.command == "gen-types":
            from i18n_lib.typegen import cmd_gen_types
            cmd_gen_types(args.app)
            return 0

        elif args.command == "db-init":
            from i18n_lib.db import init_db
            init_db(args.app)
            return 0

        elif args.command == "db-query":
            from i18n_lib.db import db_query

            rows = db_query(args.sql)
            for row in rows:
                print(row)
            print(f"\n共 {len(rows)} 行")
            return 0

        elif args.command == "find-key":
            from i18n_lib.db import init_db, search_db
            from i18n_lib.config import DB_PATH

            if not DB_PATH.exists():
                init_db(args.app)

            results = search_db(args.query, args.locale, args.limit)

            print(f"🔍 搜索: \"{args.query}\"")
            print(f"   找到 {len(results)} 条匹配（最多显示 {args.limit} 条）:")
            print()

            for r in results:
                print(f"📌 {r['key']}  ({r.get('source_file', 'unknown')})")
                print(f"   [{r['locale']}] {r['value'][:80]}")
                print()

            return 0

        elif args.command == "benchmark":
            from i18n_lib.benchmark import cmd_benchmark
            cmd_benchmark(args.app)
            return 0

        elif args.command == "compile-json":
            from i18n_lib.compile_json import compile_to_json

            print("📦 编译 TS 字典为 JSON...")
            print()

            result = compile_to_json(args.app, args.output)

            print()
            print(result["output"])
            print(f"✅ 编译成功: {result['locale_count']} 个语言文件, {result['total_keys']} 条翻译")
            return 0

        elif args.command == "compile-android":
            from i18n_lib.compile_android_xml import compile_to_android_xml

            print("📱 编译 TS 字典为 Android strings.xml...")
            print()

            result = compile_to_android_xml(args.app, args.output)

            print()
            print(result["output"])
            print(f"✅ 编译成功: {result['locale_count']} 个语言, {result['total_keys']} 条翻译")
            return 0

        elif args.command == "add-key":
            from i18n_lib.addkey import add_key

            print(f"➕ 添加翻译 key: {args.key}")
            print()

            result = add_key(
                args.key,
                args.zh,
                args.en,
                args.app,
                args.file,
            )

            if not result["success"]:
                if result["reason"] == "already_exists":
                    print(f"⚠️  key 已存在: {args.key}")
                    print(f"   文件: {result['file']}")
                    return 1
                else:
                    print(f"❌ 添加失败: {result.get('reason', 'unknown')}")
                    return 1

            print(f"✅ 添加成功!")
            print(f"   文件: {result['file']}")
            print(f"   中文: {result['zh']}")
            print(f"   英文: {result['en']}")
            print()
            print("💡 提示: 运行 'compile-json' 重新编译 JSON 供 Go 使用")
            return 0

        else:
            parser.print_help()
            return 1

    except KeyboardInterrupt:
        print("\n⏹️  已取消")
        return 130
    except Exception as e:
        print(f"❌ 错误: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        return 1


if __name__ == "__main__":
    sys.path.insert(0, str(__import__("pathlib").Path(__file__).resolve().parent))
    sys.exit(main())
